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
	"github.com/sifatulrabbi/protean/backend/internal/adapters/sandbox/dockerbox"
	"github.com/sifatulrabbi/protean/backend/internal/config"
	"github.com/sifatulrabbi/protean/backend/internal/entitlements"
	"github.com/sifatulrabbi/protean/backend/internal/httpserver"
	"github.com/sifatulrabbi/protean/backend/internal/sandbox"
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
	logger.Warn("entitlements: structural caps are not enforced",
		"reason", "metadata counts adapter is not available; using ZeroCounts placeholder")
	engine := entitlements.New(entitlements.Deps{
		Plan:             plan,
		TokenUsage:       tokenStore,
		Disk:             diskusage.New(cfg.DataDir),
		Counts:           entitlements.ZeroCounts{},
		Clock:            systemClock,
		Logger:           logger,
		WatchInterval:    cfg.EntitlementsWatchInterval,
		HostWatermarkPct: cfg.HostDiskWatermarkPct,
		LLMReserveTokens: cfg.LLMReserveTokens,
	})
	watcherCtx, stopWatcher := context.WithCancel(ctx)
	defer func() {
		stopWatcher()
		engine.Wait()
	}()
	engine.Start(watcherCtx)

	// The sandbox is the security boundary, so a boot without a healthy runtime
	// is a failed boot: there is no unsandboxed fallback (D8).
	registry := sandbox.NewRegistry(logger)
	registry.Register(dockerbox.NewFactory(dockerbox.Options{
		Image:          cfg.SandboxImage,
		DataDir:        cfg.DataDir,
		ExecTimeout:    cfg.SandboxExecTimeout,
		IdleTimeout:    cfg.SandboxIdleTimeout,
		ReapInterval:   cfg.SandboxReapInterval,
		MaxRunning:     cfg.SandboxMaxRunning,
		OutputCapBytes: cfg.SandboxOutputCapBytes,
		Logger:         logger,
		Clock:          systemClock,
	}))
	sandboxRuntime, err := registry.Select(ctx)
	if err != nil {
		return err
	}
	defer sandboxRuntime.Close()

	reaperCtx, stopReaper := context.WithCancel(ctx)
	defer func() {
		stopReaper()
		sandboxRuntime.Wait()
	}()
	sandboxRuntime.Start(reaperCtx)

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
		"sandbox_image", cfg.SandboxImage,
		"sandbox_max_running", cfg.SandboxMaxRunning,
	)
	return srv.Run(ctx)
}
