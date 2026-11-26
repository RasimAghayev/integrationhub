package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/time/rate"
)

type ctxKey int

const requestIDKey ctxKey = 0

// RequestIDHeader ensures every response carries an X-Request-ID header,
// reusing chi's request id when present.
func RequestIDHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := middleware.GetReqID(r.Context())
		if id == "" {
			id = newID()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

// RequestIDFromContext returns the request id stored by RequestIDHeader.
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}

// SecurityHeaders sets conservative security headers on every response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// RateLimiter keeps one token bucket per client IP with idle eviction.
type RateLimiter struct {
	mu    sync.Mutex
	m     map[string]*entry
	rate  rate.Limit
	burst int
	ttl   time.Duration
}

type entry struct {
	lim  *rate.Limiter
	last time.Time
}

// NewRateLimiter builds a per-IP limiter. ttl<=0 disables eviction.
func NewRateLimiter(r rate.Limit, burst int, ttl time.Duration) *RateLimiter {
	if burst <= 0 {
		burst = 1
	}
	return &RateLimiter{m: map[string]*entry{}, rate: r, burst: burst, ttl: ttl}
}

func (l *RateLimiter) get(key string, now time.Time) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ttl > 0 && len(l.m) > 1024 {
		for k, e := range l.m {
			if now.Sub(e.last) > l.ttl {
				delete(l.m, k)
			}
		}
	}
	e, ok := l.m[key]
	if !ok {
		e = &entry{lim: rate.NewLimiter(l.rate, l.burst)}
		l.m[key] = e
	}
	e.last = now
	return e.lim
}

// Middleware enforces the rate limit, returning 429 with Retry-After.
func (l *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if l == nil {
			next.ServeHTTP(w, r)
			return
		}
		if !l.get(clientIP(r), time.Now()).Allow() {
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// CORS returns middleware that allows only the configured origins.
func CORS(origins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(origins))
	for _, o := range origins {
		allowed[o] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if origin := r.Header.Get("Origin"); origin != "" && allowed[origin] {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Vary", "Origin")
				h.Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Content-Type,Authorization,Idempotency-Key,X-Request-ID")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
