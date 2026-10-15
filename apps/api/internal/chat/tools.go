package chat

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"office-ai-agent/apps/api/internal/auth"
	"office-ai-agent/apps/api/internal/finance"
	"office-ai-agent/apps/api/internal/httpx"
)

type FinanceTools interface {
	Summary(ctx context.Context, p auth.Principal, dept string, start, end time.Time, requestID string) (*finance.SummaryResult, error)
	Compare(ctx context.Context, p auth.Principal, dept string, aStart, aEnd, bStart, bEnd time.Time, requestID string) (*finance.ComparisonResult, error)
}

const financeHelpText = "Untuk pertanyaan keuangan, sertakan tanggal dengan format YYYY-MM-DD. Contoh: " +
	"'Berapa total pendapatan departemen Finance 2026-01-01 2026-01-31?' atau " +
	"'Bandingkan pendapatan 2026-01-01 dan 2026-02-01'. " +
	"Tanpa tanggal yang valid saya tidak dapat menghitung angka keuangan."

var (
	datePattern     = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)
	deptPattern     = regexp.MustCompile(`(?:departemen|department)\s+([a-zA-Z0-9][a-zA-Z0-9 \-]*)`)
	financeKeywords = []string{"total", "ringkasan", "summary", "pendapatan", "pengeluaran", "revenue", "expense", "keuangan", "finansial", "penghasilan", "belanja"}
	compareKeywords = []string{"bandingkan", "compare", "dibanding", "komparasi", " vs ", "versus"}
)

type financeIntent struct {
	kind         string // "summary" | "compare" | "help"
	dept         string
	aStart, aEnd time.Time
	bStart, bEnd time.Time
}

func detectFinanceIntent(message string) *financeIntent {
	lower := " " + strings.ToLower(message) + " "

	hasKeyword := false
	isCompare := false
	for _, kw := range financeKeywords {
		if strings.Contains(lower, kw) {
			hasKeyword = true
			break
		}
	}
	for _, kw := range compareKeywords {
		if strings.Contains(lower, kw) {
			isCompare = true
		}
	}
	if !hasKeyword && !isCompare {
		return nil
	}

	if len(datePattern.FindAllString(message, -1)) == 0 {
		return nil
	}

	intent := &financeIntent{kind: "help", dept: detectDepartment(message)}
	dates := datePattern.FindAllString(message, -1)
	parsed := make([]time.Time, 0, len(dates))
	for _, d := range dates {
		t, err := time.Parse(time.DateOnly, d)
		if err == nil {
			parsed = append(parsed, t)
		}
	}

	switch {
	case isCompare && len(parsed) >= 4:
		intent.kind = "compare"
		intent.aStart, intent.aEnd = parsed[0], parsed[1]
		intent.bStart, intent.bEnd = parsed[2], parsed[3]
	case isCompare && len(parsed) == 2:
		intent.kind = "compare"
		intent.aStart, intent.aEnd = monthBounds(parsed[0])
		intent.bStart, intent.bEnd = monthBounds(parsed[1])
	case len(parsed) == 1:
		intent.kind = "summary"
		intent.aStart, intent.aEnd = monthBounds(parsed[0])
	case len(parsed) >= 2:
		intent.kind = "summary"
		intent.aStart, intent.aEnd = parsed[0], parsed[1]
	}
	return intent
}

func detectDepartment(message string) string {
	lower := strings.ToLower(message)
	if strings.Contains(lower, "semua departemen") || strings.Contains(lower, "all departments") {
		return "all"
	}
	if m := deptPattern.FindStringSubmatch(message); m != nil {
		name := datePattern.ReplaceAllString(m[1], "")
		name = strings.TrimSpace(name)
		changed := true
		for changed && name != "" {
			changed = false
			for _, suffix := range []string{" dan", " and", " dengan", " vs", " versus", " to"} {
				if strings.HasSuffix(strings.ToLower(name), suffix) {
					name = strings.TrimSpace(name[:len(name)-len(suffix)])
					changed = true
				}
			}
		}
		name = strings.TrimRight(name, ".,?!;:- ")
		if name != "" {
			return name
		}
	}
	return ""
}

func monthBounds(t time.Time) (time.Time, time.Time) {
	start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, -1)
	return start, end
}

func (s *Service) handleFinanceIntent(ctx context.Context, p auth.Principal, sessionID string, intent *financeIntent, clientRequestID string, emit Emitter) error {
	if err := emit(Event{Type: "tool_status", Data: map[string]string{"stage": "finance"}}); err != nil {
		return err
	}
	requestID := requestIDFromContext(ctx)

	switch intent.kind {
	case "help":
		return s.finishFinanceAnswer(ctx, sessionID, clientRequestID, financeHelpText, emit)

	case "summary":
		result, err := s.finance.Summary(ctx, p, intent.dept, intent.aStart, intent.aEnd, requestID)
		if err != nil {
			return emitAPIError(emit, err)
		}
		return s.finishFinanceAnswer(ctx, sessionID, clientRequestID, formatSummary(result, intent.dept), emit)

	case "compare":
		result, err := s.finance.Compare(ctx, p, intent.dept, intent.aStart, intent.aEnd, intent.bStart, intent.bEnd, requestID)
		if err != nil {
			return emitAPIError(emit, err)
		}
		return s.finishFinanceAnswer(ctx, sessionID, clientRequestID, formatComparison(result, intent.dept), emit)
	}
	return nil
}

func (s *Service) finishFinanceAnswer(ctx context.Context, sessionID, clientRequestID, answer string, emit Emitter) error {
	const stride = 64
	for i := 0; i < len(answer); i += stride {
		end := i + stride
		if end > len(answer) {
			end = len(answer)
		}
		if err := emit(Event{Type: "text_delta", Data: map[string]string{"delta": answer[i:end]}}); err != nil {
			return err
		}
	}
	messageID, err := s.store.AppendMessage(ctx, sessionID, "assistant", answer, nil, clientRequestID)
	if err != nil {
		s.log.Error("assistant message persist failed", "error", err)
		return emitAPIError(emit, httpx.NewAPIError(500, "INTERNAL", "could not store answer"))
	}
	return emit(Event{Type: "done", Data: map[string]any{
		"message_id": messageID,
		"citations":  []Citation{},
	}})
}

func formatSummary(r *finance.SummaryResult, dept string) string {
	scopeLabel := "semua departemen"
	if dept != "" && !strings.EqualFold(dept, "all") {
		scopeLabel = "departemen " + dept
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Total %s untuk periode %s s/d %s: %s.",
		scopeLabel, r.Start.Format(time.DateOnly), r.End.Format(time.DateOnly),
		formatMinorAmount(r.Currency, r.TotalMinor))
	for _, ct := range r.ByCategory {
		fmt.Fprintf(&sb, "\n- %s: %s", ct.Category, formatMinorAmount(r.Currency, ct.TotalMinorUnit))
	}
	if len(r.ByCategory) == 0 {
		sb.WriteString("\nTidak ada transaksi pada periode ini.")
	}
	return sb.String()
}

func formatComparison(r *finance.ComparisonResult, dept string) string {
	scopeLabel := "semua departemen"
	if dept != "" && !strings.EqualFold(dept, "all") {
		scopeLabel = "departemen " + dept
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Perbandingan %s:\n- Periode %s s/d %s: %s\n- Periode %s s/d %s: %s\n- Selisih: %s",
		scopeLabel,
		r.FirstStart.Format(time.DateOnly), r.FirstEnd.Format(time.DateOnly), formatMinorAmount(r.Currency, r.FirstMinor),
		r.SecondStart.Format(time.DateOnly), r.SecondEnd.Format(time.DateOnly), formatMinorAmount(r.Currency, r.SecondMinor),
		formatMinorAmount(r.Currency, r.DeltaMinor))
	if r.DeltaPercent != nil {
		fmt.Fprintf(&sb, " (%s%%)", formatPercent(*r.DeltaPercent))
	}
	return sb.String()
}

func formatMinorAmount(currency string, minor int64) string {
	sign := ""
	if minor < 0 {
		sign = "-"
		minor = -minor
	}
	numPart := fmt.Sprintf("%s,%02d", groupDigits(minor/100), minor%100)
	if currency == "" {
		return sign + "-" + numPart
	}
	return sign + currency + " " + numPart
}

func groupDigits(n int64) string {
	digits := strconv.FormatInt(n, 10)
	var sb strings.Builder
	pre := len(digits) % 3
	if pre > 0 {
		sb.WriteString(digits[:pre])
	}
	for i := pre; i < len(digits); i += 3 {
		if sb.Len() > 0 {
			sb.WriteByte('.')
		}
		sb.WriteString(digits[i : i+3])
	}
	return sb.String()
}

func formatPercent(p float64) string {
	sign := ""
	if p > 0 {
		sign = "+"
	}
	return fmt.Sprintf("%s%.2f", sign, p)
}
