package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

const hs256 = "HS256"

type JWTConfig struct {
	Issuer   string
	Audience string
	Key      []byte
	TTL      time.Duration
}

type JWTAuthenticator struct {
	db  *sql.DB
	cfg JWTConfig
	now func() time.Time
}

func NewJWT(db *sql.DB, cfg JWTConfig) *JWTAuthenticator {
	return &JWTAuthenticator{db: db, cfg: cfg, now: time.Now}
}

func (a *JWTAuthenticator) Authenticate(ctx context.Context, token string) (Principal, error) {
	subject, err := VerifyHS256(token, a.cfg, a.now())
	if err != nil {
		return Principal{}, ErrInvalidToken
	}
	return loadPrincipal(ctx, a.db, "external_subject", subject)
}

// SignHS256 is used only by the development token endpoint. OIDC will replace
// token issuance while retaining the external_subject mapping.
func SignHS256(subject string, cfg JWTConfig, now time.Time) (string, error) {
	if strings.TrimSpace(subject) == "" || len(cfg.Key) < 32 || cfg.TTL <= 0 {
		return "", errors.New("invalid JWT signing configuration")
	}
	header, _ := json.Marshal(map[string]string{"alg": hs256, "typ": "JWT"})
	claims, err := json.Marshal(map[string]any{
		"iss": cfg.Issuer, "aud": cfg.Audience, "sub": subject,
		"iat": now.Unix(), "exp": now.Add(cfg.TTL).Unix(),
	})
	if err != nil {
		return "", err
	}
	parts := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	mac := hmac.New(sha256.New, cfg.Key)
	_, _ = mac.Write([]byte(parts))
	return parts + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func VerifyHS256(token string, cfg JWTConfig, now time.Time) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || len(cfg.Key) < 32 {
		return "", ErrInvalidToken
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", ErrInvalidToken
	}
	var header struct {
		Alg string `json:"alg"`
	}
	if json.Unmarshal(headerJSON, &header) != nil || header.Alg != hs256 {
		return "", ErrInvalidToken
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", ErrInvalidToken
	}
	mac := hmac.New(sha256.New, cfg.Key)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return "", ErrInvalidToken
	}
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ErrInvalidToken
	}
	var claims struct {
		Issuer   string          `json:"iss"`
		Audience json.RawMessage `json:"aud"`
		Subject  string          `json:"sub"`
		Expires  int64           `json:"exp"`
	}
	if json.Unmarshal(claimsJSON, &claims) != nil || claims.Issuer != cfg.Issuer ||
		claims.Subject == "" || claims.Expires <= now.Unix() || !hasAudience(claims.Audience, cfg.Audience) {
		return "", ErrInvalidToken
	}
	return claims.Subject, nil
}

func hasAudience(raw json.RawMessage, want string) bool {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return one == want
	}
	var many []string
	if json.Unmarshal(raw, &many) != nil {
		return false
	}
	for _, got := range many {
		if got == want {
			return true
		}
	}
	return false
}

func loadPrincipal(ctx context.Context, db *sql.DB, column, value string) (Principal, error) {
	if db == nil || value == "" {
		return Principal{}, ErrInvalidToken
	}
	if column != "email" && column != "external_subject" {
		return Principal{}, fmt.Errorf("auth: unsupported principal lookup")
	}
	var p Principal
	var roles []string
	err := db.QueryRowContext(ctx, `
		SELECT u.id::text, u.email, u.display_name, u.department_id::text,
		       COALESCE(array_agg(r.name) FILTER (WHERE r.name IS NOT NULL), '{}')
		FROM users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		LEFT JOIN roles r ON r.id = ur.role_id
		WHERE `+column+` = $1 AND u.status = 'active'
		GROUP BY u.id
	`, value).Scan(&p.ID, &p.Email, &p.DisplayName, &p.DepartmentID, pq.Array(&roles))
	if errors.Is(err, sql.ErrNoRows) {
		return Principal{}, ErrInvalidToken
	}
	if err != nil {
		return Principal{}, fmt.Errorf("auth: user lookup failed: %w", err)
	}
	p.Roles = roles
	return p, nil
}
