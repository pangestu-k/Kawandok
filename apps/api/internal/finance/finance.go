package finance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"office-ai-agent/apps/api/internal/audit"
	"office-ai-agent/apps/api/internal/auth"
	"office-ai-agent/apps/api/internal/httpx"
)

const FinanceRole = "finance"

const maxPeriodDays = 366 * 5

type Service struct {
	db       *sql.DB
	auditLog *audit.Logger
	log      *slog.Logger
}

func NewService(db *sql.DB, auditLog *audit.Logger, log *slog.Logger) *Service {
	return &Service{db: db, auditLog: auditLog, log: log}
}

type departmentScope struct {
	all        bool
	department string
}

func (s *Service) resolveScope(ctx context.Context, p auth.Principal, requested string) (departmentScope, error) {
	elevated := p.IsAdmin() || p.HasRole(FinanceRole)
	requested = strings.TrimSpace(requested)

	if requested == "" || strings.EqualFold(requested, "all") {
		if strings.EqualFold(requested, "all") {
			if !elevated {
				return departmentScope{}, httpx.NewAPIError(403, "FINANCE_FORBIDDEN",
					"accessing all departments requires the finance role")
			}
			return departmentScope{all: true}, nil
		}
		if elevated {
			return departmentScope{all: true}, nil
		}
		if p.DepartmentID == "" {
			return departmentScope{}, httpx.NewAPIError(403, "FINANCE_FORBIDDEN",
				"your account is not linked to a department; ask an administrator")
		}
		return departmentScope{department: p.DepartmentID}, nil
	}

	var deptID string
	err := s.db.QueryRowContext(ctx,
		`SELECT id::text FROM departments WHERE lower(name) = lower($1)`, requested).Scan(&deptID)
	if errors.Is(err, sql.ErrNoRows) {
		return departmentScope{}, httpx.ErrBadRequest(fmt.Sprintf("unknown department %q", requested))
	}
	if err != nil {
		s.log.Error("department lookup failed", "error", err)
		return departmentScope{}, httpx.NewAPIError(500, "INTERNAL", "department lookup failed")
	}

	if elevated {
		return departmentScope{department: deptID}, nil
	}
	if !strings.EqualFold(p.DepartmentID, deptID) {
		return departmentScope{}, httpx.NewAPIError(403, "FINANCE_FORBIDDEN",
			"you can only access financial data of your own department")
	}
	return departmentScope{department: deptID}, nil
}

type CategoryTotal struct {
	Category       string `json:"category"`
	TotalMinorUnit int64  `json:"total_minor_units"`
}

type SummaryResult struct {
	Currency   string          `json:"currency"`
	TotalMinor int64           `json:"total_minor_units"`
	ByCategory []CategoryTotal `json:"by_category"`
	Start      time.Time       `json:"start"`
	End        time.Time       `json:"end"`
}

func (s *Service) Summary(ctx context.Context, p auth.Principal, deptName string, start, end time.Time, requestID string) (*SummaryResult, error) {
	if err := validatePeriod(start, end); err != nil {
		return nil, err
	}
	scope, err := s.resolveScope(ctx, p, deptName)
	if err != nil {
		s.record(ctx, p.ID, "finance.summary", "denied", requestID)
		return nil, err
	}

	totals, err := s.totalsByCurrency(ctx, scope, start, end)
	if err != nil {
		return nil, err
	}
	if len(totals) > 1 {
		return nil, httpx.NewAPIError(409, "CONFLICTING_CURRENCIES",
			"the selected period contains multiple currencies; results would be ambiguous")
	}
	currency, total := "", int64(0)
	if len(totals) == 1 {
		currency, total = totals[0].currency, totals[0].total
	}

	byCategory := []CategoryTotal{}
	args := []any{start, end}
	deptClause := deptClause(scope, &args)
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.category, COALESCE(sum(t.amount_minor_units), 0)
		FROM financial_transactions t
		JOIN financial_reports f ON f.id = t.report_id
		WHERE f.validation_status = 'validated'`+deptClause+`
		  AND t.transaction_date BETWEEN $1 AND $2
		GROUP BY t.category
		ORDER BY sum(t.amount_minor_units) DESC
		LIMIT 25
	`, args...)
	if err != nil {
		s.log.Error("category aggregate failed", "error", err)
		return nil, httpx.NewAPIError(500, "INTERNAL", "financial aggregate failed")
	}
	defer rows.Close()
	for rows.Next() {
		var ct CategoryTotal
		if err := rows.Scan(&ct.Category, &ct.TotalMinorUnit); err == nil {
			byCategory = append(byCategory, ct)
		}
	}

	s.record(ctx, p.ID, "finance.summary", "allowed", requestID)
	return &SummaryResult{
		Currency:   currency,
		TotalMinor: total,
		ByCategory: byCategory,
		Start:      start,
		End:        end,
	}, nil
}

type ComparisonResult struct {
	Currency     string    `json:"currency"`
	FirstMinor   int64     `json:"first_period_minor_units"`
	SecondMinor  int64     `json:"second_period_minor_units"`
	DeltaMinor   int64     `json:"delta_minor_units"`
	DeltaPercent *float64  `json:"delta_percent,omitempty"`
	FirstStart   time.Time `json:"first_period_start"`
	FirstEnd     time.Time `json:"first_period_end"`
	SecondStart  time.Time `json:"second_period_start"`
	SecondEnd    time.Time `json:"second_period_end"`
}

func (s *Service) Compare(ctx context.Context, p auth.Principal, deptName string, aStart, aEnd, bStart, bEnd time.Time, requestID string) (*ComparisonResult, error) {
	if err := validatePeriod(aStart, aEnd); err != nil {
		return nil, err
	}
	if err := validatePeriod(bStart, bEnd); err != nil {
		return nil, err
	}
	scope, err := s.resolveScope(ctx, p, deptName)
	if err != nil {
		s.record(ctx, p.ID, "finance.compare", "denied", requestID)
		return nil, err
	}

	totalsA, err := s.totalsByCurrency(ctx, scope, aStart, aEnd)
	if err != nil {
		return nil, err
	}
	totalsB, err := s.totalsByCurrency(ctx, scope, bStart, bEnd)
	if err != nil {
		return nil, err
	}
	if len(totalsA) > 1 || len(totalsB) > 1 {
		return nil, httpx.NewAPIError(409, "CONFLICTING_CURRENCIES",
			"the selected periods contain multiple currencies; comparison would be ambiguous")
	}

	currencyA, totalA := "", int64(0)
	if len(totalsA) == 1 {
		currencyA, totalA = totalsA[0].currency, totalsA[0].total
	}
	currencyB, totalB := "", int64(0)
	if len(totalsB) == 1 {
		currencyB, totalB = totalsB[0].currency, totalsB[0].total
	}
	if currencyA != "" && currencyB != "" && currencyA != currencyB {
		return nil, httpx.NewAPIError(409, "CONFLICTING_CURRENCIES",
			"the two periods use different currencies; comparison requires a single currency")
	}

	result := &ComparisonResult{
		Currency:    currencyA,
		FirstMinor:  totalA,
		SecondMinor: totalB,
		DeltaMinor:  totalA - totalB,
		FirstStart:  aStart, FirstEnd: aEnd,
		SecondStart: bStart, SecondEnd: bEnd,
	}
	if totalB != 0 {
		pct := math.Round(float64(totalA-totalB)/math.Abs(float64(totalB))*10000) / 100
		result.DeltaPercent = &pct
	}
	s.record(ctx, p.ID, "finance.compare", "allowed", requestID)
	return result, nil
}

type currencyTotal struct {
	currency string
	total    int64
}

func (s *Service) totalsByCurrency(ctx context.Context, scope departmentScope, start, end time.Time) ([]currencyTotal, error) {
	args := []any{start, end}
	deptClause := deptClause(scope, &args)
	rows, err := s.db.QueryContext(ctx, `
		SELECT f.currency, COALESCE(sum(t.amount_minor_units), 0)
		FROM financial_transactions t
		JOIN financial_reports f ON f.id = t.report_id
		WHERE f.validation_status = 'validated'`+deptClause+`
		  AND t.transaction_date BETWEEN $1 AND $2
		GROUP BY f.currency
	`, args...)
	if err != nil {
		s.log.Error("financial aggregate failed", "error", err)
		return nil, httpx.NewAPIError(500, "INTERNAL", "financial aggregate failed")
	}
	defer rows.Close()

	out := []currencyTotal{}
	for rows.Next() {
		var ct currencyTotal
		if err := rows.Scan(&ct.currency, &ct.total); err != nil {
			return nil, httpx.NewAPIError(500, "INTERNAL", "financial aggregate failed")
		}
		out = append(out, ct)
	}
	return out, rows.Err()
}

func deptClause(scope departmentScope, args *[]any) string {
	if scope.all {
		return ""
	}
	*args = append(*args, scope.department)
	return fmt.Sprintf(" AND f.department_id = $%d::uuid", len(*args))
}

func validatePeriod(start, end time.Time) error {
	if end.Before(start) {
		return httpx.ErrBadRequest("period end is before period start")
	}
	if end.Sub(start) > maxPeriodDays*24*time.Hour {
		return httpx.ErrBadRequest("period range exceeds five years")
	}
	return nil
}

func (s *Service) record(ctx context.Context, actorID, action, result, requestID string) {
	if s.auditLog == nil {
		return
	}
	if err := s.auditLog.Record(ctx, actorID, action, "finance", "", result, requestID); err != nil {
		s.log.Error("finance audit write failed", "error", err)
	}
}
