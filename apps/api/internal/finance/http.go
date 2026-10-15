package finance

import (
	"net/http"
	"time"

	"office-ai-agent/apps/api/internal/auth"
	"office-ai-agent/apps/api/internal/httpx"
)

const dateLayout = "2006-01-02"

// Register exposes read-only, scoped and audited finance endpoints. docs/06
// allows public finance endpoints only with explicit permission checks and
// audit — both are enforced by the underlying Service.
func Register(mux *http.ServeMux, svc *Service) {
	h := &Handlers{svc: svc}
	mux.HandleFunc("GET /v1/finance/summary", h.summary)
	mux.HandleFunc("GET /v1/finance/compare", h.compare)
}

type Handlers struct {
	svc *Service
}

func (h *Handlers) principal(r *http.Request) (auth.Principal, bool) {
	return auth.PrincipalFromContext(r.Context())
}

func (h *Handlers) writeError(w http.ResponseWriter, r *http.Request, err error) {
	if apiErr, ok := err.(*httpx.APIError); ok {
		httpx.WriteError(w, r, apiErr)
		return
	}
	httpx.WriteError(w, r, httpx.NewAPIError(http.StatusInternalServerError, "INTERNAL", "internal server error"))
}

func parseDateParam(r *http.Request, name string) (time.Time, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(dateLayout, raw)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func (h *Handlers) summary(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		httpx.WriteError(w, r, httpx.ErrUnauthorized())
		return
	}

	start, okStart := parseDateParam(r, "start")
	end, okEnd := parseDateParam(r, "end")
	if !okStart || !okEnd {
		httpx.WriteError(w, r, httpx.ErrBadRequest(
			"query parameters start and end are required (format YYYY-MM-DD)"))
		return
	}

	result, err := h.svc.Summary(r.Context(), p, r.URL.Query().Get("department"), start, end,
		httpx.RequestIDFromContext(r.Context()))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handlers) compare(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		httpx.WriteError(w, r, httpx.ErrUnauthorized())
		return
	}

	q := r.URL.Query()
	aStart, okA := parseDateParam(r, "a_start")
	aEnd, okAEnd := parseDateParam(r, "a_end")
	bStart, okB := parseDateParam(r, "b_start")
	bEnd, okBEnd := parseDateParam(r, "b_end")
	if !okA || !okAEnd || !okB || !okBEnd {
		httpx.WriteError(w, r, httpx.ErrBadRequest(
			"query parameters a_start, a_end, b_start and b_end are required (format YYYY-MM-DD)"))
		return
	}

	result, err := h.svc.Compare(r.Context(), p, q.Get("department"), aStart, aEnd, bStart, bEnd,
		httpx.RequestIDFromContext(r.Context()))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}
