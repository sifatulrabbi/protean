// Package config loads process configuration from the environment,
// optionally seeded by a .env file at the repository root.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const (
	DefaultAPIPort = 8788
	DefaultDataDir = "./tmp/app-data"

	// DefaultEnvFile is relative to the backend module directory.
	DefaultEnvFile = "../.env"

	// DefaultSQLiteDBFile is appended to DataDir when PROTEAN_SQLITE_DB_PATH is unset.
	DefaultSQLiteDBFile = "protean.db"

	// DefaultEntitlementsWatchInterval is how often the entitlements watcher
	// re-measures disk usage.
	DefaultEntitlementsWatchInterval = 30 * time.Second

	// DefaultHostDiskWatermarkPct is the host disk usage percentage at which
	// the global out-of-space switch turns on.
	DefaultHostDiskWatermarkPct = 80
)

type Config struct {
	APIPort          int
	DataDir          string
	SQLiteDBPath     string
	OpenRouterAPIKey string
	OpenAIAPIKey     string
	TavilyAPIKey     string

	EntitlementsWatchInterval time.Duration
	HostDiskWatermarkPct      int
}

func (c Config) Addr() string { return fmt.Sprintf(":%d", c.APIPort) }

// Load reads DefaultEnvFile (if present) and then the environment.
func Load() (Config, error) { return LoadFrom(DefaultEnvFile) }

// LoadFrom reads envFile (missing file is not an error) and then the environment.
// Values already present in the environment win over the file.
func LoadFrom(envFile string) (Config, error) {
	if envFile != "" {
		if err := loadEnvFile(envFile); err != nil {
			return Config{}, fmt.Errorf("load env file %q: %w", envFile, err)
		}
	}

	cfg := Config{
		APIPort:                   DefaultAPIPort,
		DataDir:                   DefaultDataDir,
		OpenRouterAPIKey:          os.Getenv("OPENROUTER_API_KEY"),
		OpenAIAPIKey:              os.Getenv("OPENAI_API_KEY"),
		TavilyAPIKey:              os.Getenv("TAVILY_API_KEY"),
		EntitlementsWatchInterval: DefaultEntitlementsWatchInterval,
		HostDiskWatermarkPct:      DefaultHostDiskWatermarkPct,
	}

	if v := os.Getenv("PROTEAN_API_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("PROTEAN_API_PORT %q: %w", v, err)
		}
		if port < 1 || port > 65535 {
			return Config{}, fmt.Errorf("PROTEAN_API_PORT %d out of range", port)
		}
		cfg.APIPort = port
	}
	if v := os.Getenv("PROTEAN_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	cfg.DataDir = filepath.Clean(cfg.DataDir)

	if v := os.Getenv("PROTEAN_ENTITLEMENTS_WATCH_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("PROTEAN_ENTITLEMENTS_WATCH_INTERVAL %q: %w", v, err)
		}
		if d <= 0 {
			return Config{}, fmt.Errorf("PROTEAN_ENTITLEMENTS_WATCH_INTERVAL %s must be positive", d)
		}
		cfg.EntitlementsWatchInterval = d
	}
	if v := os.Getenv("PROTEAN_HOST_DISK_WATERMARK_PCT"); v != "" {
		pct, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("PROTEAN_HOST_DISK_WATERMARK_PCT %q: %w", v, err)
		}
		if pct < 1 || pct > 100 {
			return Config{}, fmt.Errorf("PROTEAN_HOST_DISK_WATERMARK_PCT %d out of range", pct)
		}
		cfg.HostDiskWatermarkPct = pct
	}

	cfg.SQLiteDBPath = filepath.Join(cfg.DataDir, DefaultSQLiteDBFile)
	if v := os.Getenv("PROTEAN_SQLITE_DB_PATH"); v != "" {
		cfg.SQLiteDBPath = filepath.Clean(v)
	}

	return cfg, nil
}
