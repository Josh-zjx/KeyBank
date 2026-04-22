package main

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/josh-zjx/keybank/handlers"
)

// newTestMux builds the same routing table that main() will register,
// allowing integration tests to exercise the full request path.
func newTestMux(store KeyStore) http.Handler {
	h := handlers.New(store, slog.Default())
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/keys", h.CreateKey)
	mux.HandleFunc("GET /api/keys/{id}", h.FetchKey)
	mux.HandleFunc("GET /", h.HomePage)
	mux.HandleFunc("GET /share/{id}", h.SharePage)
	return mux
}

func TestPrivateKeyIsOneTime(t *testing.T) {
	store := newMemKeyStore()

	original := []byte("test-private-key-data")
	id, err := store.Save(original)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := store.Load(id)
	if err != nil {
		t.Fatalf("first Load failed: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Errorf("first Load = %q, want %q", got, original)
	}

	got2, err := store.Load(id)
	if err != nil {
		t.Fatalf("second Load returned unexpected error: %v", err)
	}
	if got2 != nil {
		t.Errorf("second Load should return nil after first use, got %q", got2)
	}
}

func TestFetchEndpointIsOneTime(t *testing.T) {
	store := newMemKeyStore()

	id, err := store.Save([]byte("fake-private-key"))
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	mux := newTestMux(store)

	req1 := httptest.NewRequest(http.MethodGet, "/api/keys/"+id, nil)
	w1 := httptest.NewRecorder()
	mux.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Errorf("first fetch: got status %d, want %d", w1.Code, http.StatusOK)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/keys/"+id, nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code != http.StatusNotFound {
		t.Errorf("second fetch: got status %d, want %d", w2.Code, http.StatusNotFound)
	}
}
