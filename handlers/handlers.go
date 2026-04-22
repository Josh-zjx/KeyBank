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
	pub, priv, err := generateKey()
	if err != nil {
		h.logger.Error("generateKey", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	id, err := h.store.Save(priv)
	if err != nil {
		h.logger.Error("store.Save", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.logger.Info("key created", "id", id)
	body, err := json.Marshal(CreateKeyResponse{ID: id, PubPEM: string(pub)})
	if err != nil {
		h.logger.Error("json.Marshal", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(body)
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
	_, _ = w.Write(key)
}

// HomePage handles GET /.
func (h *Handlers) HomePage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// SharePage handles GET /share/{id}.
func (h *Handlers) SharePage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func generateKey() (public, private []byte, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, err
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
