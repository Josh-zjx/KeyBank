package store_test

import (
	"bytes"
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/josh-zjx/keybank/handlers"
	"github.com/josh-zjx/keybank/internal/store"
)

func TestStoreContract(t *testing.T) {
	for _, name := range []string{"mem", "redis"} {
		t.Run(name, func(t *testing.T) {
			var backend handlers.Store = store.NewMemKeyStore()
			expire := func() { time.Sleep(100 * time.Millisecond) }
			if name == "redis" {
				mr := miniredis.RunT(t)
				r := store.NewRedisKeyStore(mr.Addr())
				defer r.Close()
				backend = r
				expire = func() { mr.FastForward(time.Hour) }
			}
			ctx := context.Background()
			t.Run("immutable snapshots and conditional consume", func(t *testing.T) {
				original := []byte(`{"secret":"unchanged"}`)
				id, err := backend.Save(ctx, original, time.Hour)
				if err != nil {
					t.Fatal(err)
				}
				original[0] = 'X'
				snapshot, err := backend.Get(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(snapshot, []byte(`{"secret":"unchanged"}`)) {
					t.Fatal("save aliased caller memory")
				}
				snapshot[0] = 'X'
				snapshot, _ = backend.Get(ctx, id)
				if snapshot[0] != '{' {
					t.Fatal("read aliased stored bytes")
				}
				if ok, err := backend.Consume(ctx, id, []byte("wrong snapshot")); err != nil || ok {
					t.Fatal("consumed mismatched snapshot")
				}
				if ok, err := backend.Consume(ctx, id, snapshot); err != nil || !ok {
					t.Fatal("failed to consume snapshot")
				}
				if got, err := backend.Get(ctx, id); err != nil || got != nil {
					t.Fatal("key still present")
				}
				if ok, err := backend.Consume(ctx, id, snapshot); err != nil || ok {
					t.Fatal("consumed twice")
				}
			})
			t.Run("TTL does not refresh on reads or failed consumes", func(t *testing.T) {
				id, err := backend.Save(ctx, []byte("expiring"), 40*time.Millisecond)
				if err != nil {
					t.Fatal(err)
				}
				snapshot, _ := backend.Get(ctx, id)
				if snapshot == nil {
					t.Fatal("expired too early")
				}
				_, _ = backend.Consume(ctx, id, []byte("incorrect"))
				expire()
				if ok, err := backend.Consume(ctx, id, snapshot); err != nil || ok {
					t.Fatal("consumed after expiry")
				}
				if got, _ := backend.Get(ctx, id); got != nil {
					t.Fatal("read after expiry")
				}
			})
			t.Run("exactly one concurrent consumer", func(t *testing.T) {
				id, err := backend.Save(ctx, []byte("one-time"), time.Hour)
				if err != nil {
					t.Fatal(err)
				}
				snapshot, _ := backend.Get(ctx, id)
				var wg sync.WaitGroup
				var wins, failures atomic.Int32
				for i := 0; i < 30; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						ok, err := backend.Consume(ctx, id, snapshot)
						if err != nil {
							failures.Add(1)
						}
						if ok {
							wins.Add(1)
						}
					}()
				}
				wg.Wait()
				if wins.Load() != 1 || failures.Load() != 0 {
					t.Fatalf("wins %d errors %d", wins.Load(), failures.Load())
				}
			})
			t.Run("cancelled context", func(t *testing.T) {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				if _, err := backend.Save(cancelled, []byte("x"), time.Hour); err == nil {
					t.Fatal("ignored cancellation")
				}
				if _, err := backend.Get(cancelled, "id"); err == nil {
					t.Fatal("ignored cancellation")
				}
				if _, err := backend.Consume(cancelled, "id", nil); err == nil {
					t.Fatal("ignored cancellation")
				}
			})
		})
	}
}

func TestRedisNamespaceAndErrors(t *testing.T) {
	mr := miniredis.RunT(t)
	r := store.NewRedisKeyStore(mr.Addr())
	defer r.Close()
	ctx := context.Background()
	id, err := r.Save(ctx, []byte("value"), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !mr.Exists("keybank:v1:keys:"+id) || mr.Exists(id) {
		t.Fatal("wrong namespace")
	}
	mr.Set("legacy-id", "old record")
	if got, err := r.Get(ctx, "legacy-id"); err != nil || got != nil {
		t.Fatal("legacy record exposed")
	}
	_ = r.Close()
	if _, err := r.Save(ctx, []byte("x"), time.Hour); err == nil {
		t.Fatal("save swallowed error")
	}
	if _, err := r.Get(ctx, id); err == nil {
		t.Fatal("get swallowed error")
	}
	if ok, err := r.Consume(ctx, id, []byte("value")); err == nil || ok {
		t.Fatal("consume swallowed error")
	}
}
