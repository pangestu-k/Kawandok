package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	"office-ai-agent/apps/api/internal/auth"
	"office-ai-agent/apps/api/internal/finance"
	"office-ai-agent/apps/api/internal/httpx"
	"office-ai-agent/apps/api/internal/llm"
)

func TestDetectFinanceIntent(t *testing.T) {
	tests := []struct {
		name      string
		message   string
		wantNil   bool
		wantKind  string
		wantDept  string
		checkVars func(t *testing.T, intent *financeIntent)
	}{
		{
			name:    "non finance message ignored",
			message: "Apa isi dokumen kebijakan cuti?",
			wantNil: true,
		},
		{
			name:     "summary with explicit range",
			message:  "Berapa total pendapatan departemen Finance 2026-01-01 2026-01-31?",
			wantKind: "summary",
			wantDept: "Finance",
			checkVars: func(t *testing.T, intent *financeIntent) {
				if intent.aStart.Format(time.DateOnly) != "2026-01-01" || intent.aEnd.Format(time.DateOnly) != "2026-01-31" {
					t.Errorf("range = %s..%s", intent.aStart, intent.aEnd)
				}
			},
		},
		{
			name:     "compare two months",
			message:  "Bandingkan pendapatan 2026-01-01 dan 2026-02-01",
			wantKind: "compare",
			checkVars: func(t *testing.T, intent *financeIntent) {
				if intent.aStart.Format(time.DateOnly) != "2026-01-01" || intent.aEnd.Format(time.DateOnly) != "2026-01-31" {
					t.Errorf("period A = %s..%s", intent.aStart, intent.aEnd)
				}
				if intent.bStart.Format(time.DateOnly) != "2026-02-01" || intent.bEnd.Format(time.DateOnly) != "2026-02-28" {
					t.Errorf("period B = %s..%s", intent.bStart, intent.bEnd)
				}
			},
		},
		{
			name:     "all departments keyword",
			message:  "Ringkasan keuangan semua departemen 2026-01-01 2026-01-31",
			wantKind: "summary",
			wantDept: "all",
		},
		{
			name:     "keyword without dates asks for help",
			message:  "Berapa total pendapatan kita?",
			wantKind: "help",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent := detectFinanceIntent(tt.message)
			if tt.wantNil {
				if intent != nil {
					t.Fatalf("intent = %+v, want nil", intent)
				}
				return
			}
			if intent == nil {
				t.Fatal("intent = nil, want detected")
			}
			if intent.kind != tt.wantKind {
				t.Errorf("kind = %q, want %q", intent.kind, tt.wantKind)
			}
			if intent.dept != tt.wantDept {
				t.Errorf("dept = %q, want %q", intent.dept, tt.wantDept)
			}
			if tt.checkVars != nil {
				tt.checkVars(t, intent)
			}
		})
	}
}

type recordingFinance struct {
	summaryCalled bool
	compareCalled bool
	deniedDept    string
}

func (r *recordingFinance) Summary(_ context.Context, _ auth.Principal, dept string, _, _ time.Time, _ string) (*finance.SummaryResult, error) {
	r.summaryCalled = true
	if dept == "Forbidden" {
		return nil, httpx.NewAPIError(403, "FINANCE_FORBIDDEN", "not allowed")
	}
	return &finance.SummaryResult{
		Currency:   "IDR",
		TotalMinor: 3_200_000_000,
		ByCategory: []finance.CategoryTotal{
			{Category: "revenue", TotalMinorUnit: 3_200_000_000},
		},
		Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
	}, nil
}

func (r *recordingFinance) Compare(_ context.Context, _ auth.Principal, _ string, _, _, _, _ time.Time, _ string) (*finance.ComparisonResult, error) {
	r.compareCalled = true
	pct := 25.0
	return &finance.ComparisonResult{
		Currency:     "IDR",
		FirstMinor:   3_200_000_000,
		SecondMinor:  2_400_000_000,
		DeltaMinor:   800_000_000,
		DeltaPercent: &pct,
	}, nil
}

func TestFinanceSummaryThroughChat(t *testing.T) {
	store := newFakeStore()
	tools := &recordingFinance{}
	svc := NewService(store, &fakeRetriever{}, llm.NewGroundedMock(),
		&fakeAuthorizer{readable: map[string]bool{}}, tools, testLogger)

	p := auth.Principal{ID: "u-finance", Roles: []string{"finance"}}
	sessID, _ := svc.CreateSession(context.Background(), p, "")
	out := collectEvents(t, svc, p, sessID, "Berapa total pendapatan 2026-01-01 2026-01-31?", "req-fin-1")

	if !tools.summaryCalled {
		t.Fatal("finance summary tool was not called")
	}
	reply := strings.Join(out.textDeltas, "")
	if !strings.Contains(reply, "IDR 32.000.000,00") {
		t.Errorf("reply %q must contain exact fixture total", reply)
	}
	if !strings.Contains(reply, "- revenue: IDR 32.000.000,00") {
		t.Errorf("reply %q must break down by category", reply)
	}
	if out.citations != 0 {
		t.Errorf("finance answers must not emit document citations, got %d", out.citations)
	}
}

func TestFinanceForbiddenThroughChat(t *testing.T) {
	store := newFakeStore()
	tools := &recordingFinance{}
	svc := NewService(store, &fakeRetriever{}, llm.NewGroundedMock(),
		&fakeAuthorizer{readable: map[string]bool{}}, tools, testLogger)

	p := auth.Principal{ID: "u-employee", Roles: []string{"employee"}}
	sessID, _ := svc.CreateSession(context.Background(), p, "")
	out := collectEvents(t, svc, p, sessID, "Berapa total pendapatan departemen Forbidden 2026-01-01 2026-01-31?", "req-fin-2")

	if out.errorCode != "FINANCE_FORBIDDEN" {
		t.Errorf("error = %q, want FINANCE_FORBIDDEN", out.errorCode)
	}
}

func TestFinanceCompareThroughChat(t *testing.T) {
	store := newFakeStore()
	tools := &recordingFinance{}
	svc := NewService(store, &fakeRetriever{}, llm.NewGroundedMock(),
		&fakeAuthorizer{readable: map[string]bool{}}, tools, testLogger)

	p := auth.Principal{ID: "u-finance", Roles: []string{"finance"}}
	sessID, _ := svc.CreateSession(context.Background(), p, "")
	out := collectEvents(t, svc, p, sessID, "Bandingkan pendapatan 2026-01-01 dan 2026-02-01", "req-fin-3")

	if !tools.compareCalled {
		t.Fatal("finance compare tool was not called")
	}
	reply := strings.Join(out.textDeltas, "")
	if !strings.Contains(reply, "+25.00%") {
		t.Errorf("reply %q must contain variance percent", reply)
	}
}

func TestFinanceHelpWithoutDates(t *testing.T) {
	store := newFakeStore()
	tools := &recordingFinance{}
	svc := NewService(store, &fakeRetriever{}, llm.NewGroundedMock(),
		&fakeAuthorizer{readable: map[string]bool{}}, tools, testLogger)

	p := auth.Principal{ID: "u-finance", Roles: []string{"finance"}}
	sessID, _ := svc.CreateSession(context.Background(), p, "")
	out := collectEvents(t, svc, p, sessID, "Berapa total pendapatan kita?", "req-fin-4")

	if tools.summaryCalled || tools.compareCalled {
		t.Error("no tool may run without explicit dates")
	}
	reply := strings.Join(out.textDeltas, "")
	if !strings.Contains(reply, "YYYY-MM-DD") {
		t.Errorf("help reply must explain the date format, got %q", reply)
	}
}

func TestFormatMinorAmount(t *testing.T) {
	if got := formatMinorAmount("IDR", 3_200_000_000); got != "IDR 32.000.000,00" {
		t.Errorf("formatMinorAmount() = %q", got)
	}
	if got := formatMinorAmount("IDR", -150); got != "-IDR 1,50" {
		t.Errorf("formatMinorAmount() negative = %q", got)
	}
	if got := formatMinorAmount("", 0); got != "-0,00" {
		t.Errorf("formatMinorAmount() empty currency = %q", got)
	}
}
