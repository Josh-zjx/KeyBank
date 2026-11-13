package handlers_test

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/josh-zjx/keybank/handlers"
	"github.com/josh-zjx/keybank/internal/store"
)

// This existing fixture wraps the real store to force failures and races at
// precise boundaries, without changing production storage behavior.
type fakeStore struct {
	handlers.Store
	ttl         time.Duration
	snapshot    []byte
	fail        string
	consumeHook func()
}

func newFakeStore() *fakeStore { return &fakeStore{Store: store.NewMemKeyStore()} }
func (f *fakeStore) Save(ctx context.Context, payload []byte, ttl time.Duration) (string, error) {
	if f.fail == "save" {
		return "", errors.New("backend failure")
	}
	f.ttl = ttl
	f.snapshot = bytes.Clone(payload)
	return f.Store.Save(ctx, payload, ttl)
}
func (f *fakeStore) Get(ctx context.Context, id string) ([]byte, error) {
	if f.fail == "get" {
		return nil, errors.New("backend failure")
	}
	return f.Store.Get(ctx, id)
}
func (f *fakeStore) Consume(ctx context.Context, id string, payload []byte) (bool, error) {
	if f.fail == "consume" {
		return false, errors.New("backend failure")
	}
	if f.consumeHook != nil {
		f.consumeHook()
	}
	return f.Store.Consume(ctx, id, payload)
}
func newTestHandlers(t *testing.T, backend handlers.Store) *handlers.Handlers {
	t.Helper()
	return handlers.New(backend, slog.New(slog.NewTextHandler(io.Discard, nil)), 7*24*time.Hour)
}
func request(app http.Handler, method, path, body, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	return w
}
func post(app http.Handler, path, body string) *httptest.ResponseRecorder {
	return request(app, "POST", path, body, "application/json")
}
func create(t *testing.T, app http.Handler, kind, pass string) handlers.CreateKeyResponse {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"type": kind, "passphrase": pass})
	w := post(app, "/api/v1/keys", string(body))
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	var reply handlers.CreateKeyResponse
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	return reply
}
func fetchPath(id string) string { return "/api/v1/keys/" + id + "/fetch" }

func TestKeyRoundTrips(t *testing.T) {
	for _, backendName := range []string{"mem", "redis"} {
		for _, kind := range []string{"ecc", "aes256"} {
			for _, pass := range []string{"", " a Unicode 🔐 passphrase "} {
				t.Run(backendName+"/"+kind+"/"+map[bool]string{true: "protected", false: "unprotected"}[pass != ""], func(t *testing.T) {
					var backend handlers.Store = store.NewMemKeyStore()
					if backendName == "redis" {
						redis := store.NewRedisKeyStore(miniredis.RunT(t).Addr())
						t.Cleanup(func() { _ = redis.Close() })
						backend = redis
					}
					app := newTestHandlers(t, backend).Handler(nil)
					made := create(t, app, kind, pass)
					if len(made.ID) != 32 || made.Type != kind {
						t.Fatalf("unexpected create: %+v", made)
					}
					if pass != "" {
						for _, bad := range []string{"{}", `{"passphrase":"wrong"}`} {
							if w := post(app, fetchPath(made.ID), bad); w.Code != 403 {
								t.Fatalf("wrong credential: %d %s", w.Code, w.Body)
							}
						}
					}
					body, _ := json.Marshal(map[string]string{"passphrase": pass})
					w := post(app, fetchPath(made.ID), string(body))
					if w.Code != 200 {
						t.Fatalf("fetch: %d %s", w.Code, w.Body)
					}
					if w.Header().Get("Cache-Control") != "no-store" {
						t.Fatal("key is cacheable")
					}
					var got handlers.FetchKeyResponse
					if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
						t.Fatal(err)
					}
					if got.ID != made.ID || got.Type != kind {
						t.Fatal("identity mismatch")
					}
					if kind == "aes256" {
						key, err := base64.RawURLEncoding.Strict().DecodeString(got.Key)
						if err != nil || len(key) != 32 || made.Key != got.Key || got.PrivateKey != "" || made.PublicKey != "" {
							t.Fatal("AES key mismatch")
						}
					} else {
						privRaw, err := base64.RawURLEncoding.Strict().DecodeString(got.PrivateKey)
						if err != nil {
							t.Fatal(err)
						}
						private, err := ecdh.X25519().NewPrivateKey(privRaw)
						if err != nil {
							t.Fatal(err)
						}
						pubRaw, err := base64.RawURLEncoding.Strict().DecodeString(made.PublicKey)
						if err != nil {
							t.Fatal(err)
						}
						if !bytes.Equal(private.PublicKey().Bytes(), pubRaw) || got.PublicKey != made.PublicKey || made.Key != "" || got.Key != "" {
							t.Fatal("ECC keypair mismatch")
						}
						peer, err := ecdh.X25519().GenerateKey(rand.Reader)
						if err != nil {
							t.Fatal(err)
						}
						pub, err := ecdh.X25519().NewPublicKey(pubRaw)
						if err != nil {
							t.Fatal(err)
						}
						a, err := peer.ECDH(pub)
						if err != nil {
							t.Fatal(err)
						}
						b, err := private.ECDH(peer.PublicKey())
						if err != nil || !bytes.Equal(a, b) {
							t.Fatal("ECDH mismatch")
						}
					}
					if w := post(app, fetchPath(made.ID), string(body)); w.Code != 404 {
						t.Fatal("second fetch succeeded")
					}
				})
			}
		}
	}
}

func TestCreateValidationAndTTL(t *testing.T) {
	for _, tc := range []struct {
		body string
		code int
		ttl  time.Duration
	}{
		{`{"type":"ecc"}`, 201, 24 * time.Hour}, {`{"type":"aes256","ttl":0}`, 201, 24 * time.Hour},
		{`{"type":"ecc","ttl":60}`, 201, time.Minute}, {`{"type":"ecc","ttl":604800}`, 201, 7 * 24 * time.Hour},
		{`{"type":"ecc","ttl":59}`, 400, 0}, {`{"type":"ecc","ttl":604801}`, 400, 0},
		{`{"type":"ecc","ttl":-1}`, 400, 0}, {`{"type":"ecc","ttl":1e30}`, 400, 0},
		{`{"type":"ecc","ttl":60.1}`, 400, 0}, {`{}`, 400, 0}, {`{"type":"rsa"}`, 400, 0},
		{``, 400, 0}, {`null`, 400, 0}, {`[]`, 400, 0}, {`{"type":"ecc"} {}`, 400, 0},
		{`{"type":"ecc","extra":1}`, 400, 0}, {`{"type":"ecc","type":"aes256"}`, 400, 0},
		{`{"Type":"ecc"}`, 400, 0}, {`{"type":"ecc","passphrase":null}`, 400, 0},
		{`{"type":"ecc","passphrase":1}`, 400, 0}, {`{"type":"ecc","ttl":null}`, 400, 0},
		{`{"type":"aes256","passphrase":"` + strings.Repeat("a", 1025) + `"}`, 400, 0},
		{`{"type":"ecc","extra":"` + strings.Repeat("a", 17000) + `"}`, 413, 0},
	} {
		t.Run(tc.body[:min(len(tc.body), 60)], func(t *testing.T) {
			backend := newFakeStore()
			app := newTestHandlers(t, backend).Handler(nil)
			w := post(app, "/api/v1/keys", tc.body)
			if w.Code != tc.code {
				t.Fatalf("got %d %s, want %d", w.Code, w.Body, tc.code)
			}
			if backend.ttl != tc.ttl {
				t.Fatalf("TTL %v, want %v", backend.ttl, tc.ttl)
			}
		})
	}
	backend := newFakeStore()
	app := handlers.New(backend, slog.Default(), 2*time.Hour).Handler(nil)
	create(t, app, "ecc", "")
	if backend.ttl != 2*time.Hour {
		t.Fatal("configured default ignored")
	}
	if w := post(app, "/api/v1/keys", `{"type":"ecc","ttl":7201}`); w.Code != 400 {
		t.Fatal("configured max ignored")
	}
}

func TestRejectedFetchDoesNotConsume(t *testing.T) {
	cases := []struct {
		method, body, media string
		code                int
	}{
		{"POST", ``, "application/json", 400}, {"POST", `null`, "application/json", 400},
		{"POST", `{"pub":"legacy"}`, "application/json", 400}, {"POST", `{} {}`, "application/json", 400},
		{"POST", `{"passphrase":null}`, "application/json", 400}, {"POST", `{"passphrase":true}`, "application/json", 400},
		{"POST", `{"passphrase":"a","passphrase":"b"}`, "application/json", 400},
		{"POST", `{"passphrase":"` + strings.Repeat("a", 1025) + `"}`, "application/json", 400},
		{"POST", `{"passphrase":"` + strings.Repeat("a", 17000) + `"}`, "application/json", 413},
		{"POST", `{}`, "text/plain", 415}, {"POST", `{}`, "", 415}, {"GET", `{}`, "application/json", 405},
	}
	for _, tc := range cases {
		t.Run(tc.method+tc.media+tc.body[:min(len(tc.body), 30)], func(t *testing.T) {
			app := newTestHandlers(t, store.NewMemKeyStore()).Handler(nil)
			made := create(t, app, "ecc", "")
			w := request(app, tc.method, fetchPath(made.ID), tc.body, tc.media)
			if w.Code != tc.code {
				t.Fatalf("got %d %s, want %d", w.Code, w.Body, tc.code)
			}
			if w := post(app, fetchPath(made.ID), `{}`); w.Code != 200 {
				t.Fatal("rejection burned key")
			}
		})
	}
}

func TestPassphraseStoredAsSaltedVerifier(t *testing.T) {
	backend := newFakeStore()
	app := newTestHandlers(t, backend).Handler(nil)
	create(t, app, "aes256", "test secret")
	first := bytes.Clone(backend.snapshot)
	create(t, app, "aes256", "test secret")
	var a, b store.Record
	if err := json.Unmarshal(first, &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(backend.snapshot, &b); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(first, []byte("test secret")) || len(a.Salt) != 16 || len(a.Verifier) != 32 || a.Iterations != 600000 || bytes.Equal(a.Salt, b.Salt) || bytes.Equal(a.Verifier, b.Verifier) || bytes.Equal(a.Secret, b.Secret) {
		t.Fatal("invalid verifier or reused randomness")
	}
}

func TestConcurrentAuthenticatedFetch(t *testing.T) {
	for _, name := range []string{"mem", "redis"} {
		t.Run(name, func(t *testing.T) {
			var backend handlers.Store = store.NewMemKeyStore()
			if name == "redis" {
				r := store.NewRedisKeyStore(miniredis.RunT(t).Addr())
				defer r.Close()
				backend = r
			}
			app := newTestHandlers(t, backend).Handler(nil)
			made := create(t, app, "aes256", "correct")
			var wg sync.WaitGroup
			var wins, unexpected atomic.Int32
			for i := 0; i < 12; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					body := `{"passphrase":"correct"}`
					if i%2 == 0 {
						body = `{"passphrase":"wrong"}`
					}
					w := post(app, fetchPath(made.ID), body)
					if w.Code == 200 {
						if i%2 == 0 {
							unexpected.Add(1)
						}
						wins.Add(1)
					} else if w.Code != 403 && w.Code != 404 {
						unexpected.Add(1)
					}
				}(i)
			}
			wg.Wait()
			if wins.Load() != 1 || unexpected.Load() != 0 {
				t.Fatalf("wins %d unexpected %d", wins.Load(), unexpected.Load())
			}
		})
	}
}

func TestExpiryBetweenReadAndConsume(t *testing.T) {
	mr := miniredis.RunT(t)
	r := store.NewRedisKeyStore(mr.Addr())
	defer r.Close()
	backend := &fakeStore{Store: r}
	app := newTestHandlers(t, backend).Handler(nil)
	made := create(t, app, "ecc", "correct")
	backend.consumeHook = func() { mr.FastForward(25 * time.Hour) }
	if w := post(app, fetchPath(made.ID), `{"passphrase":"correct"}`); w.Code != 404 {
		t.Fatalf("expired key returned: %d", w.Code)
	}
}

func TestStorageFailuresAndCorruptRecords(t *testing.T) {
	for _, operation := range []string{"save", "get", "consume"} {
		t.Run(operation, func(t *testing.T) {
			backend := newFakeStore()
			app := newTestHandlers(t, backend).Handler(nil)
			if operation == "save" {
				backend.fail = operation
				if w := post(app, "/api/v1/keys", `{"type":"ecc"}`); w.Code != 500 {
					t.Fatal(w.Code)
				}
				return
			}
			made := create(t, app, "ecc", "")
			backend.fail = operation
			if w := post(app, fetchPath(made.ID), `{}`); w.Code != 500 || strings.Contains(w.Body.String(), "backend") {
				t.Fatal(w.Code)
			}
			backend.fail = ""
			if w := post(app, fetchPath(made.ID), `{}`); w.Code != 200 {
				t.Fatal("failure consumed key")
			}
		})
	}
	backend := store.NewMemKeyStore()
	app := newTestHandlers(t, backend).Handler(nil)
	for _, payload := range []string{`{`, `null`, `{"type":"aes256","secret":"AA=="}`, `{"type":"rsa"}`} {
		id, err := backend.Save(context.Background(), []byte(payload), time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		if w := post(app, fetchPath(id), `{}`); w.Code != 500 {
			t.Fatal(w.Code)
		}
		if got, _ := backend.Get(context.Background(), id); !bytes.Equal(got, []byte(payload)) {
			t.Fatal("corrupt record consumed")
		}
	}
}

func TestRoutesAndMediaType(t *testing.T) {
	app := newTestHandlers(t, store.NewMemKeyStore()).Handler(nil)
	for _, path := range []string{"/", "/share/id", "/static/app.js", "/api/keys", "/api/v1/keys/not-an-id/fetch"} {
		w := post(app, path, `{}`)
		if w.Code != 404 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	if w := request(app, "GET", "/healthz", "", " "); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := request(app, "POST", "/api/v1/keys", `{"type":"ecc"}`, "text/plain"); w.Code != 415 {
		t.Fatal(w.Code)
	}
	if w := request(app, "POST", "/api/v1/keys", `{"type":"ecc"}`, "application/json; charset=utf-8"); w.Code != 201 {
		t.Fatal(w.Code)
	}
}

func TestCORS(t *testing.T) {
	app := newTestHandlers(t, store.NewMemKeyStore()).Handler([]string{"https://notes.example"})
	for _, tc := range []struct {
		origin, method, header string
		status                 int
	}{
		{"https://notes.example", "POST", "content-type", 204},
		{"https://evil.example", "POST", "content-type", 403},
		{"null", "POST", "content-type", 403},
		{"https://notes.example", "DELETE", "content-type", 403},
		{"https://notes.example", "POST", "authorization", 403},
	} {
		t.Run(tc.origin+tc.method+tc.header, func(t *testing.T) {
			req := httptest.NewRequest("OPTIONS", "/api/v1/keys", nil)
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Access-Control-Request-Method", tc.method)
			req.Header.Set("Access-Control-Request-Headers", tc.header)
			w := httptest.NewRecorder()
			app.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatal(w.Code, w.Body)
			}
			if tc.status == 204 && (w.Header().Get("Access-Control-Allow-Origin") != tc.origin || w.Header().Get("Access-Control-Allow-Credentials") != "") {
				t.Fatal("incorrect CORS headers")
			}
		})
	}
	made := create(t, app, "aes256", "")
	req := httptest.NewRequest("POST", fetchPath(made.ID), strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := post(app, fetchPath(made.ID), `{}`); w.Code != 200 {
		t.Fatal("CORS denial consumed key")
	}
	req = httptest.NewRequest("POST", "https://api.example/api/v1/keys", strings.NewReader(`{"type":"ecc"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://api.example")
	w = httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatal("same origin rejected", w.Code)
	}
}

func TestCORSProxyAndSchemeBoundaries(t *testing.T) {
	app := newTestHandlers(t, store.NewMemKeyStore()).Handler(nil)
	for _, tc := range []struct {
		peer, origin, forwarded string
		status                  int
	}{
		{"127.0.0.1:1234", "https://api.example", "https", 201},
		{"10.0.0.2:1234", "https://api.example", "https", 201},
		{"192.0.2.1:1234", "https://api.example", "https", 403},
		{"127.0.0.1:1234", "http://api.example", "https", 403},
		{"127.0.0.1:1234", "https://other.example", "https", 403},
	} {
		req := httptest.NewRequest("POST", "http://api.example/api/v1/keys", strings.NewReader(`{"type":"ecc"}`))
		req.RemoteAddr = tc.peer
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("X-Forwarded-Proto", tc.forwarded)
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("%+v: got %d", tc, w.Code)
		}
	}
}
