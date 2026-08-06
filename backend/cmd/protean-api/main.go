package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/sifatulrabbi/protean/backend/internal/adapters/clock"
	"github.com/sifatulrabbi/protean/backend/internal/adapters/entitlements/diskusage"
	"github.com/sifatulrabbi/protean/backend/internal/adapters/entitlements/sqlitestore"
	"github.com/sifatulrabbi/protean/backend/internal/config"
	"github.com/sifatulrabbi/protean/backend/internal/entitlements"
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

	systemClock := clock.NewSystem()

	tokenStore, err := sqlitestore.Open(cfg.SQLiteDBPath)
	if err != nil {
		return err
	}
	defer tokenStore.Close()

	plan := entitlements.Free()
	engine := entitlements.New(entitlements.Deps{
		Plan:             plan,
		TokenUsage:       tokenStore,
		Disk:             diskusage.New(cfg.DataDir),
		Counts:           entitlements.ZeroCounts{},
		Clock:            systemClock,
		Logger:           logger,
		WatchInterval:    cfg.EntitlementsWatchInterval,
		HostWatermarkPct: cfg.HostDiskWatermarkPct,
	})
	watcherCtx, stopWatcher := context.WithCancel(ctx)
	defer func() {
		stopWatcher()
		engine.Wait()
	}()
	engine.Start(watcherCtx)

	srv := httpserver.New(httpserver.Deps{
		Addr:   cfg.Addr(),
		Logger: logger,
		Clock:  systemClock,
	})

	logger.Info("starting protean-api",
		"addr", cfg.Addr(),
		"data_dir", cfg.DataDir,
		"sqlite_db", cfg.SQLiteDBPath,
		"plan", plan.Name,
	)
	return srv.Run(ctx)
}
