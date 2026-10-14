package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"
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

	var p Principal
	var roles []string
	err := d.db.QueryRowContext(ctx, `
		SELECT u.id::text, u.email, u.display_name, u.department_id::text,
		       COALESCE(array_agg(r.name) FILTER (WHERE r.name IS NOT NULL), '{}')
		FROM users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		LEFT JOIN roles r ON r.id = ur.role_id
		WHERE (u.email = $1 OR u.id::text = $1) AND u.status = 'active'
		GROUP BY u.id
	`, ident).Scan(&p.ID, &p.Email, &p.DisplayName, &p.DepartmentID, pq.Array(&roles))
	if errors.Is(err, sql.ErrNoRows) {
		return Principal{}, ErrInvalidToken
	}
	if err != nil {
		return Principal{}, fmt.Errorf("auth: user lookup failed: %w", err)
	}
	p.Roles = roles
	return p, nil
}
