package handlers_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/josh-zjx/keybank/handlers"
)

// fakeStore satisfies handlers.Store for testing without RSA or Redis.
type fakeStore struct {
	data    map[string][]byte
	nextID  string
	lastTTL time.Duration
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		data:   make(map[string][]byte),
		nextID: "test-id-abc",
	}
}

func (f *fakeStore) Save(key []byte, ttl time.Duration) (string, error) {
	f.lastTTL = ttl
	f.data[f.nextID] = key
	return f.nextID, nil
}

func (f *fakeStore) Load(id string) ([]byte, error) {
	v, ok := f.data[id]
	if !ok {
		return nil, nil
	}

	delete(f.data, id)
	return v, nil
}

func newTestHandlers(t *testing.T, store handlers.Store) *handlers.Handlers {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Join(filepath.Dir(file), "..")

	templates, err := handlers.ParseTemplates(os.DirFS(root))
	if err != nil {
		t.Fatalf("ParseTemplates: %v", err)
	}

	return handlers.New(store, slog.Default(), templates)
}

// newServeMux registers all routes on a fresh ServeMux using h.
func newServeMux(h *handlers.Handlers) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/keys", h.CreateKey)
	mux.HandleFunc("GET /api/keys/{id}", h.FetchKey)
	mux.HandleFunc("GET /", h.HomePage)
	mux.HandleFunc("GET /share/{id}", h.SharePage)
	return mux
}

func TestCreateKeyReturnsJSON(t *testing.T) {
	h := newTestHandlers(t, newFakeStore())
	mux := newServeMux(h)

	req := httptest.NewRequest(http.MethodPost, "/api/keys", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type: got %q, want application/json", ct)
	}

	var resp handlers.CreateKeyResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp.ID == "" {
		t.Fatal("id field is empty")
	}
	if resp.PubPEM == "" {
		t.Fatal("pub_pem field is empty")
	}
}

func TestFetchKeyReturnsKey(t *testing.T) {
	store := newFakeStore()
	id, _ := store.Save([]byte("fake-priv-key"), 24*time.Hour)
	h := newTestHandlers(t, store)
	mux := newServeMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/keys/"+id, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	if got := w.Body.String(); got != "fake-priv-key" {
		t.Fatalf("body: got %q, want %q", got, "fake-priv-key")
	}
}

func TestFetchKeyIsOneTime(t *testing.T) {
	store := newFakeStore()
	id, _ := store.Save([]byte("fake-priv-key"), 24*time.Hour)
	h := newTestHandlers(t, store)
	mux := newServeMux(h)

	req1 := httptest.NewRequest(http.MethodGet, "/api/keys/"+id, nil)
	w1 := httptest.NewRecorder()
	mux.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first fetch: got %d, want 200", w1.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/keys/"+id, nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code != http.StatusNotFound {
		t.Fatalf("second fetch: got %d, want 404", w2.Code)
	}
}

func TestFetchKeyNotFound(t *testing.T) {
	h := newTestHandlers(t, newFakeStore())
	mux := newServeMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/keys/no-such-id", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", w.Code)
	}
}

func TestHomePageOK(t *testing.T) {
	h := newTestHandlers(t, newFakeStore())
	mux := newServeMux(h)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type: got %q, want text/html; charset=utf-8", got)
	}

	body := w.Body.String()
	for _, want := range []string{
		"Create a secure note",
		"Generate link",
		"/static/style.css",
		"/static/qrcode.js",
		"/static/app.js",
		`textarea id="message"`,
		`name="expiryPreset"`,
		`data-page="create"`,
		`data-share-url`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("home page missing %q", want)
		}
	}
}

func TestSharePageOK(t *testing.T) {
	h := newTestHandlers(t, newFakeStore())
	mux := newServeMux(h)

	req := httptest.NewRequest(http.MethodGet, "/share/abc123", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type: got %q, want text/html; charset=utf-8", got)
	}

	body := w.Body.String()
	for _, want := range []string{
		"Open a shared note",
		"Decrypt",
		"abc123",
		`data-share-id="abc123"`,
		`data-page="share"`,
		`data-autohide-seconds="60"`,
		"Ready to fetch the one-time private key and decrypt locally.",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("share page missing %q", want)
		}
	}
}

// TTL boundary tests

func postWithBody(mux http.Handler, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(http.MethodPost, "/api/keys", nil)
	} else {
		r = httptest.NewRequest(http.MethodPost, "/api/keys", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func TestCreateKeyDefaultTTL(t *testing.T) {
	store := newFakeStore()
	mux := newServeMux(newTestHandlers(t, store))
	w := postWithBody(mux, "")
	if w.Code != http.StatusCreated {
		t.Fatalf("got %d, want 201", w.Code)
	}
	if store.lastTTL != 24*time.Hour {
		t.Errorf("lastTTL = %v, want 24h", store.lastTTL)
	}
}

func TestCreateKeyTTLZeroDefaultsTo24h(t *testing.T) {
	store := newFakeStore()
	mux := newServeMux(newTestHandlers(t, store))
	w := postWithBody(mux, `{"ttl":0}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("got %d, want 201", w.Code)
	}
	if store.lastTTL != 24*time.Hour {
		t.Errorf("lastTTL = %v, want 24h", store.lastTTL)
	}
}

func TestCreateKeyMinTTL(t *testing.T) {
	mux := newServeMux(newTestHandlers(t, newFakeStore()))
	w := postWithBody(mux, `{"ttl":60}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("got %d, want 201", w.Code)
	}
}

func TestCreateKeyMaxTTL(t *testing.T) {
	mux := newServeMux(newTestHandlers(t, newFakeStore()))
	w := postWithBody(mux, `{"ttl":604800}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("got %d, want 201", w.Code)
	}
}

func TestCreateKeyTTLTooLow(t *testing.T) {
	mux := newServeMux(newTestHandlers(t, newFakeStore()))
	w := postWithBody(mux, `{"ttl":59}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "invalid_ttl" {
		t.Errorf("error = %q, want %q", body["error"], "invalid_ttl")
	}
}

func TestCreateKeyTTLTooHigh(t *testing.T) {
	mux := newServeMux(newTestHandlers(t, newFakeStore()))
	w := postWithBody(mux, `{"ttl":604801}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "invalid_ttl" {
		t.Errorf("error = %q, want %q", body["error"], "invalid_ttl")
	}
}

// --- Store error paths ---

// errorStore always returns an error from Save and Load.
type errorStore struct{}

func (e *errorStore) Save(_ []byte, _ time.Duration) (string, error) {
	return "", fmt.Errorf("store unavailable")
}

func (e *errorStore) Load(_ string) ([]byte, error) {
	return nil, fmt.Errorf("store unavailable")
}

func TestCreateKeyStoreSaveError(t *testing.T) {
	h := newTestHandlers(t, &errorStore{})
	mux := newServeMux(h)

	req := httptest.NewRequest("POST", "/api/keys", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 500 {
		t.Fatalf("status: got %d, want 500", w.Code)
	}
}

func TestFetchKeyStoreLoadError(t *testing.T) {
	h := newTestHandlers(t, &errorStore{})
	mux := newServeMux(h)

	req := httptest.NewRequest("GET", "/api/keys/any-id", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 500 {
		t.Fatalf("status: got %d, want 500", w.Code)
	}
}
