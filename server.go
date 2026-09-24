package main

import (
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/josh-zjx/keybank/handlers"
)

func newAppHandler(assetFS fs.FS, store KeyStore, logger *slog.Logger, createLimiter, fetchLimiter handlers.RateLimiter, autoHideSeconds int, maxTTL time.Duration, trustXFF bool) (http.Handler, error) {
	templates, err := handlers.ParseTemplates(assetFS)
	if err != nil {
		return nil, err
	}

	h := handlers.New(&storeAdapter{ks: store}, logger, templates)
	h.SetAutoHideSeconds(autoHideSeconds)
	h.SetMaxTTL(maxTTL)

	staticFS, err := fs.Sub(assetFS, "static")
	if err != nil {
		return nil, err
	}

	// Separate handler for static files: lighter security headers, cacheable
	staticMux := http.NewServeMux()
	staticMux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	staticHandler := handlers.Chain(
		handlers.PanicRecover(logger),
		handlers.StaticHeaders(),
	)(staticMux)

	// Mux for all dynamic routes with full security headers including Cache-Control: no-store
	dynamicMux := http.NewServeMux()
	dynamicMux.Handle("POST /api/keys", handlers.Chain(
		handlers.MaxBodySize(64*1024),
		handlers.RateLimit(createLimiter, trustXFF),
	)(http.HandlerFunc(h.CreateKey)))
	dynamicMux.Handle("GET /api/keys/{id}",
		handlers.RateLimit(fetchLimiter, trustXFF)(http.HandlerFunc(h.FetchKey)))
	dynamicMux.HandleFunc("GET /{$}", h.HomePage)
	dynamicMux.HandleFunc("GET /share/{id}", h.SharePage)
	dynamicMux.HandleFunc("GET /", h.NotFoundPage)
	dynamicHandler := handlers.Chain(
		handlers.PanicRecover(logger),
		handlers.SecurityHeaders(),
	)(dynamicMux)

	// Root mux routes /static/ to staticHandler, everything else to dynamicHandler
	rootMux := http.NewServeMux()
	rootMux.Handle("/static/", staticHandler)
	rootMux.Handle("/", dynamicHandler)

	return rootMux, nil
}
