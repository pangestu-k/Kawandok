package httpapi

import (
	"net/http"
	"regexp"
	"time"

	"office-ai-agent/apps/api/internal/metrics"
)

var uuidPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &metricsRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)
		elapsed := time.Since(start).Milliseconds()
		route := normalizeRoute(r.URL.Path)
		metrics.ObserveHTTP(route, r.Method, rec.status, float64(elapsed))
	})
}

type metricsRecorder struct {
	http.ResponseWriter
	status int
}

func (rec *metricsRecorder) WriteHeader(code int) {
	rec.status = code
	rec.ResponseWriter.WriteHeader(code)
}

func (rec *metricsRecorder) Flush() {
	if f, ok := rec.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func normalizeRoute(path string) string {
	normalized := uuidPattern.ReplaceAllString(path, "{id}")
	if len(normalized) > 120 {
		normalized = normalized[:120]
	}
	return normalized
}

func MetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(metrics.Render()))
	})
}
