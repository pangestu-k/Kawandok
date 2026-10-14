package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"office-ai-agent/apps/api/internal/auth"
	"office-ai-agent/apps/api/internal/httpx"
)

type Handlers struct {
	Svc *Service
}

func Register(mux *http.ServeMux, h *Handlers) {
	mux.HandleFunc("POST /v1/chat/sessions", h.createSession)
	mux.HandleFunc("POST /v1/chat/sessions/{id}/messages", h.postMessage)
	mux.HandleFunc("GET /v1/chat/sessions/{id}/messages", h.history)
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
	httpx.WriteError(w, r, httpx.NewAPIError(http.StatusInternalServerError, "INTERNAL", "internal server error"))
}

func (h *Handlers) createSession(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		httpx.WriteError(w, r, httpx.ErrUnauthorized())
		return
	}
	var body struct {
		Title string `json:"title"`
	}
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteError(w, r, httpx.ErrBadRequest("invalid JSON body"))
			return
		}
	}

	sessionID, err := h.Svc.CreateSession(r.Context(), p, body.Title)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]string{"session_id": sessionID})
}

func (h *Handlers) postMessage(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		httpx.WriteError(w, r, httpx.ErrUnauthorized())
		return
	}
	var body struct {
		Message         string `json:"message"`
		ClientRequestID string `json:"client_request_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, r, httpx.ErrBadRequest("invalid JSON body"))
		return
	}

	flusher, canFlush := w.(http.Flusher)
	if !canFlush {
		httpx.WriteError(w, r, httpx.NewAPIError(http.StatusInternalServerError, "INTERNAL", "streaming unsupported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	emit := func(ev Event) error {
		payload, err := json.Marshal(ev.Data)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, payload); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	if err := h.Svc.HandleMessage(r.Context(), p, r.PathValue("id"), body.Message, body.ClientRequestID, emit); err != nil {
		var apiErr *httpx.APIError
		if errors.As(err, &apiErr) {
			payload, _ := json.Marshal(map[string]string{"code": apiErr.Code, "message": apiErr.Message})
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", payload)
			flusher.Flush()
			return
		}
		payload, _ := json.Marshal(map[string]string{"code": "INTERNAL", "message": "internal server error"})
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", payload)
		flusher.Flush()
	}
}

func (h *Handlers) history(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		httpx.WriteError(w, r, httpx.ErrUnauthorized())
		return
	}
	messages, err := h.Svc.History(r.Context(), p, r.PathValue("id"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"messages": messages})
}
