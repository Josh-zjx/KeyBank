package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
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

// MemKeyStore is an in-memory KeyStore used in tests.
type MemKeyStore struct {
	mu   sync.Mutex
	data map[string]memEntry
}

func NewMemKeyStore() *MemKeyStore {
	return &MemKeyStore{data: make(map[string]memEntry)}
}

func (m *MemKeyStore) Save(rec Record, ttl time.Duration) (string, error) {
	id, err := randomID()
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	m.data[id] = memEntry{rec: rec, expiry: time.Now().Add(ttl)}
	m.mu.Unlock()
	return id, nil
}

func (m *MemKeyStore) Load(id string) (*Record, error) {
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

// RedisKeyStore implements KeyStore using Redis GETDEL for atomic one-time semantics.
type RedisKeyStore struct {
	client *goredis.Client
}

func NewRedisKeyStore(addr string) *RedisKeyStore {
	return &RedisKeyStore{
		client: goredis.NewClient(&goredis.Options{Addr: addr}),
	}
}

func (r *RedisKeyStore) Save(rec Record, ttl time.Duration) (string, error) {
	payload, err := json.Marshal(rec)
	if err != nil {
		return "", err
	}
	// A future refactor should thread request-scoped context through the KeyStore interface.
	ctx := context.Background()
	for attempt := 0; attempt < 3; attempt++ {
		id, err := randomID()
		if err != nil {
			return "", err
		}
		ok, err := r.client.SetNX(ctx, id, payload, ttl).Result()
		if err != nil {
			return "", err
		}
		if ok {
			return id, nil
		}
	}
	return "", errors.New("store: failed to allocate unique id")
}

func (r *RedisKeyStore) Load(id string) (*Record, error) {
	// A future refactor should thread request-scoped context through the KeyStore interface.
	val, err := r.client.GetDel(context.Background(), id).Bytes()
	if errors.Is(err, goredis.Nil) {
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

// Client returns the underlying Redis client for advanced use cases (e.g., rate limiting).
func (r *RedisKeyStore) Client() *goredis.Client {
	return r.client
}

// StoreAdapter bridges KeyStore (package store) to handlers.Store (package handlers).
// handlers.Store works with raw []byte to avoid an import cycle.
type StoreAdapter struct{ Ks KeyStore }

func (a *StoreAdapter) Save(privPEM []byte, ttl time.Duration) (string, error) {
	rec := Record{PrivPEM: string(privPEM), CreatedAt: time.Now()}
	return a.Ks.Save(rec, ttl)
}

func (a *StoreAdapter) Load(id string) ([]byte, error) {
	rec, err := a.Ks.Load(id)
	if err != nil || rec == nil {
		return nil, err
	}
	return []byte(rec.PrivPEM), nil
}
