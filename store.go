package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	goredis "github.com/go-redis/redis/v8"
)

type KeyStore interface {
	Save(rec Record, ttl time.Duration) (id string, err error)
	Load(id string) (*Record, error)
}

// memEntry holds a Record with its expiry for TTL enforcement.
type memEntry struct {
	rec    Record
	expiry time.Time
}

// memKeyStore is an in-memory KeyStore used in tests.
type memKeyStore struct {
	mu   sync.Mutex
	data map[string]memEntry
}

func newMemKeyStore() *memKeyStore {
	return &memKeyStore{data: make(map[string]memEntry)}
}

func (m *memKeyStore) Save(rec Record, ttl time.Duration) (string, error) {
	id, err := randomID()
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	m.data[id] = memEntry{rec: rec, expiry: time.Now().Add(ttl)}
	m.mu.Unlock()
	return id, nil
}

func (m *memKeyStore) Load(id string) (*Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.data[id]
	if !ok {
		return nil, nil
	}
	delete(m.data, id)
	if time.Now().After(entry.expiry) {
		return nil, nil
	}
	rec := entry.rec
	return &rec, nil
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// redisKeyStore implements KeyStore using Redis GETDEL for atomic one-time semantics.
type redisKeyStore struct {
	client *goredis.Client
}

func newRedisKeyStore(addr string) *redisKeyStore {
	return &redisKeyStore{
		client: goredis.NewClient(&goredis.Options{Addr: addr}),
	}
}

func (r *redisKeyStore) Save(rec Record, ttl time.Duration) (string, error) {
	id, err := randomID()
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(rec)
	if err != nil {
		return "", err
	}
	if err := r.client.Set(context.Background(), id, payload, ttl).Err(); err != nil {
		return "", err
	}
	return id, nil
}

func (r *redisKeyStore) Load(id string) (*Record, error) {
	val, err := r.client.GetDel(context.Background(), id).Bytes()
	if err == goredis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rec Record
	if err := json.Unmarshal(val, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// storeAdapter bridges KeyStore (package main) to handlers.Store (package handlers).
// handlers.Store works with raw []byte to avoid an import cycle.
type storeAdapter struct{ ks KeyStore }

func (a *storeAdapter) Save(privPEM []byte, ttl time.Duration) (string, error) {
	rec := Record{PrivPEM: string(privPEM), CreatedAt: time.Now()}
	return a.ks.Save(rec, ttl)
}

func (a *storeAdapter) Load(id string) ([]byte, error) {
	rec, err := a.ks.Load(id)
	if err != nil || rec == nil {
		return nil, err
	}
	return []byte(rec.PrivPEM), nil
}
