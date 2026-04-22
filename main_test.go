package main

import (
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/josh-zjx/keybank/handlers"
)

// newTestMux builds the same routing table that main() will register,
// allowing integration tests to exercise the full request path.
func newTestMux(t *testing.T, store KeyStore) http.Handler {
	t.Helper()

	templates, err := handlers.ParseTemplates(os.DirFS("."))
	if err != nil {
		t.Fatalf("ParseTemplates: %v", err)
	}
	h := handlers.New(&storeAdapter{ks: store}, slog.Default(), templates)

	staticFS, err := fs.Sub(os.DirFS("."), "static")
	if err != nil {
		t.Fatalf("fs.Sub(static): %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	mux.HandleFunc("POST /api/keys", h.CreateKey)
	mux.HandleFunc("GET /api/keys/{id}", h.FetchKey)
	mux.HandleFunc("GET /", h.HomePage)
	mux.HandleFunc("GET /share/{id}", h.SharePage)
	return mux
}

func TestPrivateKeyIsOneTime(t *testing.T) {
	store := newMemKeyStore()

	original := Record{PrivPEM: "test-private-key-data", CreatedAt: time.Now()}
	id, err := store.Save(original, 24*time.Hour)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := store.Load(id)
	if err != nil {
		t.Fatalf("first Load failed: %v", err)
	}
	if got == nil || got.PrivPEM != original.PrivPEM {
		t.Errorf("first Load = %v, want PrivPEM %q", got, original.PrivPEM)
	}

	got2, err := store.Load(id)
	if err != nil {
		t.Fatalf("second Load returned unexpected error: %v", err)
	}
	if got2 != nil {
		t.Errorf("second Load should return nil after first use, got %+v", got2)
	}
}

func TestFetchEndpointIsOneTime(t *testing.T) {
	store := newMemKeyStore()

	id, err := store.Save(Record{PrivPEM: "fake-private-key", CreatedAt: time.Now()}, 24*time.Hour)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	mux := newTestMux(t, store)

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

func TestStaticStyleRouteServesCSS(t *testing.T) {
	mux := newTestMux(t, newMemKeyStore())

	req := httptest.NewRequest(http.MethodGet, "/static/style.css", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Header().Get("Content-Type"); got != "text/css; charset=utf-8" {
		t.Fatalf("Content-Type: got %q, want text/css; charset=utf-8", got)
	}
	if body := w.Body.String(); body == "" {
		t.Fatal("style.css body is empty")
	}
}
