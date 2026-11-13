package main

import (
	"log/slog"
	"os"

	"github.com/josh-zjx/keybank/internal/demo"
	"github.com/josh-zjx/keybank/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := service.LoadConfig("14001")
	if err != nil {
		logger.Error("configuration", "err", err)
		os.Exit(1)
	}
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(service.Healthcheck(cfg.Port))
	}
	app, err := demo.NewHandler(cfg.APIBaseURL, cfg.AutoHideSeconds, cfg.MaxTTL, logger)
	if err != nil {
		logger.Error("demo initialization", "err", err)
		os.Exit(1)
	}
	if err := service.Run(app, cfg.Port, logger); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
