package handlers

import (
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
)

// Middleware is an HTTP handler wrapper.
type Middleware func(http.Handler) http.Handler

// Chain composes multiple middlewares into a single one, executing left to right.
func Chain(mw ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		for i := len(mw) - 1; i >= 0; i-- {
			next = mw[i](next)
		}
		return next
	}
}

// PanicRecover catches any panic in downstream handlers, logs it, and returns 500.
// The stack trace is written to the logger, never to the response body.
func PanicRecover(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered",
						"panic", rec,
						"stack", string(debug.Stack()),
					)
					http.Error(w, "internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders sets production security headers on every response.
func SecurityHeaders() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Strict-Transport-Security", "max-age=15552000; includeSubDomains")
			h.Set("Content-Security-Policy",
				"default-src 'self'; script-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Cache-Control", "no-store")
			h.Set("X-Frame-Options", "DENY")
			h.Set("X-Content-Type-Options", "nosniff")
			next.ServeHTTP(w, r)
		})
	}
}

// MaxBodySize wraps the request body with http.MaxBytesReader, capping reads at limit bytes.
// Downstream handlers that read beyond the limit will receive *http.MaxBytesError.
func MaxBodySize(limit int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimiter decides whether a request from the given IP should be allowed.
type RateLimiter interface {
	Allow(ip string) bool
}

// RateLimit returns a middleware that enforces the given limiter per client IP.
// Blocked requests receive 429 Too Many Requests.
func RateLimit(limiter RateLimiter) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := remoteIP(r)
			if !limiter.Allow(ip) {
				http.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// remoteIP returns the host portion of r.RemoteAddr (port stripped).
// It does not trust X-Forwarded-For to prevent IP spoofing.
func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
