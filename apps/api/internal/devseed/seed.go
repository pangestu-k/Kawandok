package devseed

import (
	"context"
	"database/sql"
	"fmt"
)

func Run(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`INSERT INTO departments (name) VALUES ('General'), ('Finance'), ('HR'), ('IT')
		 ON CONFLICT (name) DO NOTHING`,
		`INSERT INTO roles (name) VALUES ('admin'), ('employee'), ('finance')
		 ON CONFLICT (name) DO NOTHING`,
		`INSERT INTO users (email, display_name, department_id)
		 SELECT v.email, v.display_name, d.id
		 FROM (VALUES
		     ('admin@dev.local', 'Dev Admin', 'General'),
		     ('budi@dev.local', 'Budi Santoso', 'General'),
		     ('siti@dev.local', 'Siti Rahayu', 'Finance'),
		     ('rian@dev.local', 'Rian Pratama', 'Finance')
		 ) AS v(email, display_name, dept)
		 JOIN departments d ON d.name = v.dept
		 ON CONFLICT (email) DO NOTHING`,
		`INSERT INTO user_roles (user_id, role_id)
		 SELECT u.id, r.id
		 FROM (VALUES
		     ('admin@dev.local', 'admin'),
		     ('admin@dev.local', 'employee'),
		     ('budi@dev.local', 'employee'),
		     ('siti@dev.local', 'finance'),
		     ('siti@dev.local', 'employee'),
		     ('rian@dev.local', 'employee')
		 ) AS v(email, role)
		 JOIN users u ON u.email = v.email
		 JOIN roles r ON r.name = v.role
		 ON CONFLICT DO NOTHING`,
	}
	for _, stmt := range statements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("devseed: %w", err)
		}
	}
	return nil
}
