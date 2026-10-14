package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
)

type ctxKey int

const requestIDKey ctxKey = 0

const maxRequestIDLen = 128

func NewRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}

func ContextWithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return e.Code + ": " + e.Message
}

func NewAPIError(status int, code, message string) *APIError {
	return &APIError{Status: status, Code: code, Message: message}
}

func ErrNotFound() *APIError {
	return NewAPIError(http.StatusNotFound, "NOT_FOUND", "resource not found")
}

func ErrUnauthorized() *APIError {
	return NewAPIError(http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
}

func ErrBadRequest(message string) *APIError {
	return NewAPIError(http.StatusBadRequest, "INVALID_REQUEST", message)
}

func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func WriteError(w http.ResponseWriter, r *http.Request, apiErr *APIError) {
	WriteJSON(w, apiErr.Status, map[string]any{
		"error": map[string]any{
			"code":       apiErr.Code,
			"message":    apiErr.Message,
			"request_id": RequestIDFromContext(r.Context()),
		},
	})
}

func NormalizeRequestID(id string) string {
	if id == "" || len(id) > maxRequestIDLen {
		return NewRequestID()
	}
	return id
}
