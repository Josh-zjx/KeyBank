package handlers_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/josh-zjx/keybank/handlers"
)

// fakeStore satisfies handlers.Store for testing without RSA or Redis.
type fakeStore struct {
	data   map[string][]byte
	nextID string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		data:   make(map[string][]byte),
		nextID: "test-id-abc",
	}
}

func (f *fakeStore) Save(key []byte) (string, error) {
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
	h := handlers.New(newFakeStore(), slog.Default())
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
	id, _ := store.Save([]byte("fake-priv-key"))
	h := handlers.New(store, slog.Default())
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
	id, _ := store.Save([]byte("fake-priv-key"))
	h := handlers.New(store, slog.Default())
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
	h := handlers.New(newFakeStore(), slog.Default())
	mux := newServeMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/keys/no-such-id", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", w.Code)
	}
}

func TestHomePageOK(t *testing.T) {
	h := handlers.New(newFakeStore(), slog.Default())
	mux := newServeMux(h)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
}

func TestSharePageOK(t *testing.T) {
	h := handlers.New(newFakeStore(), slog.Default())
	mux := newServeMux(h)

	req := httptest.NewRequest(http.MethodGet, "/share/abc123", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
}
