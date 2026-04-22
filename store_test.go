package main

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

// --- memKeyStore tests ---

func TestMemStoreRoundTrip(t *testing.T) {
	store := newMemKeyStore()
	rec := Record{PrivPEM: "priv-pem-data", CreatedAt: time.Now()}

	id, err := store.Save(rec, time.Hour)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Load(id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got == nil {
		t.Fatal("Load returned nil, want record")
	}
	if got.PrivPEM != rec.PrivPEM {
		t.Errorf("PrivPEM: got %q, want %q", got.PrivPEM, rec.PrivPEM)
	}
}

func TestMemStoreIsOneTime(t *testing.T) {
	store := newMemKeyStore()
	id, _ := store.Save(Record{PrivPEM: "pem", CreatedAt: time.Now()}, time.Hour)

	first, err := store.Load(id)
	if err != nil || first == nil {
		t.Fatalf("first Load: got (%v, %v), want non-nil", first, err)
	}

	second, err := store.Load(id)
	if err != nil {
		t.Fatalf("second Load error: %v", err)
	}
	if second != nil {
		t.Errorf("second Load should return nil, got %+v", second)
	}
}

func TestMemStoreTTLExpiry(t *testing.T) {
	store := newMemKeyStore()
	id, _ := store.Save(Record{PrivPEM: "pem", CreatedAt: time.Now()}, 10*time.Millisecond)

	time.Sleep(20 * time.Millisecond)

	got, err := store.Load(id)
	if err != nil {
		t.Fatalf("Load after expiry: %v", err)
	}
	if got != nil {
		t.Errorf("Load after TTL expiry: got %+v, want nil", got)
	}
}

func TestMemStoreConcurrentLoad(t *testing.T) {
	store := newMemKeyStore()
	id, _ := store.Save(Record{PrivPEM: "pem", CreatedAt: time.Now()}, time.Hour)

	const goroutines = 20
	var wins atomic.Int32
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()
			rec, _ := store.Load(id)
			if rec != nil {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()

	if wins.Load() != 1 {
		t.Errorf("exactly 1 goroutine should win the Load, got %d", wins.Load())
	}
}

// --- redisKeyStore tests (via miniredis) ---

func TestRedisStoreRoundTrip(t *testing.T) {
	mr := miniredis.RunT(t)
	store := newRedisKeyStore(mr.Addr())

	rec := Record{PrivPEM: "redis-priv-pem", CreatedAt: time.Now().Truncate(time.Second)}
	id, err := store.Save(rec, time.Hour)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Load(id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got == nil {
		t.Fatal("Load returned nil, want record")
	}
	if got.PrivPEM != rec.PrivPEM {
		t.Errorf("PrivPEM: got %q, want %q", got.PrivPEM, rec.PrivPEM)
	}
}

func TestRedisStoreIsOneTime(t *testing.T) {
	mr := miniredis.RunT(t)
	store := newRedisKeyStore(mr.Addr())

	id, _ := store.Save(Record{PrivPEM: "pem", CreatedAt: time.Now()}, time.Hour)

	first, err := store.Load(id)
	if err != nil || first == nil {
		t.Fatalf("first Load: got (%v, %v), want non-nil", first, err)
	}

	second, err := store.Load(id)
	if err != nil {
		t.Fatalf("second Load error: %v", err)
	}
	if second != nil {
		t.Errorf("second Load should return nil, got %+v", second)
	}
}

func TestRedisStoreTTLHonored(t *testing.T) {
	mr := miniredis.RunT(t)
	store := newRedisKeyStore(mr.Addr())

	id, err := store.Save(Record{PrivPEM: "pem", CreatedAt: time.Now()}, 5*time.Second)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Advance miniredis clock past the TTL.
	mr.FastForward(6 * time.Second)

	got, err := store.Load(id)
	if err != nil {
		t.Fatalf("Load after TTL: %v", err)
	}
	if got != nil {
		t.Errorf("Load after TTL expiry: got %+v, want nil", got)
	}
}

func TestRedisStoreNotFound(t *testing.T) {
	mr := miniredis.RunT(t)
	store := newRedisKeyStore(mr.Addr())

	got, err := store.Load("no-such-id")
	if err != nil {
		t.Fatalf("Load non-existent key: %v", err)
	}
	if got != nil {
		t.Errorf("Load non-existent: got %+v, want nil", got)
	}
}

func TestRedisStoreLoadCorruptedJSON(t *testing.T) {
	mr := miniredis.RunT(t)
	store := newRedisKeyStore(mr.Addr())

	// Inject a raw non-JSON value so Unmarshal returns an error.
	if err := mr.Set("corrupt-id", "not-valid-json"); err != nil {
		t.Fatalf("miniredis.Set: %v", err)
	}

	_, err := store.Load("corrupt-id")
	if err == nil {
		t.Error("Load on corrupted JSON: expected error, got nil")
	}
}

func TestRedisStoreSaveConnectionError(t *testing.T) {
	mr := miniredis.RunT(t)
	addr := mr.Addr()
	store := newRedisKeyStore(addr)
	mr.Close()

	_, err := store.Save(Record{PrivPEM: "pem", CreatedAt: time.Now()}, time.Hour)
	if err == nil {
		t.Error("Save with closed Redis: expected error, got nil")
	}
}

func TestRedisStoreLoadConnectionError(t *testing.T) {
	mr := miniredis.RunT(t)
	addr := mr.Addr()
	store := newRedisKeyStore(addr)

	// Save while Redis is up, then close it before Load.
	id, _ := store.Save(Record{PrivPEM: "pem", CreatedAt: time.Now()}, time.Hour)
	mr.Close()

	_, err := store.Load(id)
	if err == nil {
		t.Error("Load with closed Redis: expected error, got nil")
	}
}

// --- storeAdapter tests ---

func TestStoreAdapterRoundTrip(t *testing.T) {
	ks := newMemKeyStore()
	adapter := &storeAdapter{ks: ks}

	privPEM := []byte("-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----\n")
	id, err := adapter.Save(privPEM, time.Hour)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := adapter.Load(id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if string(got) != string(privPEM) {
		t.Errorf("Load: got %q, want %q", got, privPEM)
	}
}

func TestStoreAdapterNotFound(t *testing.T) {
	adapter := &storeAdapter{ks: newMemKeyStore()}

	got, err := adapter.Load("no-such-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("got %q, want nil", got)
	}
}
