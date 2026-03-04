package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds validated process configuration for the sandbox service.
type Config struct {
	// Port is the TCP port the HTTP server listens on.
	Port string
	// WorkspaceBase is the host directory that contains one subdirectory per session.
	WorkspaceBase string
	// ServiceTokens maps bearer tokens to the logical service name that owns them.
	ServiceTokens map[string]string
	// DefaultImage is the Docker image used when creating a new sandbox.
	DefaultImage string
	// ContainerPrefix namespaces Docker container names created by this service.
	ContainerPrefix string
	// ContainerWorkdir is the path mounted inside the container as the workspace root.
	ContainerWorkdir string
	// ContainerUser is the user identity used for commands executed inside the container.
	ContainerUser string
	// ExecDefaultTimeout is applied when callers omit an exec timeout.
	ExecDefaultTimeout time.Duration
	// ExecMaxTimeout caps caller-provided exec timeouts.
	ExecMaxTimeout time.Duration
	// ExecMaxOutputBytes caps stdout and stderr captured from an exec request.
	ExecMaxOutputBytes int
}

// Load reads environment variables, applies defaults, and validates the result.
func Load() (*Config, error) {
	if env := os.Getenv("ENV"); env != "production" && env != "prod" {
		// Local development relies on a .env file; production is expected to inject env vars directly.
		if err := godotenv.Load(); err != nil {
			log.Fatalln("Failed to load ENVs", err)
		}
	}

	port := getenvDefault("PORT", "8091")
	workspaceBase := strings.TrimSpace(os.Getenv("SANDBOX_WORKSPACE_BASE"))
	if workspaceBase == "" {
		return nil, fmt.Errorf("SANDBOX_WORKSPACE_BASE is required")
	}
	if !filepath.IsAbs(workspaceBase) {
		return nil, fmt.Errorf("SANDBOX_WORKSPACE_BASE must be an absolute path")
	}

	tokensRaw := os.Getenv("SANDBOX_SERVICE_TOKENS")
	if tokensRaw == "" {
		return nil, fmt.Errorf("SANDBOX_SERVICE_TOKENS is required")
	}

	tokens, err := parseTokens(tokensRaw)
	if err != nil {
		return nil, err
	}

	defaultImage := strings.TrimSpace(os.Getenv("SANDBOX_DEFAULT_IMAGE"))
	if defaultImage == "" {
		return nil, fmt.Errorf("SANDBOX_DEFAULT_IMAGE is required")
	}

	execDefaultTimeoutMs, err := parseIntEnv(
		"SANDBOX_EXEC_DEFAULT_TIMEOUT_MS",
		30000,
	)
	if err != nil {
		return nil, err
	}

	execMaxTimeoutMs, err := parseIntEnv("SANDBOX_EXEC_MAX_TIMEOUT_MS", 300000)
	if err != nil {
		return nil, err
	}

	execMaxOutputBytes, err := parseIntEnv(
		"SANDBOX_EXEC_MAX_OUTPUT_BYTES",
		65536,
	)
	if err != nil {
		return nil, err
	}
	if execDefaultTimeoutMs > execMaxTimeoutMs {
		return nil, fmt.Errorf("SANDBOX_EXEC_DEFAULT_TIMEOUT_MS must be less than or equal to SANDBOX_EXEC_MAX_TIMEOUT_MS")
	}

	containerPrefix := strings.TrimSpace(
		getenvDefault("SANDBOX_CONTAINER_PREFIX", "protean-sandbox"),
	)
	if containerPrefix == "" {
		return nil, fmt.Errorf("SANDBOX_CONTAINER_PREFIX is required")
	}

	containerWorkdir := strings.TrimSpace(
		getenvDefault("SANDBOX_CONTAINER_WORKDIR", "/workspace"),
	)
	if containerWorkdir == "" {
		return nil, fmt.Errorf("SANDBOX_CONTAINER_WORKDIR is required")
	}
	if !filepath.IsAbs(containerWorkdir) {
		return nil, fmt.Errorf("SANDBOX_CONTAINER_WORKDIR must be an absolute path")
	}

	containerUser := strings.TrimSpace(
		getenvDefault("SANDBOX_CONTAINER_USER", "1000:1000"),
	)
	if containerUser == "" {
		return nil, fmt.Errorf("SANDBOX_CONTAINER_USER is required")
	}

	return &Config{
		Port:               port,
		WorkspaceBase:      workspaceBase,
		ServiceTokens:      tokens,
		DefaultImage:       defaultImage,
		ContainerPrefix:    containerPrefix,
		ContainerWorkdir:   containerWorkdir,
		ContainerUser:      containerUser,
		ExecDefaultTimeout: time.Duration(execDefaultTimeoutMs) * time.Millisecond,
		ExecMaxTimeout:     time.Duration(execMaxTimeoutMs) * time.Millisecond,
		ExecMaxOutputBytes: execMaxOutputBytes,
	}, nil
}

// getenvDefault returns the environment variable when present, otherwise the fallback.
func getenvDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// parseIntEnv parses a positive integer environment variable with a fallback value.
func parseIntEnv(key string, fallback int) (int, error) {
	value := getenvDefault(key, strconv.Itoa(fallback))
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", key)
	}
	return parsed, nil
}

// parseTokens parses a comma-separated list of serviceName:token pairs.
func parseTokens(raw string) (map[string]string, error) {
	m := make(map[string]string)
	pairs := strings.Split(raw, ",")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid token pair: %q", pair)
		}
		// Tokens must be unique because bearer auth only tells us which token was supplied.
		if _, exists := m[parts[1]]; exists {
			return nil, fmt.Errorf("duplicate service token: %q", parts[1])
		}
		m[parts[1]] = parts[0]
	}

	if len(m) == 0 {
		return nil, fmt.Errorf("no valid service tokens found")
	}

	return m, nil
}
