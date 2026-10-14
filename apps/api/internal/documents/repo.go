package documents

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"office-ai-agent/apps/api/internal/authz"
	"office-ai-agent/apps/api/internal/queue"
)

var ErrNotFound = errors.New("documents: not found")

type Document struct {
	ID                 string    `json:"id"`
	Title              string    `json:"title"`
	Classification     string    `json:"classification"`
	OwningDepartmentID string    `json:"department_id"`
	CreatedBy          string    `json:"-"`
	CreatedAt          time.Time `json:"created_at"`
	VersionID          string    `json:"version_id"`
	Status             string    `json:"status"`
	Filename           string    `json:"filename"`
	MimeType           string    `json:"-"`
	SizeBytes          int64     `json:"size_bytes"`
	StorageBucket      string    `json:"-"`
	StorageKey         string    `json:"-"`
}

type GrantInput struct {
	PrincipalType string
	PrincipalID   string
	Action        string
}

type CreateInput struct {
	Title              string
	Classification     string
	OwningDepartmentID string
	CreatedBy          string
	Filename           string
	MimeType           string
	SHA256             string
	StorageBucket      string
	StorageKey         string
	SizeBytes          int64
	Grants             []GrantInput
}

type ListParams struct {
	Query    string
	Status   string
	Page     int
	PageSize int
}

type ListResult struct {
	Items    []Document `json:"items"`
	Total    int64      `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
}

type Repo struct {
	db *sql.DB
}

func NewRepo(db *sql.DB) *Repo {
	return &Repo{db: db}
}

type CreateResult struct {
	Document Document
	JobID    string
}

func (r *Repo) Create(ctx context.Context, in CreateInput) (CreateResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return CreateResult{}, err
	}
	defer tx.Rollback()

	var docID, versionID, jobID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO documents (title, classification, owning_department_id, created_by)
		VALUES ($1, $2, $3::uuid, $4::uuid)
		RETURNING id::text
	`, in.Title, in.Classification, in.OwningDepartmentID, in.CreatedBy).Scan(&docID)
	if err != nil {
		return CreateResult{}, fmt.Errorf("documents: insert document: %w", err)
	}

	err = tx.QueryRowContext(ctx, `
		INSERT INTO document_versions
		    (document_id, version_number, original_filename, mime_type, size_bytes, sha256, storage_bucket, storage_key, status)
		VALUES ($1::uuid, 1, $2, $3, $4, $5, $6, $7, 'uploaded')
		RETURNING id::text
	`, docID, in.Filename, in.MimeType, in.SizeBytes, in.SHA256, in.StorageBucket, in.StorageKey).Scan(&versionID)
	if err != nil {
		return CreateResult{}, fmt.Errorf("documents: insert version: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE documents SET current_version_id = $1::uuid WHERE id = $2::uuid`,
		versionID, docID,
	); err != nil {
		return CreateResult{}, fmt.Errorf("documents: link current version: %w", err)
	}

	err = tx.QueryRowContext(ctx, `
		INSERT INTO ingestion_jobs (document_version_id, job_type, status)
		VALUES ($1::uuid, 'ingest', 'queued')
		RETURNING id::text
	`, versionID).Scan(&jobID)
	if err != nil {
		return CreateResult{}, fmt.Errorf("documents: insert ingestion job: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ingestion_outbox (job_id, status) VALUES ($1::uuid, 'pending')
	`, jobID); err != nil {
		return CreateResult{}, fmt.Errorf("documents: insert outbox: %w", err)
	}

	for _, g := range in.Grants {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO document_grants (document_id, principal_type, principal_id, action)
			VALUES ($1::uuid, $2, $3::uuid, $4)
			ON CONFLICT DO NOTHING
		`, docID, g.PrincipalType, g.PrincipalID, g.Action); err != nil {
			return CreateResult{}, fmt.Errorf("documents: insert grant: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return CreateResult{}, err
	}

	return CreateResult{
		Document: Document{
			ID:                 docID,
			Title:              in.Title,
			Classification:     in.Classification,
			OwningDepartmentID: in.OwningDepartmentID,
			CreatedBy:          in.CreatedBy,
			VersionID:          versionID,
			Status:             "uploaded",
			Filename:           in.Filename,
			MimeType:           in.MimeType,
			SizeBytes:          in.SizeBytes,
			StorageBucket:      in.StorageBucket,
			StorageKey:         in.StorageKey,
		},
		JobID: jobID,
	}, nil
}

func (r *Repo) MarkOutboxDispatched(ctx context.Context, jobID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE ingestion_outbox SET status = 'dispatched', dispatched_at = now()
		WHERE job_id = $1::uuid AND status = 'pending'
	`, jobID)
	return err
}

func (r *Repo) SetVersionStatus(ctx context.Context, versionID, status string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE document_versions SET status = $2 WHERE id = $1::uuid
	`, versionID, status)
	return err
}

func (r *Repo) PendingOutboxJobs(ctx context.Context, limit int) ([]queue.JobMessage, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT j.id::text, j.job_type, v.id::text, d.id::text, v.version_number, v.storage_bucket, v.storage_key
		FROM ingestion_outbox o
		JOIN ingestion_jobs j ON j.id = o.job_id
		JOIN document_versions v ON v.id = j.document_version_id
		JOIN documents d ON d.id = v.document_id
		WHERE o.status = 'pending' AND j.status = 'queued'
		ORDER BY o.created_at
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("documents: pending outbox: %w", err)
	}
	defer rows.Close()

	msgs := []queue.JobMessage{}
	for rows.Next() {
		var m queue.JobMessage
		var jobType string
		if err := rows.Scan(&m.JobID, &jobType, &m.DocumentVersionID,
			&m.DocumentID, &m.VersionNumber, &m.StorageBucket, &m.StorageKey); err != nil {
			return nil, fmt.Errorf("documents: pending outbox scan: %w", err)
		}
		if jobType == "reindex" {
			m.JobType = queue.JobTypeReindex
		} else {
			m.JobType = queue.JobTypeIngest
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

func (r *Repo) CreateReindexJob(ctx context.Context, documentID, versionID string) (jobID string, deduped bool, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback()

	err = tx.QueryRowContext(ctx, `
		INSERT INTO ingestion_jobs (document_version_id, job_type, status)
		VALUES ($1::uuid, 'reindex', 'queued')
		RETURNING id::text
	`, versionID).Scan(&jobID)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			active, aerr := r.activeJobForVersion(ctx, versionID)
			if aerr != nil {
				return "", false, aerr
			}
			return active, true, nil
		}
		return "", false, fmt.Errorf("documents: insert reindex job: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ingestion_outbox (job_id, status) VALUES ($1::uuid, 'pending')
	`, jobID); err != nil {
		return "", false, fmt.Errorf("documents: insert reindex outbox: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", false, err
	}
	return jobID, false, nil
}

func (r *Repo) ActiveJobID(ctx context.Context, versionID string) (string, error) {
	return r.activeJobForVersion(ctx, versionID)
}

func (r *Repo) activeJobForVersion(ctx context.Context, versionID string) (string, error) {
	var id string
	err := r.db.QueryRowContext(ctx, `
		SELECT id::text FROM ingestion_jobs
		WHERE document_version_id = $1::uuid AND status IN ('queued', 'processing')
		LIMIT 1
	`, versionID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("documents: active job disappeared")
	}
	return id, err
}

func (r *Repo) Get(ctx context.Context, scope authz.Scope, action, id string) (Document, error) {
	args := []any{id}
	where := []string{"d.id = $1::uuid", "d.deleted_at IS NULL"}
	where = authz.AppendScopeClause(where, &args, scope, action, "d.id")

	row := r.db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT d.id::text, d.title, d.classification, d.owning_department_id::text, d.created_by::text, d.created_at,
		       COALESCE(v.id::text, ''), COALESCE(v.status, ''), COALESCE(v.original_filename, ''),
		       COALESCE(v.mime_type, ''), COALESCE(v.size_bytes, 0),
		       COALESCE(v.storage_bucket, ''), COALESCE(v.storage_key, '')
		FROM documents d
		LEFT JOIN document_versions v ON v.id = d.current_version_id
		WHERE %s
	`, strings.Join(where, " AND ")), args...)

	var doc Document
	err := row.Scan(
		&doc.ID, &doc.Title, &doc.Classification, &doc.OwningDepartmentID, &doc.CreatedBy, &doc.CreatedAt,
		&doc.VersionID, &doc.Status, &doc.Filename,
		&doc.MimeType, &doc.SizeBytes,
		&doc.StorageBucket, &doc.StorageKey,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Document{}, ErrNotFound
	}
	if err != nil {
		return Document{}, fmt.Errorf("documents: get: %w", err)
	}
	return doc, nil
}

func (r *Repo) List(ctx context.Context, scope authz.Scope, p ListParams) (ListResult, error) {
	args := []any{}
	where := []string{"d.deleted_at IS NULL"}
	where = authz.AppendScopeClause(where, &args, scope, "read", "d.id")

	if q := p.Query; q != "" {
		args = append(args, "%"+EscapeLike(q)+"%")
		idx := len(args)
		where = append(where,
			fmt.Sprintf(`(d.title ILIKE $%d ESCAPE '\' OR v.original_filename ILIKE $%d ESCAPE '\')`, idx, idx))
	}
	if p.Status != "" {
		args = append(args, p.Status)
		where = append(where, fmt.Sprintf("v.status = $%d", len(args)))
	}
	whereSQL := strings.Join(where, " AND ")

	limitIdx := len(args) + 1
	offsetIdx := len(args) + 2
	limitArgs := append(args, p.PageSize, (p.Page-1)*p.PageSize)

	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT d.id::text, d.title, d.classification, d.owning_department_id::text, d.created_at,
		       COALESCE(v.id::text, ''), COALESCE(v.status, ''), COALESCE(v.original_filename, ''),
		       COALESCE(v.size_bytes, 0)
		FROM documents d
		LEFT JOIN document_versions v ON v.id = d.current_version_id
		WHERE %s
		ORDER BY d.created_at DESC, d.id
		LIMIT $%d OFFSET $%d
	`, whereSQL, limitIdx, offsetIdx), limitArgs...)
	if err != nil {
		return ListResult{}, fmt.Errorf("documents: list: %w", err)
	}
	defer rows.Close()

	items := []Document{}
	for rows.Next() {
		var doc Document
		if err := rows.Scan(
			&doc.ID, &doc.Title, &doc.Classification, &doc.OwningDepartmentID, &doc.CreatedAt,
			&doc.VersionID, &doc.Status, &doc.Filename, &doc.SizeBytes,
		); err != nil {
			return ListResult{}, fmt.Errorf("documents: list scan: %w", err)
		}
		items = append(items, doc)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, fmt.Errorf("documents: list rows: %w", err)
	}

	var total int64
	err = r.db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM documents d
		LEFT JOIN document_versions v ON v.id = d.current_version_id
		WHERE `+whereSQL, args...).Scan(&total)
	if err != nil {
		return ListResult{}, fmt.Errorf("documents: list count: %w", err)
	}

	return ListResult{Items: items, Total: total, Page: p.Page, PageSize: p.PageSize}, nil
}

func (r *Repo) VersionStatus(ctx context.Context, scope authz.Scope, documentID, versionID string) (string, error) {
	args := []any{documentID, versionID}
	where := []string{"d.id = $1::uuid", "d.deleted_at IS NULL", "v.id = $2::uuid"}
	where = authz.AppendScopeClause(where, &args, scope, "read", "d.id")

	var status string
	err := r.db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT v.status
		FROM documents d
		JOIN document_versions v ON v.document_id = d.id
		WHERE %s
	`, strings.Join(where, " AND ")), args...).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("documents: version status: %w", err)
	}
	return status, nil
}

func EscapeLike(q string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(q)
}
