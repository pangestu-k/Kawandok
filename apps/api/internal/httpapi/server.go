package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"office-ai-agent/apps/api/internal/config"
	"office-ai-agent/apps/api/internal/logging"
)

const readyzTimeout = 2 * time.Second

type Pinger interface {
	PingContext(ctx context.Context) error
}

type Server struct {
	cfg    config.Config
	log    *slog.Logger
	pinger Pinger
	now    func() time.Time
}

func New(cfg config.Config, log *slog.Logger, pinger Pinger) *Server {
	return &Server{
		cfg:    cfg,
		log:    log,
		pinger: pinger,
		now:    time.Now,
	}
}

func NewRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.HandleFunc("/", s.handleNotFound)

	handler := secureHeaders(withRequestID(withRecovery(s.log, mux)))
	return s.withRequestLog(handler)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readyzTimeout)
	defer cancel()

	checks := map[string]string{"database": "not_configured"}
	status := http.StatusOK

	if s.pinger != nil {
		if err := s.pinger.PingContext(ctx); err != nil {
			checks["database"] = "unreachable"
			status = http.StatusServiceUnavailable
		} else {
			checks["database"] = "ok"
		}
	}

	body := map[string]any{"checks": checks}
	if status == http.StatusOK {
		body["status"] = "ok"
	} else {
		body["status"] = "unavailable"
	}
	writeJSON(w, status, body)
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, http.StatusNotFound, "NOT_FOUND", "route not found")
}

type ctxKey int

const requestIDKey ctxKey = 0

func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" || len(id) > 128 {
			id = NewRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func withRecovery(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error("panic recovered", "panic", rec, "path", r.URL.Path)
				writeError(w, r, http.StatusInternalServerError, "INTERNAL", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.status = code
	rec.ResponseWriter.WriteHeader(code)
}

func (s *Server) withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := s.now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		fields := logging.Scrub(map[string]any{
			"event":       "http_request",
			"method":      r.Method,
			"path":        r.URL.Path,
			"status":      rec.status,
			"duration_ms": s.now().Sub(start).Milliseconds(),
			"request_id":  w.Header().Get("X-Request-ID"),
		})
		s.log.Info("http request", toSlogArgs(fields)...)
	})
}

func toSlogArgs(fields map[string]any) []any {
	args := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}
	return args
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		return
	}
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":       code,
			"message":    message,
			"request_id": RequestIDFromContext(r.Context()),
		},
	})
}
