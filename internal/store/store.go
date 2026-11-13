package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	goredis "github.com/go-redis/redis/v8"
)

const namespace = "keybank:v1:keys:"

// memEntry owns a copy of an immutable payload and its expiry.
type memEntry struct {
	payload []byte
	expiry  time.Time
}

// MemKeyStore is a process-local key store for development and tests.
type MemKeyStore struct {
	mu   sync.Mutex
	data map[string]memEntry
}

// NewMemKeyStore creates an isolated in-memory store.
func NewMemKeyStore() *MemKeyStore {
	return &MemKeyStore{data: make(map[string]memEntry)}
}

// Save copies a payload and assigns a fresh ID and expiry.
func (m *MemKeyStore) Save(ctx context.Context, payload []byte, ttl time.Duration) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	for attempt := 0; attempt < 3; attempt++ {
		id, err := randomID()
		if err != nil {
			return "", err
		}
		m.mu.Lock()
		_, exists := m.data[id]
		if !exists {
			m.data[id] = memEntry{payload: bytes.Clone(payload), expiry: time.Now().Add(ttl)}
		}
		m.mu.Unlock()
		if !exists {
			return id, nil
		}
	}
	return "", errors.New("store: failed to allocate unique id")
}

// Get reads without consuming; callers must authenticate before Consume.
func (m *MemKeyStore) Get(ctx context.Context, id string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.data[id]
	if !ok {
		return nil, nil
	}
	if !time.Now().Before(entry.expiry) {
		delete(m.data, id)
		return nil, nil
	}
	return bytes.Clone(entry.payload), nil
}

// Consume deletes only if the live payload is exactly the authenticated snapshot.
func (m *MemKeyStore) Consume(ctx context.Context, id string, expected []byte) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.data[id]
	if !ok {
		return false, nil
	}
	if !time.Now().Before(entry.expiry) {
		delete(m.data, id)
		return false, nil
	}
	if !bytes.Equal(entry.payload, expected) {
		return false, nil
	}
	delete(m.data, id)
	return true, nil
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// RedisKeyStore stores immutable payloads with TTLs and conditional consumption.
type RedisKeyStore struct{ client *goredis.Client }

// NewRedisKeyStore creates a Redis client; connections are opened on demand.
func NewRedisKeyStore(addr string) *RedisKeyStore {
	return &RedisKeyStore{client: goredis.NewClient(&goredis.Options{Addr: addr})}
}

// Save allocates a fresh namespaced ID without overwriting an existing key.
func (r *RedisKeyStore) Save(ctx context.Context, payload []byte, ttl time.Duration) (string, error) {
	for attempt := 0; attempt < 3; attempt++ {
		id, err := randomID()
		if err != nil {
			return "", err
		}
		ok, err := r.client.SetNX(ctx, namespace+id, payload, ttl).Result()
		if err != nil {
			return "", err
		}
		if ok {
			return id, nil
		}
	}
	return "", errors.New("store: failed to allocate unique id")
}

// Get reads the payload without modifying its expiry or consuming it.
func (r *RedisKeyStore) Get(ctx context.Context, id string) ([]byte, error) {
	payload, err := r.client.Get(ctx, namespace+id).Bytes()
	if errors.Is(err, goredis.Nil) {
		return nil, nil
	}
	return payload, err
}

// This script performs no writes that allocate a Redis value, allowing reads
// to free existing keys even at the noeviction memory limit.
const consumeScript = `
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0
`

// Consume atomically deletes only the exact live snapshot the caller verified.
func (r *RedisKeyStore) Consume(ctx context.Context, id string, expected []byte) (bool, error) {
	n, err := r.client.Eval(ctx, consumeScript, []string{namespace + id}, expected).Int()
	return n == 1, err
}

// Close releases the Redis client's connections on shutdown.
func (r *RedisKeyStore) Close() error { return r.client.Close() }
