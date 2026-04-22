package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	goredis "github.com/go-redis/redis/v8"
)

type KeyStore interface {
	Save(key []byte) (id string, err error)
	Load(id string) (key []byte, err error)
}

// memKeyStore is an in-memory KeyStore used in tests.
type memKeyStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMemKeyStore() *memKeyStore {
	return &memKeyStore{data: make(map[string][]byte)}
}

func (m *memKeyStore) Save(key []byte) (string, error) {
	id, err := randomID()
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	m.data[id] = key
	m.mu.Unlock()
	return id, nil
}

func (m *memKeyStore) Load(id string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key, ok := m.data[id]
	if !ok {
		return nil, nil
	}
	delete(m.data, id)
	return key, nil
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
