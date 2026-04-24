package handlers

import (
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
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

// SecurityHeaders sets production security headers on share and retrieval responses.
// Sets Cache-Control: no-store to prevent caching of sensitive content.
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

// StaticHeaders sets security headers for static assets, without Cache-Control: no-store
// to allow browser caching of CSS, JS, and other static files.
func StaticHeaders() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Strict-Transport-Security", "max-age=15552000; includeSubDomains")
			h.Set("Content-Security-Policy",
				"default-src 'self'; script-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'")
			h.Set("Referrer-Policy", "no-referrer")
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
// trustXFF controls whether to trust the X-Forwarded-For header.
func RateLimit(limiter RateLimiter, trustXFF bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := remoteIP(r, trustXFF)
			if !limiter.Allow(ip) {
				http.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// remoteIP returns the client IP address.
// When trustXFF is true and X-Forwarded-For is present, returns the first (leftmost) IP from that header.
// Otherwise returns the host portion of r.RemoteAddr (port stripped).
func remoteIP(r *http.Request, trustXFF bool) string {
	if trustXFF {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// Take the first (leftmost) IP from the X-Forwarded-For list.
			ips := strings.Split(xff, ",")
			if len(ips) > 0 {
				if first := strings.TrimSpace(ips[0]); first != "" {
					return first
				}
			}
		}
	}
	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
