package documents

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"office-ai-agent/apps/api/internal/audit"
	"office-ai-agent/apps/api/internal/auth"
	"office-ai-agent/apps/api/internal/authz"
	"office-ai-agent/apps/api/internal/queue"
	"office-ai-agent/apps/api/internal/storage"
)

type fakeDispatcher struct {
	mu       sync.Mutex
	messages []queue.JobMessage
}

func (f *fakeDispatcher) Dispatch(_ context.Context, msg queue.JobMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, msg)
	return nil
}

func (f *fakeDispatcher) Close() error { return nil }

func (f *fakeDispatcher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.messages)
}

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("ACL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ACL_TEST_DATABASE_URL not set; skipping database integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("database unreachable, skipping integration test: %v", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("database unreachable, skipping integration test: %v", err)
	}
	t.Cleanup(func() {
		db.Exec(`TRUNCATE document_grants, document_versions, documents, audit_events, user_roles, users, roles, departments CASCADE`)
		db.Close()
	})

	_, err = db.Exec(`TRUNCATE document_grants, document_versions, documents, audit_events, user_roles, users, roles, departments CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}

	seed := []string{
		`INSERT INTO departments (name) VALUES ('General'), ('Finance')`,
		`INSERT INTO roles (name) VALUES ('admin'), ('employee'), ('finance')`,
		`INSERT INTO users (email, display_name, department_id)
		 SELECT v.email, v.display_name, d.id FROM (VALUES
		     ('admin@dev.local', 'Dev Admin', 'General'),
		     ('budi@dev.local', 'Budi Santoso', 'General'),
		     ('siti@dev.local', 'Siti Rahayu', 'Finance'),
		     ('rian@dev.local', 'Rian Pratama', 'Finance'),
		     ('dewi@dev.local', 'Dewi Lestari', 'General')
		 ) AS v(email, display_name, dept)
		 JOIN departments d ON d.name = v.dept`,
		`INSERT INTO user_roles (user_id, role_id)
		 SELECT u.id, r.id FROM (VALUES
		     ('admin@dev.local', 'admin'),
		     ('budi@dev.local', 'employee'),
		     ('siti@dev.local', 'finance'),
		     ('siti@dev.local', 'employee'),
		     ('rian@dev.local', 'employee'),
		     ('dewi@dev.local', 'employee')
		 ) AS v(email, role)
		 JOIN users u ON u.email = v.email
		 JOIN roles r ON r.name = v.role`,
	}
	for _, stmt := range seed {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed %q: %v", stmt, err)
		}
	}
	return db
}

const testMaxUploadBytes = 1 << 20

func newTestServer(t *testing.T, db *sql.DB) (http.Handler, *fakeDispatcher) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	dispatcher := &fakeDispatcher{}
	svc := NewService(
		NewRepo(db),
		storage.NewMemory(),
		authz.NewChecker(db),
		audit.NewLogger(db),
		db,
		dispatcher,
		testMaxUploadBytes,
		time.Minute,
		"test-bucket",
		logger,
	)
	mux := http.NewServeMux()
	Register(mux, &Handlers{Svc: svc, Log: logger})
	return auth.Middleware(auth.NewDev(db), mux), dispatcher
}

var docxMagic = []byte{0x50, 0x4B, 0x03, 0x04}

func testDocxContent() []byte {
	return append(append([]byte{}, docxMagic...), []byte("word/document.xml")...)
}

func testPDFContent() []byte {
	return []byte("%PDF-1.4\n% synthetic test document\n")
}

func multipartBody(t *testing.T, fields map[string]string, filename string, content []byte) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("write field: %v", err)
		}
	}
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("write file content: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	return &buf, mw.FormDataContentType()
}

func doJSON(t *testing.T, handler http.Handler, method, target, token string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, body)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func uploadDocument(t *testing.T, handler http.Handler, token string, fields map[string]string, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	body, contentType := multipartBody(t, fields, filename, content)
	return doJSON(t, handler, http.MethodPost, "/v1/documents", token, body, contentType)
}

func baseFields(deptID string) map[string]string {
	return map[string]string{
		"title":          "Finance Policy 2026",
		"classification": "confidential",
		"department_id":  deptID,
	}
}

func fetchFinanceDeptID(t *testing.T, db *sql.DB) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`SELECT id::text FROM departments WHERE name = 'Finance'`).Scan(&id); err != nil {
		t.Fatalf("finance department lookup: %v", err)
	}
	return id
}

func decodeResponse(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("invalid JSON response %q: %v", rec.Body.String(), err)
	}
	return body
}

func TestUploadAndAccessMatrix(t *testing.T) {
	db := testDB(t)
	dept := fetchFinanceDeptID(t, db)
	handler, _ := newTestServer(t, db)

	rec := uploadDocument(t, handler, "dev-admin@dev.local", map[string]string{
		"title":          "Finance Policy 2026",
		"classification": "confidential",
		"department_id":  dept,
		"grants": `[
			{"principal_type":"department","principal_name":"Finance","action":"read"},
			{"principal_type":"department","principal_name":"Finance","action":"download"},
			{"principal_type":"role","principal_name":"finance","action":"read"},
			{"principal_type":"user","principal_name":"budi@dev.local","action":"read"}
		]`,
	}, "policy.docx", testDocxContent())
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, body %s", rec.Code, rec.Body.String())
	}
	created := decodeResponse(t, rec)
	docID, _ := created["document_id"].(string)
	if docID == "" {
		t.Fatal("document_id missing from upload response")
	}

	t.Run("admin can read", func(t *testing.T) {
		rec := doJSON(t, handler, http.MethodGet, "/v1/documents/"+docID, "dev-admin@dev.local", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("admin get status = %d body %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("granted user via user grant can read but not download", func(t *testing.T) {
		rec := doJSON(t, handler, http.MethodGet, "/v1/documents/"+docID, "dev-budi@dev.local", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("budi get status = %d body %s", rec.Code, rec.Body.String())
		}
		rec = doJSON(t, handler, http.MethodGet, "/v1/documents/"+docID+"/download", "dev-budi@dev.local", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("budi download status = %d, want 404 (read grant does not include download)", rec.Code)
		}
	})

	t.Run("finance department users can read and download", func(t *testing.T) {
		for _, token := range []string{"dev-siti@dev.local", "dev-rian@dev.local"} {
			rec := doJSON(t, handler, http.MethodGet, "/v1/documents/"+docID, token, nil, "")
			if rec.Code != http.StatusOK {
				t.Fatalf("%s get status = %d body %s", token, rec.Code, rec.Body.String())
			}
			rec = doJSON(t, handler, http.MethodGet, "/v1/documents/"+docID+"/download", token, nil, "")
			if rec.Code != http.StatusOK {
				t.Fatalf("%s download status = %d body %s", token, rec.Code, rec.Body.String())
			}
			body, _ := io.ReadAll(rec.Body)
			if !bytes.Equal(body, testDocxContent()) {
				t.Errorf("%s downloaded content differs from uploaded content", token)
			}
			if ct := rec.Header().Get("Content-Type"); ct != MimeDOCX {
				t.Errorf("%s content-type = %q", token, ct)
			}
		}
	})

	t.Run("unauthorized user gets uniform 404 everywhere", func(t *testing.T) {
		for _, path := range []string{
			"/v1/documents/" + docID,
			"/v1/documents/" + docID + "/download",
			"/v1/documents/" + docID + "/versions/" + created["version_id"].(string) + "/status",
		} {
			rec := doJSON(t, handler, http.MethodGet, path, "dev-dewi@dev.local", nil, "")
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s status = %d, want 404", path, rec.Code)
			}
		}
	})

	t.Run("missing and forbidden documents are indistinguishable", func(t *testing.T) {
		forbidden := doJSON(t, handler, http.MethodGet, "/v1/documents/"+docID, "dev-dewi@dev.local", nil, "")
		missing := doJSON(t, handler, http.MethodGet, "/v1/documents/00000000-0000-0000-0000-000000000000", "dev-dewi@dev.local", nil, "")
		if forbidden.Code != missing.Code || forbidden.Body.String() != missing.Body.String() {
			t.Error("forbidden and missing documents must return identical responses")
		}
	})

	t.Run("list respects scope and counts", func(t *testing.T) {
		rec := doJSON(t, handler, http.MethodGet, "/v1/documents", "dev-admin@dev.local", nil, "")
		adminList := decodeResponse(t, rec)
		if adminList["total"].(float64) != 1 {
			t.Errorf("admin total = %v, want 1", adminList["total"])
		}
		rec = doJSON(t, handler, http.MethodGet, "/v1/documents", "dev-dewi@dev.local", nil, "")
		dewiList := decodeResponse(t, rec)
		if dewiList["total"].(float64) != 0 {
			t.Errorf("dewi total = %v, want 0", dewiList["total"])
		}
		rec = doJSON(t, handler, http.MethodGet, "/v1/documents", "dev-siti@dev.local", nil, "")
		sitiList := decodeResponse(t, rec)
		if sitiList["total"].(float64) != 1 {
			t.Errorf("siti total = %v, want 1", sitiList["total"])
		}
	})

	t.Run("search by filename finds document for authorized user only", func(t *testing.T) {
		rec := doJSON(t, handler, http.MethodGet, "/v1/documents?query=policy", "dev-siti@dev.local", nil, "")
		if got := decodeResponse(t, rec)["total"].(float64); got != 1 {
			t.Errorf("siti search total = %v, want 1", got)
		}
		rec = doJSON(t, handler, http.MethodGet, "/v1/documents?query=policy", "dev-dewi@dev.local", nil, "")
		if got := decodeResponse(t, rec)["total"].(float64); got != 0 {
			t.Errorf("dewi search total = %v, want 0", got)
		}
	})

	t.Run("denied access attempts are audited", func(t *testing.T) {
		var count int
		if err := db.QueryRow(`
			SELECT count(*) FROM audit_events
			WHERE action = 'document.access_denied' AND result = 'denied'
		`).Scan(&count); err != nil {
			t.Fatalf("audit count: %v", err)
		}
		if count == 0 {
			t.Error("expected denied-access audit rows")
		}
	})

	t.Run("metadata is complete and storage key is opaque", func(t *testing.T) {
		var key, filename, sha string
		var size int64
		if err := db.QueryRow(`
			SELECT storage_key, original_filename, sha256, size_bytes
			FROM document_versions WHERE document_id = $1::uuid
		`, docID).Scan(&key, &filename, &sha, &size); err != nil {
			t.Fatalf("version lookup: %v", err)
		}
		if key == filename || key == "policy.docx" {
			t.Errorf("storage key must be opaque, got %q", key)
		}
		if filename != "policy.docx" {
			t.Errorf("filename = %q", filename)
		}
		sum := sha256.Sum256(testDocxContent())
		if sha != hex.EncodeToString(sum[:]) {
			t.Errorf("sha256 mismatch: %q", sha)
		}
		if size != int64(len(testDocxContent())) {
			t.Errorf("size = %d, want %d", size, len(testDocxContent()))
		}
	})
}

func TestUploadValidationFailures(t *testing.T) {
	db := testDB(t)
	dept := fetchFinanceDeptID(t, db)
	handler, _ := newTestServer(t, db)

	t.Run("non-admin cannot upload", func(t *testing.T) {
		rec := uploadDocument(t, handler, "dev-budi@dev.local", baseFields(dept), "policy.pdf", testPDFContent())
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", rec.Code)
		}
	})

	t.Run("content not matching extension is rejected", func(t *testing.T) {
		rec := uploadDocument(t, handler, "dev-admin@dev.local", baseFields(dept), "policy.docx", testPDFContent())
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Errorf("status = %d, want 415", rec.Code)
		}
	})

	t.Run("unsupported extension is rejected", func(t *testing.T) {
		rec := uploadDocument(t, handler, "dev-admin@dev.local", baseFields(dept), "policy.exe", testPDFContent())
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Errorf("status = %d, want 415", rec.Code)
		}
	})

	t.Run("oversized file is rejected", func(t *testing.T) {
		big := make([]byte, testMaxUploadBytes+10)
		copy(big, testPDFContent())
		rec := uploadDocument(t, handler, "dev-admin@dev.local", baseFields(dept), "big.pdf", big)
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("status = %d, want 413", rec.Code)
		}
	})

	t.Run("unknown department is rejected", func(t *testing.T) {
		fields := baseFields(dept)
		fields["department_id"] = "00000000-0000-0000-0000-000000000000"
		rec := uploadDocument(t, handler, "dev-admin@dev.local", fields, "policy.pdf", testPDFContent())
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("bad classification is rejected", func(t *testing.T) {
		fields := baseFields(dept)
		fields["classification"] = "public"
		rec := uploadDocument(t, handler, "dev-admin@dev.local", fields, "policy.pdf", testPDFContent())
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("malformed grants JSON is rejected", func(t *testing.T) {
		fields := baseFields(dept)
		fields["grants"] = "not-json"
		rec := uploadDocument(t, handler, "dev-admin@dev.local", fields, "policy.pdf", testPDFContent())
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("unknown grant principal is rejected", func(t *testing.T) {
		fields := baseFields(dept)
		fields["grants"] = `[{"principal_type":"role","principal_name":"interns","action":"read"}]`
		rec := uploadDocument(t, handler, "dev-admin@dev.local", fields, "policy.pdf", testPDFContent())
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("upload without auth is rejected", func(t *testing.T) {
		body, contentType := multipartBody(t, baseFields(dept), "policy.pdf", testPDFContent())
		rec := doJSON(t, handler, http.MethodPost, "/v1/documents", "", body, contentType)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})
}

func TestGrantRevocationIsImmediate(t *testing.T) {
	db := testDB(t)
	dept := fetchFinanceDeptID(t, db)
	handler, _ := newTestServer(t, db)

	fields := baseFields(dept)
	fields["grants"] = `[{"principal_type":"user","principal_name":"dewi@dev.local","action":"download"}]`
	rec := uploadDocument(t, handler, "dev-admin@dev.local", fields, "policy.pdf", testPDFContent())
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d body %s", rec.Code, rec.Body.String())
	}
	docID := decodeResponse(t, rec)["document_id"].(string)

	if rec := doJSON(t, handler, http.MethodGet, "/v1/documents/"+docID+"/download", "dev-dewi@dev.local", nil, ""); rec.Code != http.StatusOK {
		t.Fatalf("pre-revocation download status = %d", rec.Code)
	}

	if _, err := db.Exec(`DELETE FROM document_grants`); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	if rec := doJSON(t, handler, http.MethodGet, "/v1/documents/"+docID+"/download", "dev-dewi@dev.local", nil, ""); rec.Code != http.StatusNotFound {
		t.Errorf("post-revocation download status = %d, want 404", rec.Code)
	}
}

func TestPresignedModeWithMemoryBackendIsRejected(t *testing.T) {
	db := testDB(t)
	dept := fetchFinanceDeptID(t, db)
	handler, _ := newTestServer(t, db)

	rec := uploadDocument(t, handler, "dev-admin@dev.local", baseFields(dept), "policy.pdf", testPDFContent())
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d", rec.Code)
	}
	docID := decodeResponse(t, rec)["document_id"].(string)

	rec = doJSON(t, handler, http.MethodGet, "/v1/documents/"+docID+"/download?mode=presigned", "dev-admin@dev.local", nil, "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestVersionStatusEndpoint(t *testing.T) {
	db := testDB(t)
	dept := fetchFinanceDeptID(t, db)
	handler, _ := newTestServer(t, db)

	fields := baseFields(dept)
	fields["grants"] = `[{"principal_type":"department","principal_name":"Finance","action":"read"}]`
	rec := uploadDocument(t, handler, "dev-admin@dev.local", fields, "policy.pdf", testPDFContent())
	created := decodeResponse(t, rec)
	docID := created["document_id"].(string)
	versionID := created["version_id"].(string)

	rec = doJSON(t, handler, http.MethodGet,
		"/v1/documents/"+docID+"/versions/"+versionID+"/status", "dev-siti@dev.local", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status endpoint = %d body %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body["status"] != "queued" {
		t.Errorf("status = %v, want queued (dispatched via outbox)", body["status"])
	}

	rec = doJSON(t, handler, http.MethodGet,
		"/v1/documents/"+docID+"/versions/00000000-0000-0000-0000-000000000000/status", "dev-admin@dev.local", nil, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown version status = %d, want 404", rec.Code)
	}

	rec = doJSON(t, handler, http.MethodGet,
		"/v1/documents/"+docID+"/versions/"+versionID+"/status", "dev-dewi@dev.local", nil, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("unauthorized version status = %d, want 404", rec.Code)
	}
}

func TestUploadEnqueuesIngestionJob(t *testing.T) {
	db := testDB(t)
	dept := fetchFinanceDeptID(t, db)
	handler, dispatcher := newTestServer(t, db)

	fields := baseFields(dept)
	fields["grants"] = `[{"principal_type":"department","principal_name":"Finance","action":"read"}]`
	rec := uploadDocument(t, handler, "dev-admin@dev.local", fields, "policy.pdf", testPDFContent())
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d body %s", rec.Code, rec.Body.String())
	}
	created := decodeResponse(t, rec)
	docID := created["document_id"].(string)
	versionID := created["version_id"].(string)

	if dispatcher.count() != 1 {
		t.Fatalf("dispatcher messages = %d, want 1", dispatcher.count())
	}
	msg := dispatcher.messages[0]
	if msg.JobType != queue.JobTypeIngest || msg.DocumentID != docID || msg.DocumentVersionID != versionID {
		t.Errorf("unexpected message %+v", msg)
	}
	if msg.StorageKey == "" || msg.StorageBucket != "test-bucket" {
		t.Errorf("message storage fields incomplete: %+v", msg)
	}

	var jobStatus, outboxStatus string
	if err := db.QueryRow(`
		SELECT j.status, o.status FROM ingestion_jobs j
		JOIN ingestion_outbox o ON o.job_id = j.id
		WHERE j.document_version_id = $1::uuid
	`, versionID).Scan(&jobStatus, &outboxStatus); err != nil {
		t.Fatalf("job/outbox lookup: %v", err)
	}
	if jobStatus != "queued" || outboxStatus != "dispatched" {
		t.Errorf("job=%s outbox=%s, want queued/dispatched", jobStatus, outboxStatus)
	}

	var versionStatus string
	if err := db.QueryRow(`SELECT status FROM document_versions WHERE id = $1::uuid`, versionID).Scan(&versionStatus); err != nil {
		t.Fatalf("version lookup: %v", err)
	}
	if versionStatus != "queued" {
		t.Errorf("version status = %s, want queued", versionStatus)
	}
}

func TestReindexEndpoint(t *testing.T) {
	db := testDB(t)
	dept := fetchFinanceDeptID(t, db)
	handler, dispatcher := newTestServer(t, db)

	fields := baseFields(dept)
	fields["grants"] = `[{"principal_type":"user","principal_name":"budi@dev.local","action":"read"}]`
	rec := uploadDocument(t, handler, "dev-admin@dev.local", fields, "policy.pdf", testPDFContent())
	created := decodeResponse(t, rec)
	docID := created["document_id"].(string)
	if dispatcher.count() != 1 {
		t.Fatalf("initial dispatch count = %d", dispatcher.count())
	}

	t.Run("non-granted user gets uniform 404", func(t *testing.T) {
		rec := doJSON(t, handler, http.MethodPost, "/v1/documents/"+docID+"/reindex", "dev-dewi@dev.local", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})

	t.Run("reader without manage rights gets 404", func(t *testing.T) {
		rec := doJSON(t, handler, http.MethodPost, "/v1/documents/"+docID+"/reindex", "dev-budi@dev.local", nil, "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})

	t.Run("admin reindex while job is active is deduplicated", func(t *testing.T) {
		rec := doJSON(t, handler, http.MethodPost, "/v1/documents/"+docID+"/reindex", "dev-admin@dev.local", nil, "")
		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d body %s", rec.Code, rec.Body.String())
		}
		if got := decodeResponse(t, rec)["deduplicated"]; got != true {
			t.Errorf("deduplicated = %v, want true (active ingest job)", got)
		}
		if dispatcher.count() != 1 {
			t.Errorf("dispatcher count = %d, want 1 (no new dispatch)", dispatcher.count())
		}
	})

	t.Run("reindex after completion schedules a new job", func(t *testing.T) {
		if _, err := db.Exec(`UPDATE ingestion_jobs SET status = 'ready', finished_at = now()`); err != nil {
			t.Fatalf("complete jobs: %v", err)
		}
		if _, err := db.Exec(`UPDATE document_versions SET status = 'ready'`); err != nil {
			t.Fatalf("complete version: %v", err)
		}
		rec := doJSON(t, handler, http.MethodPost, "/v1/documents/"+docID+"/reindex", "dev-admin@dev.local", nil, "")
		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d body %s", rec.Code, rec.Body.String())
		}
		body := decodeResponse(t, rec)
		if body["deduplicated"] != false {
			t.Errorf("deduplicated = %v, want false", body["deduplicated"])
		}
		if dispatcher.count() != 2 {
			t.Errorf("dispatcher count = %d, want 2", dispatcher.count())
		}
	})
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"../../etc/passwd", "passwd"},
		{"..\\..\\win.ini", "win.ini"},
		{"policy\x00.pdf", "policy.pdf"},
		{".", ""},
		{"..", ""},
		{"  report final.pdf  ", "report final.pdf"},
	}
	for _, tt := range tests {
		if got := SanitizeFilename(tt.in); got != tt.want {
			t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	if got := SanitizeFilename(strings.Repeat("a", 300) + ".pdf"); len(got) != 255 {
		t.Errorf("long filename length = %d, want 255", len(got))
	}
}

func TestDetectMimeType(t *testing.T) {
	if _, err := DetectMimeType("a.pdf", testPDFContent()); err != nil {
		t.Errorf("valid pdf rejected: %v", err)
	}
	if _, err := DetectMimeType("a.xlsx", append(append([]byte{}, docxMagic...), []byte("xl/worksheets")...)); err != nil {
		t.Errorf("valid xlsx rejected: %v", err)
	}
	if _, err := DetectMimeType("a.docx", testDocxContent()); err != nil {
		t.Errorf("valid docx rejected: %v", err)
	}
	if _, err := DetectMimeType("a.txt", testPDFContent()); err == nil {
		t.Error("txt extension must be rejected")
	}
	if _, err := DetectMimeType("a.pdf", testDocxContent()); err == nil {
		t.Error("zip content with .pdf extension must be rejected")
	}
}

func TestEscapeLike(t *testing.T) {
	if got := EscapeLike(`50%_off\`); got != `50\%\_off\\` {
		t.Errorf("EscapeLike() = %q", got)
	}
}
