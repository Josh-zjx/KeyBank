// Package handlers implements the versioned KeyBank HTTP API, independently of
// the secret-note demo. It never accepts note plaintext or ciphertext.
package handlers

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/josh-zjx/keybank/internal/store"
)

const (
	minTTL        = time.Minute
	maxAllowedTTL = 7 * 24 * time.Hour
	defaultTTL    = 24 * time.Hour
	maxBodyBytes  = 16 * 1024
)

// Store persists immutable payloads and consumes an authenticated snapshot.
type Store interface {
	Save(context.Context, []byte, time.Duration) (string, error)
	Get(context.Context, string) ([]byte, error)
	Consume(context.Context, string, []byte) (bool, error)
}

// Handlers holds API dependencies; no templates or browser assets are needed.
type Handlers struct {
	store  Store
	logger *slog.Logger
	maxTTL time.Duration
}

// New creates API handlers with the deployment's TTL ceiling.
func New(store Store, logger *slog.Logger, maxTTL time.Duration) *Handlers {
	if maxTTL < minTTL || maxTTL > maxAllowedTTL {
		maxTTL = maxAllowedTTL
	}
	return &Handlers{store: store, logger: logger, maxTTL: maxTTL}
}

// CreateKeyResponse exposes only the public half of ECC keys.
type CreateKeyResponse struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	PublicKey string `json:"public_key,omitempty"`
	Key       string `json:"key,omitempty"`
}

// FetchKeyResponse contains key material directly; transport must use TLS.
type FetchKeyResponse struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	PublicKey  string `json:"public_key,omitempty"`
	PrivateKey string `json:"private_key,omitempty"`
	Key        string `json:"key,omitempty"`
}

type createKeyRequest struct {
	Type       string `json:"type"`
	TTL        int64  `json:"ttl"`
	Passphrase string `json:"passphrase"`
}

type fetchKeyRequest struct {
	Passphrase string `json:"passphrase"`
}

// Handler assembles the API, including JSON errors, CORS, and body limits.
func (h *Handlers) Handler(origins []string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/keys", onlyMethod(http.MethodPost, h.CreateKey))
	mux.HandleFunc("/api/v1/keys/{id}/fetch", onlyMethod(http.MethodPost, h.FetchKey))
	mux.HandleFunc("/healthz", onlyMethod(http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		h.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { writeJSONError(w, 404, "not_found") })
	return Chain(PanicRecover(h.logger), SecurityHeaders(), CORS(origins), MaxBodySize(maxBodyBytes))(mux)
}

func onlyMethod(method string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.Header().Set("Allow", method)
			writeJSONError(w, 405, "method_not_allowed")
			return
		}
		next(w, r)
	}
}

// CreateKey generates independent random key material with an optional gate.
func (h *Handlers) CreateKey(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var req createKeyRequest
	if err := decodeJSON(r.Body, &req, "type", "ttl", "passphrase"); err != nil {
		h.writeDecodeError(w, err)
		return
	}
	if req.Type != "ecc" && req.Type != "aes256" {
		writeJSONError(w, 400, "invalid_type")
		return
	}
	if len(req.Passphrase) > 1024 {
		writeJSONError(w, 400, "invalid_request")
		return
	}
	ttl := defaultTTL
	if ttl > h.maxTTL {
		ttl = h.maxTTL
	}
	if req.TTL != 0 {
		if req.TTL < int64(minTTL/time.Second) || req.TTL > int64(h.maxTTL/time.Second) {
			writeJSONError(w, 400, "invalid_ttl")
			return
		}
		ttl = time.Duration(req.TTL) * time.Second
	}
	public, secret, err := generateKey(req.Type)
	if err != nil {
		h.internalError(w, "generate key")
		return
	}
	rec := store.Record{Type: req.Type, Secret: secret, CreatedAt: time.Now().UTC()}
	if err := protectRecord(&rec, req.Passphrase); err != nil {
		h.internalError(w, "protect key")
		return
	}
	payload, err := json.Marshal(rec) // #nosec G117 -- Persistence intentionally contains key material; this payload is never an HTTP response.
	if err != nil {
		h.internalError(w, "encode record")
		return
	}
	id, err := h.store.Save(r.Context(), payload, ttl)
	if err != nil {
		h.internalError(w, "save key")
		return
	}
	resp := CreateKeyResponse{ID: id, Type: req.Type}
	if req.Type == "ecc" {
		resp.PublicKey = keyEncoding.EncodeToString(public)
	} else {
		resp.Key = keyEncoding.EncodeToString(secret)
	}
	h.writeJSON(w, http.StatusCreated, resp)
}

// FetchKey authenticates before atomically consuming the exact record read.
func (h *Handlers) FetchKey(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var req fetchKeyRequest
	if err := decodeJSON(r.Body, &req, "passphrase"); err != nil {
		h.writeDecodeError(w, err)
		return
	}
	if len(req.Passphrase) > 1024 {
		writeJSONError(w, 400, "invalid_request")
		return
	}
	id := r.PathValue("id")
	decoded, err := hex.DecodeString(id)
	if err != nil || len(decoded) != 16 || hex.EncodeToString(decoded) != id {
		writeJSONError(w, 404, "not_found")
		return
	}
	payload, err := h.store.Get(r.Context(), id)
	if err != nil {
		h.internalError(w, "read key")
		return
	}
	if payload == nil {
		writeJSONError(w, 404, "not_found")
		return
	}
	var rec store.Record
	if err := json.Unmarshal(payload, &rec); err != nil {
		h.internalError(w, "decode record")
		return
	}
	if err := validateRecord(rec); err != nil {
		h.internalError(w, "validate record")
		return
	}
	if !matchesPassphrase(rec, req.Passphrase) {
		writeJSONError(w, 403, "invalid_passphrase")
		return
	}
	resp := FetchKeyResponse{ID: id, Type: rec.Type}
	if rec.Type == "ecc" {
		key, err := ecdh.X25519().NewPrivateKey(rec.Secret)
		if err != nil {
			h.internalError(w, "decode key")
			return
		}
		resp.PrivateKey = keyEncoding.EncodeToString(rec.Secret)
		resp.PublicKey = keyEncoding.EncodeToString(key.PublicKey().Bytes())
	} else {
		resp.Key = keyEncoding.EncodeToString(rec.Secret)
	}
	body, err := json.Marshal(resp) // #nosec G117 -- This API intentionally returns private keys over TLS, only after authentication and successful consumption below.
	if err != nil {
		h.internalError(w, "encode response")
		return
	}
	consumed, err := h.store.Consume(r.Context(), id, payload)
	if err != nil {
		h.internalError(w, "consume key")
		return
	}
	if !consumed {
		writeJSONError(w, 404, "not_found")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func requireJSON(w http.ResponseWriter, r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeJSONError(w, 415, "unsupported_media_type")
		return false
	}
	return true
}

// decodeJSON requires one object, exact field names, no duplicates or nulls,
// valid UTF-8, and no trailing data. The middleware bounds the read.
func decodeJSON(body io.Reader, dst any, fields ...string) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	if !utf8.Valid(data) {
		return errors.New("invalid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return errors.New("expected object")
	}
	allowed := make(map[string]bool, len(fields))
	for _, field := range fields {
		allowed[field] = true
	}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		name, ok := token.(string)
		if !ok || !allowed[name] {
			return errors.New("unknown or duplicate field")
		}
		delete(allowed, name)
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return errors.New("null field")
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("trailing data")
	}
	return json.Unmarshal(data, dst)
}

func (h *Handlers) writeJSON(w http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		h.internalError(w, "encode response")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func (h *Handlers) internalError(w http.ResponseWriter, operation string) {
	// Never log record payloads, IDs, credentials, or backend errors containing them.
	h.logger.Error(operation)
	writeJSONError(w, http.StatusInternalServerError, "internal_error")
}

func (h *Handlers) writeDecodeError(w http.ResponseWriter, err error) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		writeJSONError(w, 413, "request_body_too_large")
		return
	}
	writeJSONError(w, 400, "invalid_request")
}

func writeJSONError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
