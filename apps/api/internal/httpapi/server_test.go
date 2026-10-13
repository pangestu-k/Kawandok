package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"office-ai-agent/apps/api/internal/config"
)

type fakePinger struct {
	err error
}

func (f fakePinger) PingContext(context.Context) error {
	return f.err
}

func testConfig() config.Config {
	return config.Config{Env: "dev", Port: "8080", LLMProvider: "mock", EmbeddingProvider: "mock", EmbeddingDim: 1536}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHealthz(t *testing.T) {
	srv := New(testConfig(), testLogger(), nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want ok", body["status"])
	}
}

func TestReadyz(t *testing.T) {
	tests := []struct {
		name       string
		pinger     Pinger
		wantStatus int
		wantDB     string
	}{
		{
			name:       "no database configured",
			pinger:     nil,
			wantStatus: http.StatusOK,
			wantDB:     "not_configured",
		},
		{
			name:       "database reachable",
			pinger:     fakePinger{},
			wantStatus: http.StatusOK,
			wantDB:     "ok",
		},
		{
			name:       "database unreachable",
			pinger:     fakePinger{err: errors.New("connection refused")},
			wantStatus: http.StatusServiceUnavailable,
			wantDB:     "unreachable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := New(testConfig(), testLogger(), tt.pinger)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			var body struct {
				Status string            `json:"status"`
				Checks map[string]string `json:"checks"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			if body.Checks["database"] != tt.wantDB {
				t.Errorf("database check = %q, want %q", body.Checks["database"], tt.wantDB)
			}
			if tt.wantStatus == http.StatusOK && body.Status != "ok" {
				t.Errorf("status = %q, want ok", body.Status)
			}
			if tt.wantStatus == http.StatusServiceUnavailable && body.Status != "unavailable" {
				t.Errorf("status = %q, want unavailable", body.Status)
			}
		})
	}
}

func TestUnknownRouteReturnsErrorEnvelope(t *testing.T) {
	srv := New(testConfig(), testLogger(), nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body.Error.Code != "NOT_FOUND" {
		t.Errorf("code = %q, want NOT_FOUND", body.Error.Code)
	}
	if body.Error.RequestID == "" {
		t.Error("request_id must not be empty")
	}
}

func TestRequestIDHeader(t *testing.T) {
	srv := New(testConfig(), testLogger(), nil)

	t.Run("generated when missing", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		srv.Handler().ServeHTTP(rec, req)
		if got := rec.Header().Get("X-Request-ID"); got == "" {
			t.Error("X-Request-ID must be set")
		}
	})

	t.Run("existing id is preserved", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.Header.Set("X-Request-ID", "my-trace-id")
		srv.Handler().ServeHTTP(rec, req)
		if got := rec.Header().Get("X-Request-ID"); got != "my-trace-id" {
			t.Errorf("X-Request-ID = %q, want my-trace-id", got)
		}
	})

	t.Run("oversized id is replaced", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.Header.Set("X-Request-ID", strings.Repeat("x", 129))
		srv.Handler().ServeHTTP(rec, req)
		got := rec.Header().Get("X-Request-ID")
		if got == "" || len(got) > 128 {
			t.Errorf("X-Request-ID = %q, must be generated and <= 128 chars", got)
		}
	})
}

func TestSecureHeadersSet(t *testing.T) {
	srv := New(testConfig(), testLogger(), nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	srv.Handler().ServeHTTP(rec, req)

	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("X-Content-Type-Options must be nosniff")
	}
	if rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("X-Frame-Options must be DENY")
	}
	if rec.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Error("Referrer-Policy must be no-referrer")
	}
}

func TestPanicIsRecovered(t *testing.T) {
	srv := New(testConfig(), testLogger(), nil)
	panicking := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})
	handler := srv.withRequestLog(secureHeaders(withRequestID(withRecovery(testLogger(), panicking))))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body.Error.Code != "INTERNAL" {
		t.Errorf("code = %q, want INTERNAL", body.Error.Code)
	}
}
