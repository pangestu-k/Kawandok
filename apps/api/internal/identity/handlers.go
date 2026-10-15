package identity

import (
	"errors"
	"net/http"

	"office-ai-agent/apps/api/internal/auth"
	"office-ai-agent/apps/api/internal/httpx"
)

type Handlers struct {
	Svc *Service
	Log interface{ Error(string, ...any) }
}

func Register(mux *http.ServeMux, h *Handlers) {
	mux.HandleFunc("GET /v1/me", h.me)
	mux.HandleFunc("GET /v1/users", h.listUsers)
	mux.HandleFunc("GET /v1/roles", h.listRoles)
	mux.HandleFunc("GET /v1/departments", h.listDepartments)
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
	if h.Log != nil {
		h.Log.Error("identity handler error", "error", err)
	}
	httpx.WriteError(w, r, httpx.NewAPIError(http.StatusInternalServerError, "INTERNAL", "internal server error"))
}

func (h *Handlers) me(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		httpx.WriteError(w, r, httpx.ErrUnauthorized())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"id":            p.ID,
		"email":         p.Email,
		"display_name":  p.DisplayName,
		"department_id": p.DepartmentID,
		"roles":         p.Roles,
		"is_admin":      p.IsAdmin(),
	})
}

func (h *Handlers) requireAdmin(w http.ResponseWriter, r *http.Request) (auth.Principal, bool) {
	p, ok := h.principal(r)
	if !ok {
		httpx.WriteError(w, r, httpx.ErrUnauthorized())
		return auth.Principal{}, false
	}
	isAdmin, err := h.Svc.IsAdmin(r.Context(), p.ID)
	if err != nil {
		h.writeError(w, r, err)
		return auth.Principal{}, false
	}
	if !isAdmin {
		h.Svc.record(r.Context(), p.ID, "identity.admin_list", "denied", httpx.RequestIDFromContext(r.Context()))
		httpx.WriteError(w, r, httpx.NewAPIError(http.StatusForbidden, "FORBIDDEN",
			"user and role administration requires the admin role"))
		return auth.Principal{}, false
	}
	return p, true
}

func (h *Handlers) listUsers(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	users, err := h.Svc.ListUsers(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.Svc.record(r.Context(), p.ID, "identity.user_list", "allowed", httpx.RequestIDFromContext(r.Context()))
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (h *Handlers) listRoles(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	roles, err := h.Svc.ListRoles(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"roles": roles})
}

func (h *Handlers) listDepartments(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(r); !ok {
		httpx.WriteError(w, r, httpx.ErrUnauthorized())
		return
	}
	departments, err := h.Svc.ListDepartments(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"departments": departments})
}
