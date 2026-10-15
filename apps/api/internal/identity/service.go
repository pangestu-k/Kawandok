package identity

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/lib/pq"

	"office-ai-agent/apps/api/internal/audit"
)

type Service struct {
	db       *sql.DB
	auditLog *audit.Logger
	log      *slog.Logger
}

func NewService(db *sql.DB, auditLog *audit.Logger, log *slog.Logger) *Service {
	return &Service{db: db, auditLog: auditLog, log: log}
}

type User struct {
	ID          string   `json:"id"`
	Email       string   `json:"email"`
	DisplayName string   `json:"display_name"`
	Status      string   `json:"status"`
	Department  string   `json:"department"`
	Roles       []string `json:"roles"`
}

type Role struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	UserCount int64  `json:"user_count"`
}

type Department struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	UserCount int64  `json:"user_count"`
}

func (s *Service) IsAdmin(ctx context.Context, userID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM user_roles ur
			JOIN roles r ON r.id = ur.role_id
			WHERE ur.user_id = $1::uuid AND r.name = 'admin'
		)
	`, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("identity: admin check: %w", err)
	}
	return exists, nil
}

func (s *Service) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id::text, u.email, u.display_name, u.status, d.name,
		       COALESCE(array_agg(r.name) FILTER (WHERE r.name IS NOT NULL), '{}')
		FROM users u
		JOIN departments d ON d.id = u.department_id
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		LEFT JOIN roles r ON r.id = ur.role_id
		GROUP BY u.id, d.name
		ORDER BY u.display_name
	`)
	if err != nil {
		return nil, fmt.Errorf("identity: list users: %w", err)
	}
	defer rows.Close()

	out := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.Status, &u.Department, pq.Array(&u.Roles)); err != nil {
			return nil, fmt.Errorf("identity: scan user: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Service) ListRoles(ctx context.Context) ([]Role, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id::text, r.name, count(ur.user_id)
		FROM roles r
		LEFT JOIN user_roles ur ON ur.role_id = r.id
		GROUP BY r.id, r.name
		ORDER BY r.name
	`)
	if err != nil {
		return nil, fmt.Errorf("identity: list roles: %w", err)
	}
	defer rows.Close()

	out := []Role{}
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.ID, &r.Name, &r.UserCount); err != nil {
			return nil, fmt.Errorf("identity: scan role: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Service) ListDepartments(ctx context.Context) ([]Department, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.id::text, d.name, count(u.id)
		FROM departments d
		LEFT JOIN users u ON u.department_id = d.id
		GROUP BY d.id, d.name
		ORDER BY d.name
	`)
	if err != nil {
		return nil, fmt.Errorf("identity: list departments: %w", err)
	}
	defer rows.Close()

	out := []Department{}
	for rows.Next() {
		var d Department
		if err := rows.Scan(&d.ID, &d.Name, &d.UserCount); err != nil {
			return nil, fmt.Errorf("identity: scan department: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Service) record(ctx context.Context, actorID, action, result, requestID string) {
	if s.auditLog == nil {
		return
	}
	if err := s.auditLog.Record(ctx, actorID, action, "identity", "", result, requestID); err != nil {
		s.log.Error("identity audit write failed", "error", err)
	}
}
