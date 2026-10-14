package documents

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"office-ai-agent/apps/api/internal/auth"
	"office-ai-agent/apps/api/internal/httpx"
)

var validStatusFilters = map[string]bool{
	"": true, "uploaded": true, "queued": true, "processing": true, "ready": true, "failed": true,
}

type Handlers struct {
	Svc *Service
	Log *slog.Logger
}

func Register(mux *http.ServeMux, h *Handlers) {
	mux.HandleFunc("POST /v1/documents", h.upload)
	mux.HandleFunc("GET /v1/documents", h.list)
	mux.HandleFunc("GET /v1/documents/{id}", h.get)
	mux.HandleFunc("GET /v1/documents/{id}/download", h.download)
	mux.HandleFunc("GET /v1/documents/{id}/versions/{versionId}/status", h.versionStatus)
	mux.HandleFunc("POST /v1/documents/{id}/reindex", h.reindex)
	mux.HandleFunc("GET /v1/departments", h.listDepartments)
}

func (h *Handlers) listDepartments(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(r); !ok {
		httpx.WriteError(w, r, httpx.ErrUnauthorized())
		return
	}
	rows, err := h.Svc.ListDepartments(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"departments": rows})
}

func (h *Handlers) reindex(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		httpx.WriteError(w, r, httpx.ErrUnauthorized())
		return
	}

	jobID, deduped, err := h.Svc.Reindex(r.Context(), p, r.PathValue("id"), httpx.RequestIDFromContext(r.Context()))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{
		"job_id":       jobID,
		"status":       "queued",
		"deduplicated": deduped,
	})
}

func (h *Handlers) principal(r *http.Request) (auth.Principal, bool) {
	return auth.PrincipalFromContext(r.Context())
}

func (h *Handlers) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *httpx.APIError
	if errors.As(err, &apiErr) {
		httpx.WriteError(w, r, apiErr)
		return
	}
	h.Log.Error("unhandled error", "error", err, "request_id", httpx.RequestIDFromContext(r.Context()))
	httpx.WriteError(w, r, httpx.NewAPIError(http.StatusInternalServerError, "INTERNAL", "internal server error"))
}

func (h *Handlers) upload(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		httpx.WriteError(w, r, httpx.ErrUnauthorized())
		return
	}

	if err := r.ParseMultipartForm(8 << 20); err != nil {
		httpx.WriteError(w, r, httpx.ErrBadRequest("multipart/form-data body required"))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, r, httpx.ErrBadRequest("file field is required"))
		return
	}
	defer file.Close()

	max := h.Svc.MaxBytes() + 1
	data, err := io.ReadAll(io.LimitReader(file, max))
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	doc, err := h.Svc.Upload(
		r.Context(),
		p,
		r.FormValue("title"),
		r.FormValue("classification"),
		r.FormValue("department_id"),
		header.Filename,
		data,
		[]byte(r.FormValue("grants")),
		httpx.RequestIDFromContext(r.Context()),
	)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]string{
		"document_id": doc.ID,
		"version_id":  doc.VersionID,
		"status":      doc.Status,
	})
}

func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		httpx.WriteError(w, r, httpx.ErrUnauthorized())
		return
	}

	status := r.URL.Query().Get("status")
	if !validStatusFilters[status] {
		httpx.WriteError(w, r, httpx.ErrBadRequest("invalid status filter"))
		return
	}

	page, err := strconv.Atoi(orDefault(r.URL.Query().Get("page"), "1"))
	if err != nil || page < 1 {
		httpx.WriteError(w, r, httpx.ErrBadRequest("page must be a positive integer"))
		return
	}
	pageSize, err := strconv.Atoi(orDefault(r.URL.Query().Get("page_size"), "20"))
	if err != nil || pageSize < 1 {
		httpx.WriteError(w, r, httpx.ErrBadRequest("page_size must be a positive integer"))
		return
	}

	res, err := h.Svc.List(r.Context(), p, ListParams{
		Query:    r.URL.Query().Get("query"),
		Status:   status,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}

func (h *Handlers) get(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		httpx.WriteError(w, r, httpx.ErrUnauthorized())
		return
	}

	doc, err := h.Svc.Get(r.Context(), p, r.PathValue("id"), httpx.RequestIDFromContext(r.Context()))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, doc)
}

func (h *Handlers) download(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		httpx.WriteError(w, r, httpx.ErrUnauthorized())
		return
	}

	presigned := r.URL.Query().Get("mode") == "presigned"
	d, err := h.Svc.Download(r.Context(), p, r.PathValue("id"), presigned, httpx.RequestIDFromContext(r.Context()))
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	if d.PresignedURL != "" {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"url":                d.PresignedURL,
			"expires_in_seconds": int(h.Svc.presignTTL.Seconds()),
		})
		return
	}

	w.Header().Set("Content-Type", d.MimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(d.Size, 10))
	w.Header().Set("Content-Disposition", contentDisposition(d.Filename))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, d.Reader)
	d.Reader.Close()
}

func (h *Handlers) versionStatus(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		httpx.WriteError(w, r, httpx.ErrUnauthorized())
		return
	}

	status, err := h.Svc.Status(
		r.Context(),
		p,
		r.PathValue("id"),
		r.PathValue("versionId"),
		httpx.RequestIDFromContext(r.Context()),
	)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{
		"version_id": r.PathValue("versionId"),
		"status":     status,
	})
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func contentDisposition(filename string) string {
	ascii := make([]rune, 0, len(filename))
	for _, r := range filename {
		if r < 0x20 || r > 0x7E || r == '"' || r == '\\' {
			ascii = append(ascii, '_')
			continue
		}
		ascii = append(ascii, r)
	}
	return `attachment; filename="` + string(ascii) + `"; filename*=UTF-8''` + url.PathEscape(filename)
}
