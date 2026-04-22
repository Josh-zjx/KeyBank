# Milestone 0 — Foundation Implementation Plan

**Goal:** Refactor monolithic `main.go` into `config.go` / `record.go` / `handlers/` sub-package, reshape the URL surface to the target API, return JSON from key creation, and wire `log/slog` with JSON output.

**Architecture:** Split `package main` across focused files: `config.go` (env config), `record.go` (future storage types placeholder), `store.go` (all store implementations), and a `handlers/` sub-package (HTTP handlers + key generation). Routing moves to Go 1.22 stdlib `ServeMux` (method+pattern syntax, still no framework). The `handlers` package defines its own minimal `Store` interface — Go structural typing satisfies it with `memKeyStore`/`redisKeyStore` from `package main` without circular imports.

**Tech Stack:** Go 1.22+, `net/http` stdlib (`ServeMux`), `log/slog` (JSON), `encoding/json`, `github.com/go-redis/redis/v8`

---

### File Map

| File | Action | Responsibility |
|---|---|---|
| `go.mod` | Modify | Bump to `go 1.22` |
| `store.go` | Modify | Add `redisKeyStore` (moved from `main.go`); all store code in one file |
| `config.go` | Create | `Config` struct + `loadConfig()` from env vars |
| `record.go` | Create | Package-level home for storage types; M1 adds `Record{PrivPEM, CreatedAt}` here |
| `handlers/handlers.go` | Create | `Store` interface, `Handlers` struct, `generateKey`, all 4 handler methods, `CreateKeyResponse` |
| `handlers/handlers_test.go` | Create | Unit tests for all 4 handlers using a local `fakeStore` |
| `main.go` | Modify | Remove `Server`, `webServer`, `generateKey`, old store code; add `loadConfig`, `slog`, `ServeMux`, `handlers.New` wiring |
| `main_test.go` | Modify | Replace `Server`+`webServer` with `newTestMux` helper; update URL `/fetch/` → `/api/keys/` |
| `README.md` | Modify | Add API section documenting endpoints and JSON response shape |

---

### Task 1: Move `redisKeyStore` to `store.go` and bump Go version

**Files:**
- Modify: `go.mod` (line 3)
- Modify: `store.go` (add `redisKeyStore` + imports)
- Modify: `main.go` (remove `redisKeyStore` struct/methods + unused imports)

- [ ] **Step 1: Update `go.mod`**

Replace the entire file:
```
module github.com/josh-zjx/keybank

go 1.22

require github.com/go-redis/redis/v8 v8.11.3

require (
	github.com/cespare/xxhash/v2 v2.1.1 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
)
```

- [ ] **Step 2: Append `redisKeyStore` to `store.go`**

Add these imports to `store.go`'s import block (after `sync`):
```go
import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	goredis "github.com/go-redis/redis/v8"
)
```

Then append after `randomID`:
```go
// redisKeyStore implements KeyStore using Redis GETDEL for atomic one-time semantics.
type redisKeyStore struct {
	client *goredis.Client
}

func newRedisKeyStore(addr string) *redisKeyStore {
	return &redisKeyStore{
		client: goredis.NewClient(&goredis.Options{Addr: addr}),
	}
}

func (r *redisKeyStore) Save(key []byte) (string, error) {
	id, err := randomID()
	if err != nil {
		return "", err
	}
	if err := r.client.Set(context.Background(), id, key, 24*time.Hour).Err(); err != nil {
		return "", err
	}
	return id, nil
}

func (r *redisKeyStore) Load(id string) ([]byte, error) {
	val, err := r.client.GetDel(context.Background(), id).Bytes()
	if err == goredis.Nil {
		return nil, nil
	}
	return val, err
}
```

- [ ] **Step 3: Remove `redisKeyStore` and its imports from `main.go`**

Delete lines 96–124 (the `redisKeyStore` block) from `main.go`.

Update `main.go` imports — remove `context`, `time`, and `goredis "github.com/go-redis/redis/v8"`. The remaining imports are:
```go
import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"strings"
)
```

- [ ] **Step 4: Run tests**

```bash
go test ./...
```
Expected: `PASS` — both existing tests still pass; `redisKeyStore` now lives in `store.go`.

- [ ] **Step 5: Commit**

```bash
git add go.mod store.go main.go
git commit -m "refactor: move redisKeyStore to store.go, bump go 1.22"
```

---

### Task 2: Extract `config.go`

**Files:**
- Create: `config.go`
- Modify: `main.go` (use `loadConfig()` instead of inline env reads)

- [ ] **Step 1: Create `config.go`**

```go
package main

import "os"

type Config struct {
	Port      string
	RedisAddr string
}

func loadConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "14000"
	}
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	return Config{Port: port, RedisAddr: redisAddr}
}
```

- [ ] **Step 2: Update `main()` in `main.go` to use `loadConfig()`**

Replace the first several lines of `main()`:
```go
func main() {
	cfg := loadConfig()
	s := &Server{store: newRedisKeyStore(cfg.RedisAddr)}
	addr := ":" + cfg.Port
	fmt.Printf("Listening on %s\n", addr)
	if err := http.ListenAndServe(addr, http.HandlerFunc(s.webServer)); err != nil {
		panic(err)
	}
}
```

Remove `"os"` from `main.go` imports (now used only in `config.go`). Updated imports:
```go
import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
)
```

- [ ] **Step 3: Run tests**

```bash
go test ./...
```
Expected: `PASS`

- [ ] **Step 4: Commit**

```bash
git add config.go main.go
git commit -m "refactor: extract loadConfig from main()"
```

---

### Task 3: Create `record.go`

**Files:**
- Create: `record.go`

- [ ] **Step 1: Create `record.go`**

```go
package main
```

This file is the designated home for the storage record type. Milestone 1 adds `Record{PrivPEM string, CreatedAt time.Time}` here.

- [ ] **Step 2: Verify project still compiles**

```bash
go build ./...
```
Expected: success (no output).

- [ ] **Step 3: Commit**

```bash
git add record.go
git commit -m "chore: add record.go for M1 storage types"
```

---

### Task 4: Create `handlers/` package (TDD)

**Files:**
- Create: `handlers/handlers_test.go`
- Create: `handlers/handlers.go`

#### Step group A — Write failing tests

- [ ] **Step 1: Create `handlers/handlers_test.go`**

```go
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
	return &fakeStore{data: make(map[string][]byte), nextID: "test-id-abc"}
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
```

- [ ] **Step 2: Verify tests fail to compile (package doesn't exist yet)**

```bash
go test ./handlers/...
```
Expected: compilation error — `cannot find package "github.com/josh-zjx/keybank/handlers"`

#### Step group B — Implement the handlers package

- [ ] **Step 3: Create `handlers/handlers.go`**

```go
package handlers

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"net/http"
)

// Store is the minimal persistence interface handlers need.
// memKeyStore and redisKeyStore in package main satisfy this implicitly.
type Store interface {
	Save(key []byte) (id string, err error)
	Load(id string) (key []byte, err error)
}

// Handlers holds dependencies for all HTTP handler methods.
type Handlers struct {
	store  Store
	logger *slog.Logger
}

// New creates a Handlers with the given store and logger.
func New(store Store, logger *slog.Logger) *Handlers {
	return &Handlers{store: store, logger: logger}
}

// CreateKeyResponse is the JSON body returned by POST /api/keys.
type CreateKeyResponse struct {
	ID     string `json:"id"`
	PubPEM string `json:"pub_pem"`
}

// CreateKey handles POST /api/keys.
// Generates an RSA-4096 keypair, persists the private key, returns id + public key as JSON.
// Note: RSA-4096 generation takes ~2-3s; this is expected and intentional.
func (h *Handlers) CreateKey(w http.ResponseWriter, r *http.Request) {
	pub, priv := generateKey()
	id, err := h.store.Save(priv)
	if err != nil {
		h.logger.Error("store.Save", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.logger.Info("key created", "id", id)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(CreateKeyResponse{ID: id, PubPEM: string(pub)})
}

// FetchKey handles GET /api/keys/{id}.
// Returns the private key PEM, consuming it (one-time read). Returns 404 if already consumed.
func (h *Handlers) FetchKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	key, err := h.store.Load(id)
	if err != nil {
		h.logger.Error("store.Load", "id", id, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if key == nil {
		http.NotFound(w, r)
		return
	}
	h.logger.Info("key fetched", "id", id)
	w.WriteHeader(http.StatusOK)
	w.Write(key)
}

// HomePage handles GET /.
func (h *Handlers) HomePage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// SharePage handles GET /share/{id}.
func (h *Handlers) SharePage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func generateKey() (public, private []byte) {
	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		panic(err)
	}
	private = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	public = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(key.Public().(*rsa.PublicKey)),
	})
	return
}
```

- [ ] **Step 4: Run handler tests**

```bash
go test ./handlers/... -v
```
Expected: all 6 tests PASS. `TestCreateKeyReturnsJSON` will take ~2-3s due to RSA-4096 generation; this is normal.

- [ ] **Step 5: Commit**

```bash
git add handlers/handlers.go handlers/handlers_test.go
git commit -m "feat: add handlers package with JSON response for POST /api/keys"
```

---

### Task 5: Update `main_test.go`

**Files:**
- Modify: `main_test.go`

The test moves from calling `s.webServer()` directly to routing through a `ServeMux` built from the `handlers` package. `TestPrivateKeyIsOneTime` is unchanged.

- [ ] **Step 1: Replace `main_test.go`**

```go
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
```

- [ ] **Step 2: Run tests**

```bash
go test ./...
```
Expected: all tests PASS. (`main_test.go` compiles and passes; `handlers_test.go` still passes; old `Server`/`webServer` still in `main.go` but not referenced by tests.)

- [ ] **Step 3: Commit**

```bash
git add main_test.go
git commit -m "test: update main_test.go to use handlers ServeMux, new /api/keys/ URL"
```

---

### Task 6: Rewrite `main.go`

**Files:**
- Modify: `main.go`

Remove `Server`, `webServer`, `generateKey`, and all old routing. Wire `loadConfig`, `slog`, `handlers.New`, and the `ServeMux`.

- [ ] **Step 1: Replace `main.go` with the slim version**

```go
package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/josh-zjx/keybank/handlers"
)

func main() {
	cfg := loadConfig()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	store := newRedisKeyStore(cfg.RedisAddr)
	h := handlers.New(store, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/keys", h.CreateKey)
	mux.HandleFunc("GET /api/keys/{id}", h.FetchKey)
	mux.HandleFunc("GET /", h.HomePage)
	mux.HandleFunc("GET /share/{id}", h.SharePage)

	addr := ":" + cfg.Port
	logger.Info("server starting", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		panic(err)
	}
}
```

- [ ] **Step 2: Run all tests**

```bash
go test ./...
```
Expected: all tests PASS. Confirm `go vet ./...` is also clean.

```bash
go vet ./...
```
Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add main.go
git commit -m "refactor: slim main.go — ServeMux routing, slog JSON, handlers package"
```

---

### Task 7: Update `README.md`

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add an API section to `README.md`**

Append after the existing `# Feature` section:

````markdown
# API

## POST /api/keys

Generates an RSA-4096 keypair. Stores the private key server-side and returns the public key to the caller. The caller uses the public key to encrypt a message client-side; the ciphertext is never sent to the server.

**Response `201 Created`:**
```json
{
  "id": "3f2a1b4c...",
  "pub_pem": "-----BEGIN RSA PUBLIC KEY-----\n..."
}
```

| Field | Description |
|---|---|
| `id` | 128-bit random hex ID used to retrieve the private key |
| `pub_pem` | RSA-4096 public key in PKCS#1 PEM format |

## GET /api/keys/{id}

Retrieves and **permanently deletes** the private key for the given ID (one-time read). Returns `404` if the key has already been fetched or never existed.

**Response `200 OK`:** private key PEM bytes (used by the browser to decrypt the message).

## GET /

Create page (shell — UI added in Milestone 4).

## GET /share/{id}

Decryption page for share URL (shell — UI added in Milestone 4).
````

- [ ] **Step 2: Run tests one final time**

```bash
go test ./...
```
Expected: all tests PASS — milestone 0 is complete.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: document API endpoints and JSON response shape in README"
```

---

## Verification

End-to-end smoke test after all tasks:

```bash
# Start server (requires Redis on localhost:6379)
go run .

# In a second terminal — create a key
curl -s -X POST http://localhost:14000/api/keys | jq .
# Expected: {"id":"<hex>","pub_pem":"-----BEGIN RSA PUBLIC KEY-----\n..."}

# Capture the id and fetch the private key (one-time)
ID=$(curl -s -X POST http://localhost:14000/api/keys | jq -r .id)
curl -s http://localhost:14000/api/keys/$ID | head -1
# Expected: -----BEGIN RSA PRIVATE KEY-----

# Second fetch — key is gone
curl -o /dev/null -w "%{http_code}" http://localhost:14000/api/keys/$ID
# Expected: 404

# Server logs are JSON
# {"time":"...","level":"INFO","msg":"key created","id":"..."}
```

Full test suite:
```bash
go test ./... -v
go vet ./...
```
Expected: all green, no vet warnings.
