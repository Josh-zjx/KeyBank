// Package demo serves a secret-note application that uses the public HTTP API.
package demo

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/josh-zjx/keybank/handlers"
)

//go:embed templates/*.html static/*
var assets embed.FS

type pageData struct {
	Title           string
	Page            string
	BodyClass       string
	BodyTemplate    string
	ScriptPath      string
	ShareID         string
	AutoHideSeconds int
	MaxTTLSeconds   int64
	APIBaseURL      string
	StatusCode      int
}

// NewHandler serves only demo pages and assets, never stores or fetches keys.
func NewHandler(apiURL string, autoHide int, maxTTL time.Duration, logger *slog.Logger) (http.Handler, error) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return nil, fmt.Errorf("invalid API_BASE_URL")
	}
	connect := "'self'"
	if apiURL != "/api/v1" {
		if (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || strings.TrimSuffix(u.Path, "/") != "/api/v1" || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(u.Host, " ;'\"\\\r\n\t") {
			return nil, fmt.Errorf("API_BASE_URL must be /api/v1 or an absolute HTTP(S) API URL")
		}
		connect += " " + u.Scheme + "://" + u.Host
	}
	apiURL = strings.TrimSuffix(apiURL, "/")
	templates, err := template.ParseFS(assets, "templates/*.html")
	if err != nil {
		return nil, err
	}
	render := func(w http.ResponseWriter, data pageData) {
		data.AutoHideSeconds = autoHide
		data.MaxTTLSeconds = int64(maxTTL / time.Second)
		data.APIBaseURL = apiURL
		var body bytes.Buffer
		if err := templates.ExecuteTemplate(&body, "base", data); err != nil {
			logger.Error("render demo")
			http.Error(w, "internal error", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if data.StatusCode == 0 {
			data.StatusCode = 200
		}
		w.WriteHeader(data.StatusCode)
		_, _ = w.Write(body.Bytes())
	}
	mux := http.NewServeMux()
	static, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, err
	}
	mux.Handle("GET /static/", handlers.StaticHeaders()(http.StripPrefix("/static/", http.FileServer(http.FS(static)))))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		render(w, pageData{Title: "Create a secure note", Page: "create", BodyClass: "page-create", BodyTemplate: "create-body", ScriptPath: "/static/app.js"})
	})
	mux.HandleFunc("GET /share/{id}", func(w http.ResponseWriter, r *http.Request) {
		render(w, pageData{Title: "Open a shared note", Page: "share", BodyClass: "page-share", BodyTemplate: "share-body", ScriptPath: "/static/app.js", ShareID: r.PathValue("id")})
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		render(w, pageData{Title: "Page not found", Page: "error", BodyClass: "page-error", BodyTemplate: "error-body", StatusCode: 404})
	})
	secure := handlers.SecurityHeaders()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; connect-src "+connect+"; object-src 'none'; base-uri 'none'; form-action 'self'")
		if strings.HasPrefix(r.URL.Path, "/static/") {
			w.Header().Del("Cache-Control")
		}
		mux.ServeHTTP(w, r)
	}))
	return handlers.PanicRecover(logger)(secure), nil
}
