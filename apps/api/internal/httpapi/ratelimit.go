package httpapi

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"office-ai-agent/apps/api/internal/httpx"
)

type rateLimiter struct {
	mu      sync.Mutex
	perMin  int
	burst   int
	buckets map[string]*bucket
	now     func() time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(perMinute, burst int) *rateLimiter {
	return &rateLimiter{
		perMin:  perMinute,
		burst:   burst,
		buckets: map[string]*bucket{},
		now:     time.Now,
	}
}

func (r *rateLimiter) allow(key string) bool {
	if r.perMin <= 0 {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	b, ok := r.buckets[key]
	if !ok {
		r.buckets[key] = &bucket{tokens: float64(r.burst), last: now}
		return true
	}

	elapsed := now.Sub(b.last).Minutes()
	b.tokens += elapsed * float64(r.perMin)
	if b.tokens > float64(r.burst) {
		b.tokens = float64(r.burst)
	}
	b.last = now

	if b.tokens < 1 {
		if len(r.buckets) > 10_000 {
			r.gcLocked(now)
		}
		return false
	}
	b.tokens--
	if len(r.buckets) > 10_000 {
		r.gcLocked(now)
	}
	return true
}

func (r *rateLimiter) gcLocked(now time.Time) {
	for k, b := range r.buckets {
		if now.Sub(b.last) > 10*time.Minute {
			delete(r.buckets, k)
		}
	}
}

func rateLimitMiddleware(perMinute, burst int, next http.Handler) http.Handler {
	limiter := newRateLimiter(perMinute, burst)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.allow(clientKey(r)) {
			w.Header().Set("Retry-After", "1")
			httpx.WriteError(w, r, httpx.NewAPIError(http.StatusTooManyRequests, "RATE_LIMITED",
				"too many requests; slow down"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientKey(r *http.Request) string {
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return host
}
