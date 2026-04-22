package main

import (
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/josh-zjx/keybank/handlers"
)

func newAppHandler(assetFS fs.FS, store KeyStore, logger *slog.Logger, createLimiter, fetchLimiter handlers.RateLimiter, autoHideSeconds int) (http.Handler, error) {
	templates, err := handlers.ParseTemplates(assetFS)
	if err != nil {
		return nil, err
	}

	h := handlers.New(&storeAdapter{ks: store}, logger, templates)
	h.SetAutoHideSeconds(autoHideSeconds)

	staticFS, err := fs.Sub(assetFS, "static")
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	mux.Handle("POST /api/keys", handlers.Chain(
		handlers.MaxBodySize(64*1024),
		handlers.RateLimit(createLimiter),
	)(http.HandlerFunc(h.CreateKey)))
	mux.Handle("GET /api/keys/{id}",
		handlers.RateLimit(fetchLimiter)(http.HandlerFunc(h.FetchKey)))
	mux.HandleFunc("GET /", h.HomePage)
	mux.HandleFunc("GET /share/{id}", h.SharePage)

	return handlers.Chain(
		handlers.PanicRecover(logger),
		handlers.SecurityHeaders(),
	)(mux), nil
}
