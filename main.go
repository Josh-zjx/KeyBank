package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/josh-zjx/keybank/handlers"
)

func main() {
	cfg := loadConfig()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	store := newRedisKeyStore(cfg.RedisAddr)
	h := handlers.New(store, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/keys", h.CreateKey)
	mux.HandleFunc("GET /api/keys/{id}", h.FetchKey)
	mux.HandleFunc("GET /", h.HomePage)
	mux.HandleFunc("GET /share/{id}", h.SharePage)

	addr := ":" + cfg.Port
	logger.Info("server starting", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		panic(err)
	}
}
