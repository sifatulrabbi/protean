package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/sifatulrabbi/protean/backend/internal/adapters/clock"
	"github.com/sifatulrabbi/protean/backend/internal/config"
	"github.com/sifatulrabbi/protean/backend/internal/httpserver"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := httpserver.New(httpserver.Deps{
		Addr:   cfg.Addr(),
		Logger: logger,
		Clock:  clock.NewSystem(),
	})

	logger.Info("starting protean-api", "addr", cfg.Addr(), "data_dir", cfg.DataDir)
	return srv.Run(ctx)
}
