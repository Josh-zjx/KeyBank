package handlers

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"time"
)

const (
	minTTL                 = 60 * time.Second
	maxTTL                 = 7 * 24 * time.Hour
	defaultTTL             = 24 * time.Hour
	defaultAutoHideSeconds = 60
)

// VendorScript is a pinned external script reference used in page templates.
type VendorScript struct {
	Src       string
	Integrity string
}

// Store is the minimal persistence interface handlers need.
// storeAdapter in package main satisfies this implicitly.
type Store interface {
	Save(privPEM []byte, ttl time.Duration) (id string, err error)
	Load(id string) (privPEM []byte, err error)
}

// Handlers holds dependencies for all HTTP handler methods.
type Handlers struct {
	store     Store
	logger    *slog.Logger
	templates *template.Template
	autoHide  int
}

// ParseTemplates loads the create/share page templates from fsys.
func ParseTemplates(fsys fs.FS) (*template.Template, error) {
	return template.ParseFS(fsys,
		"templates/base.html",
		"templates/create.html",
		"templates/share.html",
	)
}

// New creates a Handlers with the given store, logger, and templates.
func New(store Store, logger *slog.Logger, templates *template.Template) *Handlers {
	return &Handlers{store: store, logger: logger, templates: templates, autoHide: defaultAutoHideSeconds}
}

func (h *Handlers) SetAutoHideSeconds(seconds int) {
	if seconds > 0 {
		h.autoHide = seconds
	}
}

// CreateKeyResponse is the JSON body returned by POST /api/keys.
type CreateKeyResponse struct {
	ID     string `json:"id"`
	PubPEM string `json:"pub_pem"`
}

type createKeyRequest struct {
	TTL int `json:"ttl"` // seconds; 0 → defaultTTL
}

type pageData struct {
	Title           string
	Page            string
	BodyClass       string
	BodyTemplate    string
	ScriptPath      string
	VendorScripts   []VendorScript
	ShareID         string
	AutoHideSeconds int
}

// CreateKey handles POST /api/keys.
// Generates an RSA-4096 keypair, persists the private key, returns id + public key as JSON.
// Accepts an optional JSON body {"ttl": <seconds>}. Defaults to 24h if absent.
// Returns 400 {"error":"invalid_ttl"} if the TTL is outside [60s, 7d].
func (h *Handlers) CreateKey(w http.ResponseWriter, r *http.Request) {
	var req createKeyRequest
	// Ignore EOF (empty body) — zero value applies the default TTL.
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		// Any other decode error (malformed JSON, etc.) falls through;
		// req.TTL remains 0 and defaults to defaultTTL below.
	}

	ttl := time.Duration(req.TTL) * time.Second
	if ttl == 0 {
		ttl = defaultTTL
	}
	if ttl < minTTL || ttl > maxTTL {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_ttl"})
		return
	}

	pub, priv, err := generateKey()
	if err != nil {
		h.logger.Error("generateKey", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	id, err := h.store.Save(priv, ttl)
	if err != nil {
		h.logger.Error("store.Save", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.logger.Info("key created")
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
		h.logger.Error("store.Load", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if key == nil {
		http.NotFound(w, r)
		return
	}

	h.logger.Info("key fetched")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(key)
}

// HomePage handles GET /.
func (h *Handlers) HomePage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, pageData{
		Title:           "Create a secure note",
		Page:            "create",
		BodyClass:       "page-create",
		BodyTemplate:    "create-body",
		ScriptPath:      "/static/app.js",
		VendorScripts: []VendorScript{{
			Src:       "/static/qrcode.js",
			Integrity: "sha384-ahLw45Nl1X/zUAno5v8A1m7qYEzDIOrQtPeIGkG/a+vFi9BC17OhLEhzDJZUHqN2",
		}},
		AutoHideSeconds: h.autoHide,
	})
}

// SharePage handles GET /share/{id}.
func (h *Handlers) SharePage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, pageData{
		Title:           "Open a shared note",
		Page:            "share",
		BodyClass:       "page-share",
		BodyTemplate:    "share-body",
		ScriptPath:      "/static/app.js",
		ShareID:         r.PathValue("id"),
		AutoHideSeconds: h.autoHide,
	})
}

func (h *Handlers) renderPage(w http.ResponseWriter, data pageData) {
	if h.templates == nil {
		h.logger.Error("templates not configured")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var body bytes.Buffer
	if err := h.templates.ExecuteTemplate(&body, "base", data); err != nil {
		h.logger.Error("templates.ExecuteTemplate", "err", err, "body_template", data.BodyTemplate)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body.Bytes())
}

func generateKey() (public, private []byte, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, err
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	private = pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privBytes,
	})

	pubBytes, err := x509.MarshalPKIXPublicKey(key.Public())
	if err != nil {
		return nil, nil, err
	}
	public = pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	})

	return
}
