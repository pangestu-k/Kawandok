package documents

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"office-ai-agent/apps/api/internal/audit"
	"office-ai-agent/apps/api/internal/auth"
	"office-ai-agent/apps/api/internal/authz"
	"office-ai-agent/apps/api/internal/httpx"
	"office-ai-agent/apps/api/internal/queue"
	"office-ai-agent/apps/api/internal/storage"
)

const (
	ClassificationInternal     = "internal"
	ClassificationConfidential = "confidential"
	ClassificationRestricted   = "restricted"

	MimePDF  = "application/pdf"
	MimeDOCX = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	MimeXLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
)

var mimeByExtension = map[string]string{
	".pdf":  MimePDF,
	".docx": MimeDOCX,
	".xlsx": MimeXLSX,
}

var validClassifications = map[string]bool{
	ClassificationInternal:     true,
	ClassificationConfidential: true,
	ClassificationRestricted:   true,
}

var validActions = map[string]bool{"read": true, "download": true, "manage": true}

var validPrincipalTypes = map[string]bool{"user": true, "role": true, "department": true}

type Service struct {
	repo       *Repo
	store      storage.Storage
	checker    *authz.Checker
	auditLog   *audit.Logger
	db         *sql.DB
	dispatcher queue.Dispatcher
	maxBytes   int64
	presignTTL time.Duration
	bucket     string
	log        *slog.Logger
}

func NewService(
	repo *Repo,
	store storage.Storage,
	checker *authz.Checker,
	auditLog *audit.Logger,
	db *sql.DB,
	dispatcher queue.Dispatcher,
	maxBytes int64,
	presignTTL time.Duration,
	bucket string,
	log *slog.Logger,
) *Service {
	if dispatcher == nil {
		dispatcher = queue.Nop{}
	}
	return &Service{
		repo:       repo,
		store:      store,
		checker:    checker,
		auditLog:   auditLog,
		db:         db,
		dispatcher: dispatcher,
		maxBytes:   maxBytes,
		presignTTL: presignTTL,
		bucket:     bucket,
		log:        log,
	}
}

func (s *Service) MaxBytes() int64 {
	return s.maxBytes
}

type UploadResult struct {
	Document Document `json:"-"`
}

func (s *Service) Upload(
	ctx context.Context,
	p auth.Principal,
	title, classification, departmentID, filename string,
	data []byte,
	grantsJSON []byte,
	requestID string,
) (Document, error) {
	if !p.IsAdmin() {
		return Document{}, httpx.NewAPIError(http.StatusForbidden, "FORBIDDEN", "upload requires the admin role")
	}

	title = strings.TrimSpace(title)
	if title == "" {
		return Document{}, httpx.ErrBadRequest("title is required")
	}
	if len(title) > 512 {
		return Document{}, httpx.ErrBadRequest("title must be at most 512 characters")
	}
	if !validClassifications[classification] {
		return Document{}, httpx.ErrBadRequest("classification must be internal, confidential or restricted")
	}
	if departmentID == "" {
		return Document{}, httpx.ErrBadRequest("department_id is required")
	}
	if err := s.departmentExists(ctx, departmentID); err != nil {
		return Document{}, err
	}
	if len(data) == 0 {
		return Document{}, httpx.ErrBadRequest("file is empty")
	}
	if int64(len(data)) > s.maxBytes {
		return Document{}, httpx.NewAPIError(http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE",
			fmt.Sprintf("file exceeds the maximum size of %d bytes", s.maxBytes))
	}

	filename = SanitizeFilename(filename)
	mimeType, err := DetectMimeType(filename, data)
	if err != nil {
		return Document{}, err
	}

	grants, err := s.resolveGrants(ctx, grantsJSON)
	if err != nil {
		return Document{}, err
	}

	sum := sha256.Sum256(data)
	storageKey := uuid.NewString()

	if err := s.store.Put(ctx, storageKey, data, mimeType); err != nil {
		s.recordAudit(ctx, p.ID, "document.upload", "document", "", "error", requestID)
		s.log.Error("storage put failed", "error", err)
		return Document{}, httpx.NewAPIError(http.StatusBadGateway, "STORAGE_UNAVAILABLE", "object storage is unavailable")
	}

	created, err := s.repo.Create(ctx, CreateInput{
		Title:              title,
		Classification:     classification,
		OwningDepartmentID: departmentID,
		CreatedBy:          p.ID,
		Filename:           filename,
		MimeType:           mimeType,
		SHA256:             hex.EncodeToString(sum[:]),
		StorageBucket:      s.bucket,
		StorageKey:         storageKey,
		SizeBytes:          int64(len(data)),
		Grants:             grants,
	})
	if err != nil {
		s.recordAudit(ctx, p.ID, "document.upload", "document", "", "error", requestID)
		s.log.Error("document insert failed", "error", err)
		return Document{}, httpx.NewAPIError(http.StatusInternalServerError, "INTERNAL", "could not store document metadata")
	}

	doc := created.Document
	doc.Status = "queued"
	s.dispatchJob(ctx, queue.JobMessage{
		JobID:             created.JobID,
		JobType:           queue.JobTypeIngest,
		DocumentID:        doc.ID,
		DocumentVersionID: doc.VersionID,
		VersionNumber:     1,
		StorageBucket:     doc.StorageBucket,
		StorageKey:        doc.StorageKey,
	})

	s.recordAudit(ctx, p.ID, "document.upload", "document", doc.ID, "allowed", requestID)
	return doc, nil
}

func (s *Service) dispatchJob(ctx context.Context, msg queue.JobMessage) {
	msg.IdempotencyKey = msg.JobID
	if err := s.dispatcher.Dispatch(ctx, msg); err != nil {
		s.log.Warn("job dispatch failed; outbox sweep will retry", "job_id", msg.JobID, "error", err)
		return
	}
	if err := s.repo.MarkOutboxDispatched(ctx, msg.JobID); err != nil {
		s.log.Warn("marking outbox dispatched failed", "job_id", msg.JobID, "error", err)
	}
	if err := s.repo.SetVersionStatus(ctx, msg.DocumentVersionID, "queued"); err != nil {
		s.log.Warn("marking version queued failed", "version_id", msg.DocumentVersionID, "error", err)
	}
}

func (s *Service) SweepPending(ctx context.Context) (int, error) {
	msgs, err := s.repo.PendingOutboxJobs(ctx, 100)
	if err != nil {
		return 0, err
	}
	dispatched := 0
	for _, msg := range msgs {
		s.dispatchJob(ctx, msg)
		dispatched++
	}
	return dispatched, nil
}

func (s *Service) Reindex(ctx context.Context, p auth.Principal, documentID, requestID string) (string, bool, error) {
	doc, err := s.repo.Get(ctx, authz.ScopeFor(p), "manage", documentID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			s.recordDenied(ctx, p.ID, documentID, requestID)
			return "", false, httpx.ErrNotFound()
		}
		return "", false, httpx.NewAPIError(http.StatusInternalServerError, "INTERNAL", "document lookup failed")
	}
	if doc.VersionID == "" {
		return "", false, httpx.ErrNotFound()
	}
	if doc.Status == "queued" || doc.Status == "processing" {
		jobID, err := s.repo.ActiveJobID(ctx, doc.VersionID)
		if err != nil {
			return "", false, httpx.NewAPIError(http.StatusInternalServerError, "INTERNAL", "job lookup failed")
		}
		return jobID, true, nil
	}

	jobID, deduped, err := s.repo.CreateReindexJob(ctx, doc.ID, doc.VersionID)
	if err != nil {
		s.log.Error("reindex job creation failed", "error", err)
		return "", false, httpx.NewAPIError(http.StatusInternalServerError, "INTERNAL", "could not schedule reindex")
	}
	if !deduped {
		s.dispatchJob(ctx, queue.JobMessage{
			JobID:             jobID,
			JobType:           queue.JobTypeReindex,
			DocumentID:        doc.ID,
			DocumentVersionID: doc.VersionID,
			VersionNumber:     1,
			StorageBucket:     doc.StorageBucket,
			StorageKey:        doc.StorageKey,
		})
	}
	s.recordAudit(ctx, p.ID, "document.reindex", "document", doc.ID, "allowed", requestID)
	return jobID, deduped, nil
}

func (s *Service) Get(ctx context.Context, p auth.Principal, id, requestID string) (Document, error) {
	doc, err := s.repo.Get(ctx, authz.ScopeFor(p), "read", id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			s.recordDenied(ctx, p.ID, id, requestID)
			return Document{}, httpx.ErrNotFound()
		}
		return Document{}, httpx.NewAPIError(http.StatusInternalServerError, "INTERNAL", "document lookup failed")
	}
	return doc, nil
}

func (s *Service) List(ctx context.Context, p auth.Principal, params ListParams) (ListResult, error) {
	params = normalizeListParams(params)
	res, err := s.repo.List(ctx, authz.ScopeFor(p), params)
	if err != nil {
		return ListResult{}, httpx.NewAPIError(http.StatusInternalServerError, "INTERNAL", "document listing failed")
	}
	return res, nil
}

func (s *Service) Status(ctx context.Context, p auth.Principal, documentID, versionID, requestID string) (string, error) {
	status, err := s.repo.VersionStatus(ctx, authz.ScopeFor(p), documentID, versionID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			s.recordDenied(ctx, p.ID, documentID, requestID)
			return "", httpx.ErrNotFound()
		}
		return "", httpx.NewAPIError(http.StatusInternalServerError, "INTERNAL", "status lookup failed")
	}
	return status, nil
}

type Download struct {
	Reader       io.ReadCloser
	Size         int64
	Filename     string
	MimeType     string
	PresignedURL string
}

func (s *Service) Download(ctx context.Context, p auth.Principal, id string, presigned bool, requestID string) (Download, error) {
	can, err := s.checker.Can(ctx, p, "download", id)
	if err != nil {
		s.log.Error("authz check failed", "error", err)
		return Download{}, httpx.NewAPIError(http.StatusInternalServerError, "INTERNAL", "authorization check failed")
	}
	if !can {
		s.recordDenied(ctx, p.ID, id, requestID)
		return Download{}, httpx.ErrNotFound()
	}

	doc, err := s.repo.Get(ctx, authz.ScopeFor(p), "download", id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			s.recordDenied(ctx, p.ID, id, requestID)
			return Download{}, httpx.ErrNotFound()
		}
		return Download{}, httpx.NewAPIError(http.StatusInternalServerError, "INTERNAL", "document lookup failed")
	}

	if presigned {
		url, err := s.store.PresignGet(ctx, doc.StorageKey, s.presignTTL)
		if errors.Is(err, storage.ErrPresignUnsupported) {
			return Download{}, httpx.ErrBadRequest("this storage backend does not support presigned URLs")
		}
		if err != nil {
			s.log.Error("presign failed", "error", err)
			return Download{}, httpx.NewAPIError(http.StatusBadGateway, "STORAGE_UNAVAILABLE", "could not sign download URL")
		}
		s.recordAudit(ctx, p.ID, "document.download", "document", doc.ID, "allowed", requestID)
		return Download{PresignedURL: url, Filename: doc.Filename}, nil
	}

	rc, size, err := s.store.Get(ctx, doc.StorageKey)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			s.log.Error("storage object missing for document", "document_id", doc.ID, "key", doc.StorageKey)
			return Download{}, httpx.NewAPIError(http.StatusInternalServerError, "INTERNAL", "stored object is missing")
		}
		s.log.Error("storage get failed", "error", err)
		return Download{}, httpx.NewAPIError(http.StatusBadGateway, "STORAGE_UNAVAILABLE", "object storage is unavailable")
	}

	s.recordAudit(ctx, p.ID, "document.download", "document", doc.ID, "allowed", requestID)
	return Download{Reader: rc, Size: size, Filename: doc.Filename, MimeType: doc.MimeType}, nil
}

type grantSpec struct {
	PrincipalType string `json:"principal_type"`
	PrincipalID   string `json:"principal_id"`
	PrincipalName string `json:"principal_name"`
	Action        string `json:"action"`
}

func (s *Service) resolveGrants(ctx context.Context, raw []byte) ([]GrantInput, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	var specs []grantSpec
	if err := json.Unmarshal(raw, &specs); err != nil {
		return nil, httpx.ErrBadRequest("grants must be a JSON array of {principal_type, principal_id|principal_name, action}")
	}

	seen := map[string]bool{}
	grants := make([]GrantInput, 0, len(specs))
	for _, spec := range specs {
		if !validPrincipalTypes[spec.PrincipalType] {
			return nil, httpx.ErrBadRequest("grant principal_type must be user, role or department")
		}
		if !validActions[spec.Action] {
			return nil, httpx.ErrBadRequest("grant action must be read, download or manage")
		}
		id := spec.PrincipalID
		if id == "" {
			resolved, err := s.resolvePrincipal(ctx, spec.PrincipalType, spec.PrincipalName)
			if err != nil {
				return nil, err
			}
			id = resolved
		} else if _, err := uuid.Parse(id); err != nil {
			return nil, httpx.ErrBadRequest("grant principal_id must be a UUID")
		}
		key := spec.PrincipalType + "|" + id + "|" + spec.Action
		if seen[key] {
			continue
		}
		seen[key] = true
		grants = append(grants, GrantInput{PrincipalType: spec.PrincipalType, PrincipalID: id, Action: spec.Action})
	}
	return grants, nil
}

func (s *Service) resolvePrincipal(ctx context.Context, principalType, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", httpx.ErrBadRequest("grant requires principal_id or principal_name")
	}
	if len(name) > 254 {
		return "", httpx.ErrBadRequest("grant principal_name is too long")
	}

	var (
		query string
		id    string
	)
	switch principalType {
	case "role":
		query = "SELECT id::text FROM roles WHERE name = $1"
	case "department":
		query = "SELECT id::text FROM departments WHERE name = $1"
	case "user":
		query = "SELECT id::text FROM users WHERE email = $1 AND status = 'active'"
	default:
		return "", httpx.ErrBadRequest("grant principal_type must be user, role or department")
	}

	err := s.db.QueryRowContext(ctx, query, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", httpx.ErrBadRequest(fmt.Sprintf("unknown grant principal %q", name))
	}
	if err != nil {
		s.log.Error("grant principal lookup failed", "error", err)
		return "", httpx.NewAPIError(http.StatusInternalServerError, "INTERNAL", "grant resolution failed")
	}
	return id, nil
}

func (s *Service) departmentExists(ctx context.Context, departmentID string) error {
	if _, err := uuid.Parse(departmentID); err != nil {
		return httpx.ErrBadRequest("department_id must be a UUID")
	}
	var one int
	err := s.db.QueryRowContext(ctx, "SELECT 1 FROM departments WHERE id = $1::uuid", departmentID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return httpx.ErrBadRequest("unknown department_id")
	}
	if err != nil {
		s.log.Error("department lookup failed", "error", err)
		return httpx.NewAPIError(http.StatusInternalServerError, "INTERNAL", "department lookup failed")
	}
	return nil
}

func (s *Service) recordAudit(ctx context.Context, actorID, action, targetType, targetID, result, requestID string) {
	if s.auditLog == nil {
		return
	}
	if err := s.auditLog.Record(ctx, actorID, action, targetType, targetID, result, requestID); err != nil {
		s.log.Error("audit write failed", "action", action, "error", err)
	}
}

func (s *Service) recordDenied(ctx context.Context, actorID, documentID, requestID string) {
	s.recordAudit(ctx, actorID, "document.access_denied", "document", documentID, "denied", requestID)
}

func DetectMimeType(filename string, data []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	expected, ok := mimeByExtension[ext]
	if !ok {
		return "", httpx.NewAPIError(http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE",
			"supported file types are .pdf, .docx and .xlsx")
	}
	if len(data) < 4 {
		return "", httpx.NewAPIError(http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "file content is too short")
	}

	isPDF := bytes.HasPrefix(data, []byte("%PDF-"))
	isOOXML := bytes.HasPrefix(data, []byte{0x50, 0x4B, 0x03, 0x04})
	switch {
	case isPDF && expected == MimePDF:
		return expected, nil
	case isOOXML && expected != MimePDF:
		return expected, nil
	default:
		return "", httpx.NewAPIError(http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE",
			"file content does not match its extension")
	}
}

func SanitizeFilename(name string) string {
	name = strings.TrimSpace(filepath.Base(strings.ReplaceAll(name, `\`, "/")))
	var b strings.Builder
	for _, r := range name {
		switch {
		case r < 0x20 || r == 0x7F:
			continue
		case unicode.IsControl(r):
			continue
		default:
			b.WriteRune(r)
		}
	}
	cleaned := strings.TrimSpace(b.String())
	if cleaned == "." || cleaned == ".." {
		return ""
	}
	if len(cleaned) > 255 {
		cleaned = cleaned[len(cleaned)-255:]
	}
	return cleaned
}

func normalizeListParams(p ListParams) ListParams {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 {
		p.PageSize = 20
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}
	return p
}
