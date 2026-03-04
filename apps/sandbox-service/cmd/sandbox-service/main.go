package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/protean/sandbox-service/internal/api"
	"github.com/protean/sandbox-service/internal/config"
	"github.com/protean/sandbox-service/internal/sandbox"
)

// main wires together configuration, sandbox runtime, and HTTP handlers.
func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	runtime, err := sandbox.NewDockerRuntime(
		cfg.ContainerUser,
		cfg.ContainerWorkdir,
	)
	if err != nil {
		log.Fatalf("docker runtime: %v", err)
	}
	service := sandbox.NewService(sandbox.ServiceConfig{
		WorkspaceBase:      cfg.WorkspaceBase,
		ContainerPrefix:    cfg.ContainerPrefix,
		WorkspaceMountPath: cfg.ContainerWorkdir,
		DefaultImage:       cfg.DefaultImage,
		Logger:             logger,
		ExecDefaultTimeout: cfg.ExecDefaultTimeout,
		ExecMaxTimeout:     cfg.ExecMaxTimeout,
		ExecMaxOutputBytes: cfg.ExecMaxOutputBytes,
	}, runtime)

	handler := api.New(api.Config{
		ServiceTokens: cfg.ServiceTokens,
		Service:       service,
		Logger:        logger,
	})

	log.Printf("sandbox-service listening on :%s (workspace=%s)", cfg.Port, cfg.WorkspaceBase)
	if err := http.ListenAndServe(":"+cfg.Port, handler); err != nil {
		log.Fatalf("server: %v", err)
	}
}
