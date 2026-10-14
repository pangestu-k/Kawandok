package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddlewareSkipsNonV1Paths(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	handler := Middleware(nil, next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("next handler was not called for a non-/v1 path")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestMiddlewareRequiresBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
	}{
		{"missing header", ""},
		{"wrong scheme", "Basic abc"},
		{"empty token", "Bearer "},
		{"bearer only", "Bearer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("next handler must not be reached without a token")
			})
			handler := Middleware(nil, next)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/v1/documents", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec.Code)
			}
			var body struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("invalid JSON body: %v", err)
			}
			if body.Error.Code != "UNAUTHORIZED" {
				t.Errorf("code = %q, want UNAUTHORIZED", body.Error.Code)
			}
		})
	}
}

func TestMiddlewareRejectsInvalidToken(t *testing.T) {
	authn := authenticatorFunc(func(ctx context.Context, token string) (Principal, error) {
		return Principal{}, ErrInvalidToken
	})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler must not be reached with an invalid token")
	})
	handler := Middleware(authn, next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/documents", nil)
	req.Header.Set("Authorization", "Bearer dev-nobody@dev.local")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestMiddlewarePropagatesPrincipal(t *testing.T) {
	want := Principal{ID: "u-1", Email: "admin@dev.local", Roles: []string{AdminRole}}
	authn := authenticatorFunc(func(ctx context.Context, token string) (Principal, error) {
		if token != "dev-admin@dev.local" {
			t.Errorf("token = %q, want dev-admin@dev.local", token)
		}
		return want, nil
	})

	var got Principal
	var ok bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok = PrincipalFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := Middleware(authn, next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/documents", nil)
	req.Header.Set("Authorization", "Bearer dev-admin@dev.local")
	handler.ServeHTTP(rec, req)

	if !ok {
		t.Fatal("principal missing from request context")
	}
	if got.ID != want.ID || got.Email != want.Email || !got.IsAdmin() {
		t.Errorf("principal = %+v, want %+v", got, want)
	}
}

func TestContextWithoutPrincipal(t *testing.T) {
	if _, ok := PrincipalFromContext(context.Background()); ok {
		t.Fatal("plain context must not carry a principal")
	}
}

func TestHasRole(t *testing.T) {
	p := Principal{Roles: []string{"employee", "finance"}}
	if !p.HasRole("finance") {
		t.Error("HasRole(finance) = false, want true")
	}
	if p.HasRole("admin") {
		t.Error("HasRole(admin) = true, want false")
	}
	if p.IsAdmin() {
		t.Error("IsAdmin() = true, want false")
	}
}

type authenticatorFunc func(ctx context.Context, token string) (Principal, error)

func (f authenticatorFunc) Authenticate(ctx context.Context, token string) (Principal, error) {
	return f(ctx, token)
}

var _ = errors.Is
