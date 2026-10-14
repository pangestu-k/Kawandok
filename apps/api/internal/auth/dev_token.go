package auth

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"office-ai-agent/apps/api/internal/httpx"
)

// RegisterDevToken exposes a deliberately dev-only issuer so the browser never
// receives the HMAC key. It must not be registered outside local learning use.
func RegisterDevToken(mux *http.ServeMux, db *sql.DB, cfg JWTConfig) {
	mux.HandleFunc("POST /auth/dev/token", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
			httpx.WriteError(w, r, httpx.ErrBadRequest("invalid JSON body"))
			return
		}
		p, err := loadPrincipal(r.Context(), db, "email", strings.ToLower(strings.TrimSpace(body.Email)))
		if err != nil {
			httpx.WriteError(w, r, httpx.ErrUnauthorized())
			return
		}
		token, err := SignHS256("dev:"+p.Email, cfg, time.Now())
		if err != nil {
			httpx.WriteError(w, r, httpx.NewAPIError(http.StatusInternalServerError, "INTERNAL", "could not issue token"))
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"access_token": token, "token_type": "Bearer", "expires_in": int(cfg.TTL.Seconds())})
	})
}
