package auth

import (
	"context"
	"database/sql"
	"strings"
)

const devTokenPrefix = "dev-"

type DevAuthenticator struct {
	db *sql.DB
}

func NewDev(db *sql.DB) *DevAuthenticator {
	return &DevAuthenticator{db: db}
}

func (d *DevAuthenticator) Authenticate(ctx context.Context, token string) (Principal, error) {
	if d.db == nil || !strings.HasPrefix(token, devTokenPrefix) {
		return Principal{}, ErrInvalidToken
	}
	ident := strings.TrimPrefix(token, devTokenPrefix)
	if ident == "" || len(ident) > 254 {
		return Principal{}, ErrInvalidToken
	}

	return loadPrincipal(ctx, d.db, "email", ident)
}
