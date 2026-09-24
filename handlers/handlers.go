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
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"time"
)

const (
	minTTL                 = 60 * time.Second
	maxAllowedTTL          = 7 * 24 * time.Hour
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
	maxTTL    time.Duration
}

// ParseTemplates loads the create/share page templates from fsys.
func ParseTemplates(fsys fs.FS) (*template.Template, error) {
	return template.ParseFS(fsys,
		"templates/base.html",
		"templates/create.html",
		"templates/error.html",
		"templates/share.html",
	)
}

// New creates a Handlers with the given store, logger, and templates.
func New(store Store, logger *slog.Logger, templates *template.Template) *Handlers {
	return &Handlers{
		store: store, logger: logger, templates: templates,
		autoHide: defaultAutoHideSeconds,
		maxTTL:   maxAllowedTTL,
	}
}

func (h *Handlers) SetAutoHideSeconds(seconds int) {
	if seconds > 0 {
		h.autoHide = seconds
	}
}

// SetMaxTTL sets the deployment-specific expiry ceiling without allowing the
// service-wide seven-day maximum to be exceeded.
func (h *Handlers) SetMaxTTL(ttl time.Duration) {
	if ttl >= minTTL && ttl <= maxAllowedTTL {
		h.maxTTL = ttl
	}
}

// CreateKeyResponse is the JSON body returned by POST /api/keys.
type CreateKeyResponse struct {
	ID     string `json:"id"`
	PubPEM string `json:"pub_pem"`
}

type createKeyRequest struct {
	TTL int64 `json:"ttl"` // seconds; 0 → defaultTTL
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
	MaxTTLSeconds   int64
	StatusCode      int
}

// CreateKey handles POST /api/keys.
// Generates an RSA-4096 keypair, persists the private key, returns id + public key as JSON.
// Accepts an optional JSON body {"ttl": <seconds>}. The default is the lesser
// of 24 hours and the configured maximum. Invalid JSON returns invalid_request.
// TTL values outside the configured [60s, maximum] range return invalid_ttl.
func (h *Handlers) CreateKey(w http.ResponseWriter, r *http.Request) {
	var req *createKeyRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		h.writeDecodeError(w, err)
		return
	} else if err == nil {
		if req == nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		// Only one JSON value is valid; reject trailing values or garbage.
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			h.writeDecodeError(w, err)
			return
		}
	}
	if req == nil {
		req = &createKeyRequest{}
	}

	var ttl time.Duration
	if req.TTL == 0 {
		ttl = defaultTTL
		if ttl > h.maxTTL {
			ttl = h.maxTTL
		}
	} else if req.TTL < int64(minTTL/time.Second) || req.TTL > int64(h.maxTTL/time.Second) {
		writeJSONError(w, http.StatusBadRequest, "invalid_ttl")
		return
	} else {
		ttl = time.Duration(req.TTL) * time.Second
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

func (h *Handlers) writeDecodeError(w http.ResponseWriter, err error) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		writeJSONError(w, http.StatusRequestEntityTooLarge, "request_body_too_large")
		return
	}
	writeJSONError(w, http.StatusBadRequest, "invalid_request")
}

func writeJSONError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
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
	// #nosec G705 -- /api/keys/{id} intentionally returns PEM bytes, not HTML.
	_, _ = w.Write(key)
}

// HomePage handles GET /.
func (h *Handlers) HomePage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, pageData{
		Title:        "Create a secure note",
		Page:         "create",
		BodyClass:    "page-create",
		BodyTemplate: "create-body",
		ScriptPath:   "/static/app.js",
		VendorScripts: []VendorScript{{
			Src:       "/static/qrcode.js",
			Integrity: "sha384-ahLw45Nl1X/zUAno5v8A1m7qYEzDIOrQtPeIGkG/a+vFi9BC17OhLEhzDJZUHqN2",
		}},
		AutoHideSeconds: h.autoHide,
		MaxTTLSeconds:   int64(h.maxTTL / time.Second),
	})
}

// NotFoundPage renders the placeholder page for unknown GET routes.
func (h *Handlers) NotFoundPage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, pageData{
		Title:        "Page not found",
		Page:         "error",
		BodyClass:    "page-error",
		BodyTemplate: "error-body",
		StatusCode:   http.StatusNotFound,
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
	status := data.StatusCode
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
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
