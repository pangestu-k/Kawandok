package identity

import (
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"office-ai-agent/apps/api/internal/audit"
	"office-ai-agent/apps/api/internal/auth"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("ACL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ACL_TEST_DATABASE_URL not set; skipping identity integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("database unreachable: %v", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("database unreachable: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`TRUNCATE document_chunks, ingestion_outbox, ingestion_jobs, document_versions,
		documents, document_grants, audit_events, chat_messages, chat_sessions,
		financial_transactions, financial_reports, user_roles, users, roles, departments CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return db
}

func newIdentityServer(t *testing.T, db *sql.DB) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(db, audit.NewLogger(db), logger)
	mux := http.NewServeMux()
	Register(mux, &Handlers{Svc: svc, Log: logger})
	return auth.Middleware(auth.NewDev(db), mux)
}

func seedIdentityFixtures(t *testing.T, db *sql.DB) {
	t.Helper()
	statements := []string{
		`INSERT INTO departments (name) VALUES ('General'), ('Finance')`,
		`INSERT INTO roles (name) VALUES ('admin'), ('employee'), ('finance')`,
		`INSERT INTO users (email, display_name, department_id)
		 SELECT v.email, v.display_name, d.id FROM (VALUES
		     ('admin@dev.local', 'Dev Admin', 'General'),
		     ('siti@dev.local', 'Siti Rahayu', 'Finance')
		 ) AS v(email, display_name, dept)
		 JOIN departments d ON d.name = v.dept`,
		`INSERT INTO user_roles (user_id, role_id)
		 SELECT u.id, r.id FROM (VALUES
		     ('admin@dev.local', 'admin'),
		     ('siti@dev.local', 'finance'),
		     ('siti@dev.local', 'employee')
		 ) AS v(email, role)
		 JOIN users u ON u.email = v.email
		 JOIN roles r ON r.name = v.role`,
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed %q: %v", stmt, err)
		}
	}
}

func TestListUsersRequiresAdmin(t *testing.T) {
	db := testDB(t)
	seedIdentityFixtures(t, db)
	handler := newIdentityServer(t, db)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/users", nil)
	req.Header.Set("Authorization", "Bearer dev-siti@dev.local")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin list users status = %d body %s", rec.Code, rec.Body.String())
	}

	var auditCount int
	if err := db.QueryRow(`
		SELECT count(*) FROM audit_events
		WHERE action = 'identity.admin_list' AND result = 'denied'
	`).Scan(&auditCount); err != nil {
		t.Fatalf("audit count: %v", err)
	}
	if auditCount == 0 {
		t.Error("denied admin listing must be audited")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/users", nil)
	req.Header.Set("Authorization", "Bearer dev-admin@dev.local")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("admin list users status = %d body %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Users []User `json:"users"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Users) != 2 {
		t.Fatalf("users = %d, want 2", len(body.Users))
	}
	foundAdmin := false
	for _, u := range body.Users {
		if u.Email == "admin@dev.local" {
			foundAdmin = true
			if len(u.Roles) != 1 || u.Roles[0] != "admin" {
				t.Errorf("admin roles = %v", u.Roles)
			}
		}
	}
	if !foundAdmin {
		t.Error("admin user missing from listing")
	}
}

func TestListRolesAdminOnly(t *testing.T) {
	db := testDB(t)
	seedIdentityFixtures(t, db)
	handler := newIdentityServer(t, db)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/roles", nil)
	req.Header.Set("Authorization", "Bearer dev-siti@dev.local")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin roles status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/roles", nil)
	req.Header.Set("Authorization", "Bearer dev-admin@dev.local")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin roles status = %d", rec.Code)
	}
	var body struct {
		Roles []Role `json:"roles"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Roles) != 3 {
		t.Fatalf("roles = %d, want 3", len(body.Roles))
	}
}

func TestListDepartmentsAvailableToAnyAuthenticatedUser(t *testing.T) {
	db := testDB(t)
	seedIdentityFixtures(t, db)
	handler := newIdentityServer(t, db)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/departments", nil)
	req.Header.Set("Authorization", "Bearer dev-siti@dev.local")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("departments status = %d", rec.Code)
	}
	var body struct {
		Departments []Department `json:"departments"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Departments) != 2 {
		t.Fatalf("departments = %d, want 2", len(body.Departments))
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/departments", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated departments status = %d, want 401", rec.Code)
	}
}

func TestMeEndpoint(t *testing.T) {
	db := testDB(t)
	seedIdentityFixtures(t, db)
	handler := newIdentityServer(t, db)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer dev-siti@dev.local")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("me status = %d", rec.Code)
	}
	var body struct {
		Email   string   `json:"email"`
		Roles   []string `json:"roles"`
		IsAdmin bool     `json:"is_admin"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Email != "siti@dev.local" || body.IsAdmin {
		t.Errorf("me = %+v", body)
	}
	if len(body.Roles) != 2 {
		t.Errorf("roles = %v, want 2 entries", body.Roles)
	}
}
