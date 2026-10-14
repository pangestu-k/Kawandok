package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"office-ai-agent/apps/api/internal/httpx"
)

const AdminRole = "admin"

var ErrInvalidToken = errors.New("auth: invalid token")

type Principal struct {
	ID           string   `json:"id"`
	Email        string   `json:"email"`
	DisplayName  string   `json:"display_name"`
	DepartmentID string   `json:"department_id"`
	Roles        []string `json:"roles"`
}

func (p Principal) HasRole(name string) bool {
	for _, r := range p.Roles {
		if r == name {
			return true
		}
	}
	return false
}

func (p Principal) IsAdmin() bool {
	return p.HasRole(AdminRole)
}

type Authenticator interface {
	Authenticate(ctx context.Context, token string) (Principal, error)
}

type ctxKey int

const principalKey ctxKey = 0

func ContextWithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey).(Principal)
	return p, ok
}

func Middleware(a Authenticator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/") {
			next.ServeHTTP(w, r)
			return
		}
		token := bearerToken(r)
		if token == "" {
			httpx.WriteError(w, r, httpx.ErrUnauthorized())
			return
		}
		p, err := a.Authenticate(r.Context(), token)
		if err != nil {
			httpx.WriteError(w, r, httpx.ErrUnauthorized())
			return
		}
		next.ServeHTTP(w, r.WithContext(ContextWithPrincipal(r.Context(), p)))
	})
}

func bearerToken(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}
