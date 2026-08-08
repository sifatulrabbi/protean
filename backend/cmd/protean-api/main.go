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
	"github.com/sifatulrabbi/protean/backend/internal/adapters/llm/openaicompat"
	"github.com/sifatulrabbi/protean/backend/internal/adapters/sandbox/dockerbox"
	"github.com/sifatulrabbi/protean/backend/internal/adapters/storage/fsthreads"
	"github.com/sifatulrabbi/protean/backend/internal/config"
	"github.com/sifatulrabbi/protean/backend/internal/entitlements"
	"github.com/sifatulrabbi/protean/backend/internal/harness"
	"github.com/sifatulrabbi/protean/backend/internal/httpserver"
	"github.com/sifatulrabbi/protean/backend/internal/ports"
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

	// Threads live on disk under the data dir and count against the org's disk
	// quota, so the store gets the same entitlements engine as everything else.
	threadStore := fsthreads.New(fsthreads.Options{
		DataDir:      cfg.DataDir,
		Entitlements: engine,
		Clock:        systemClock,
		Logger:       logger,
	})

	// The agent harness. The tool registry is empty here on purpose: S6
	// registers the default tools into it. A missing provider key is not a
	// boot failure — everything except the agent still works — so the harness
	// is simply left unbuilt and the operator is told why.
	agent, err := buildHarness(cfg, logger, threadStore, engine, systemClock)
	if err != nil {
		return err
	}
	// Nothing serves the harness yet: S11 exposes it over HTTP and SSE.
	_ = agent

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
		"llm_provider", cfg.LLMProvider,
		"llm_model", cfg.LLMModel,
		"harness_enabled", agent != nil,
	)
	return srv.Run(ctx)
}

// buildHarness constructs the agent harness, or returns nil when no provider
// key is configured.
func buildHarness(
	cfg config.Config,
	logger *slog.Logger,
	threads ports.ThreadStore,
	ent ports.Entitlements,
	systemClock ports.Clock,
) (*harness.Harness, error) {
	apiKey := cfg.LLMAPIKey()
	if apiKey == "" {
		logger.Warn("agent harness disabled: no provider key",
			"provider", cfg.LLMProvider, "env", cfg.LLMAPIKeyEnv())
		return nil, nil
	}

	opts := openaicompat.Options{
		BaseURL: cfg.LLMBaseURL,
		APIKey:  apiKey,
		Model:   cfg.LLMModel,
		Logger:  logger,
	}
	// The attribution headers are OpenRouter's; sending them anywhere else
	// would leak the deployment's identity to a provider that never asked.
	if cfg.LLMProvider == config.ProviderOpenRouter {
		opts.SiteURL = cfg.OpenRouterSiteURL
		opts.SiteName = cfg.OpenRouterSiteName
	}
	provider, err := openaicompat.New(opts)
	if err != nil {
		return nil, err
	}

	// S6 registers the default tools here.
	tools := harness.NewRegistry()

	return harness.New(harness.Deps{
		DataDir:      cfg.DataDir,
		Provider:     provider,
		Threads:      threads,
		Entitlements: ent,
		Tools:        tools,
		Clock:        systemClock,
		Model:        cfg.LLMModel,
		MaxTurns:     cfg.HarnessMaxTurns,
		Logger:       logger,
	})
}
