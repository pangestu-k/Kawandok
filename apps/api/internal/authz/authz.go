package authz

import (
	"context"
	"database/sql"
	"fmt"

	"office-ai-agent/apps/api/internal/auth"
)

type Checker struct {
	db *sql.DB
}

func NewChecker(db *sql.DB) *Checker {
	return &Checker{db: db}
}

type Scope struct {
	AdminAll  bool
	Principal auth.Principal
}

const grantMatchCTE = `EXISTS(
    SELECT 1
    FROM document_grants g
    JOIN users gu ON gu.id = $%d::uuid AND gu.status = 'active'
    LEFT JOIN user_roles gur ON gur.user_id = gu.id
    WHERE g.document_id = %s
      AND g.action = $%d
      AND (
          (g.principal_type = 'user' AND g.principal_id = gu.id)
       OR (g.principal_type = 'role' AND g.principal_id = gur.role_id)
       OR (g.principal_type = 'department' AND g.principal_id = gu.department_id)
      )
)`

func ScopeFor(p auth.Principal) Scope {
	return Scope{AdminAll: p.IsAdmin(), Principal: p}
}

func AppendScopeClause(where []string, args *[]any, scope Scope, action, documentRef string) []string {
	if scope.AdminAll {
		return where
	}
	*args = append(*args, scope.Principal.ID, action)
	userArg := len(*args) - 1
	actionArg := len(*args)
	return append(where, fmt.Sprintf(grantMatchCTE, userArg, documentRef, actionArg))
}

func (c *Checker) Can(ctx context.Context, p auth.Principal, action, documentID string) (bool, error) {
	if p.IsAdmin() {
		return true, nil
	}
	query := "SELECT " + fmt.Sprintf(grantMatchCTE, 1, "$2::uuid", 3)
	var exists bool
	err := c.db.QueryRowContext(ctx, query, p.ID, documentID, action).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("authz: check failed: %w", err)
	}
	return exists, nil
}
