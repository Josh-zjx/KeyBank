package handlers_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/josh-zjx/keybank/handlers"
)

// fakeRateLimiter counts Allow calls per IP and blocks after the limit.
type fakeRateLimiter struct {
	limit  int
	counts sync.Map
}

func (f *fakeRateLimiter) Allow(ip string) bool {
	var counter atomic.Int64
	v, _ := f.counts.LoadOrStore(ip, &counter)
	c := v.(*atomic.Int64).Add(1)
	return int(c) <= f.limit
}

// newMiddlewareMux registers all four routes with the provided middleware applied globally.
func newMiddlewareMux(t *testing.T, mw handlers.Middleware) http.Handler {
	store := newFakeStore()
	h := newTestHandlers(t, store)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/keys", h.CreateKey)
	mux.HandleFunc("GET /api/keys/{id}", h.FetchKey)
	mux.HandleFunc("GET /", h.HomePage)
	mux.HandleFunc("GET /share/{id}", h.SharePage)

	return mw(mux)
}

// --- Security headers ---

var securityHeaderCases = []struct {
	method string
	path   string
}{
	{http.MethodPost, "/api/keys"},
	{http.MethodGet, "/api/keys/some-id"},
	{http.MethodGet, "/"},
	{http.MethodGet, "/share/some-id"},
}

func TestSecurityHeadersOnAllEndpoints(t *testing.T) {
	handler := newMiddlewareMux(t, handlers.SecurityHeaders())

	wantHeaders := map[string]string{
		"Strict-Transport-Security": "max-age=15552000; includeSubDomains",
		"Content-Security-Policy":   "default-src 'self'; script-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'",
		"Referrer-Policy":           "no-referrer",
		"Cache-Control":             "no-store",
		"X-Frame-Options":           "DENY",
		"X-Content-Type-Options":    "nosniff",
	}

	for _, tc := range securityHeaderCases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			for header, want := range wantHeaders {
				if got := w.Header().Get(header); got != want {
					t.Errorf("%s: got %q, want %q", header, got, want)
				}
			}
		})
	}
}

// --- Panic recovery ---

func TestPanicRecoverReturns500(t *testing.T) {
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("something went wrong")
	})

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))
	handler := handlers.PanicRecover(logger)(panicHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want 500", w.Code)
	}
	if strings.Contains(w.Body.String(), "goroutine") {
		t.Error("response body contains stack trace — must not leak to client")
	}
	if !strings.Contains(logBuf.String(), "panic recovered") {
		t.Error("log should contain 'panic recovered'")
	}
}

func TestPanicRecoverDoesNotInterfereWithNormalRequests(t *testing.T) {
	normalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	handler := handlers.PanicRecover(logger)(normalHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
}

// --- MaxBodySize ---

func TestMaxBodySizeWith413ViaCreateKey(t *testing.T) {
	store := newFakeStore()
	h := newTestHandlers(t, store)

	mux := http.NewServeMux()
	mux.Handle("POST /api/keys",
		handlers.MaxBodySize(64*1024)(http.HandlerFunc(h.CreateKey)))

	padding := strings.Repeat("a", 66000)
	body := `{"x":"` + padding + `"}`

	req := httptest.NewRequest(http.MethodPost, "/api/keys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status: got %d, want 413", w.Code)
	}
}

func TestMaxBodySizeAllowsSmallBody(t *testing.T) {
	store := newFakeStore()
	h := newTestHandlers(t, store)

	mux := http.NewServeMux()
	mux.Handle("POST /api/keys",
		handlers.MaxBodySize(64*1024)(http.HandlerFunc(h.CreateKey)))

	req := httptest.NewRequest(http.MethodPost, "/api/keys", strings.NewReader(`{"ttl":300}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201", w.Code)
	}
}

// --- Rate limiting ---

func TestRateLimitAllowsUpToLimit(t *testing.T) {
	const limit = 3
	limiter := &fakeRateLimiter{limit: limit}
	handler := handlers.RateLimit(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 1; i <= limit; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i, w.Code)
		}
	}
}

func TestRateLimitBlocksAfterLimit(t *testing.T) {
	const limit = 3
	limiter := &fakeRateLimiter{limit: limit}
	handler := handlers.RateLimit(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < limit; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.2:9999"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.2:9999"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("N+1 request: got %d, want 429", w.Code)
	}
}

func TestRateLimitDifferentIPsAreIndependent(t *testing.T) {
	const limit = 1
	limiter := &fakeRateLimiter{limit: limit}
	handler := handlers.RateLimit(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:1"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("IP-A first: got %d", w.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "192.168.1.2:1"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("IP-B (different IP): got %d, want 200", w2.Code)
	}
}

// --- Chain ---

func TestChainAppliesMiddlewareInOrder(t *testing.T) {
	var order []string

	mw1 := handlers.Middleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "mw1-before")
			next.ServeHTTP(w, r)
			order = append(order, "mw1-after")
		})
	})
	mw2 := handlers.Middleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "mw2-before")
			next.ServeHTTP(w, r)
			order = append(order, "mw2-after")
		})
	})

	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler")
	})

	handler := handlers.Chain(mw1, mw2)(final)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	want := []string{"mw1-before", "mw2-before", "handler", "mw2-after", "mw1-after"}
	if len(order) != len(want) {
		t.Fatalf("order: got %v, want %v", order, want)
	}
	for i, v := range want {
		if order[i] != v {
			t.Errorf("order[%d]: got %q, want %q", i, order[i], v)
		}
	}
}

func TestRateLimitIPWithoutPort(t *testing.T) {
	limiter := &fakeRateLimiter{limit: 1}
	handler := handlers.RateLimit(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.99"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
}
