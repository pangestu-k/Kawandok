package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"office-ai-agent/apps/api/internal/config"
	"office-ai-agent/apps/api/internal/httpx"
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

func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	if s.cfg.MetricsEnabled {
		mux.Handle("GET /metrics", MetricsHandler())
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.Register(mux)
	mux.HandleFunc("/", s.handleNotFound)
	return s.withRequestLog(secureHeaders(withRequestID(withRecovery(s.log, mux))))
}

func Chain(log *slog.Logger, ratePerMinute int, next http.Handler) http.Handler {
	s := &Server{log: log, now: time.Now}
	handler := secureHeaders(s.withRequestLog(withRequestID(withRecovery(log, next))))
	handler = rateLimitMiddleware(ratePerMinute, ratePerMinute/2+1, handler)
	return metricsMiddleware(handler)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
	httpx.WriteJSON(w, status, body)
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, r, httpx.ErrNotFound())
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := httpx.NormalizeRequestID(r.Header.Get("X-Request-ID"))
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(httpx.ContextWithRequestID(r.Context(), id)))
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
				httpx.WriteError(w, r, httpx.NewAPIError(http.StatusInternalServerError, "INTERNAL", "internal server error"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status  int
	flusher http.Flusher
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.status = code
	rec.ResponseWriter.WriteHeader(code)
}

func (rec *statusRecorder) Flush() {
	if rec.flusher != nil {
		rec.flusher.Flush()
	}
}

func (s *Server) withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := s.now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		if f, ok := w.(http.Flusher); ok {
			rec.flusher = f
		}
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
