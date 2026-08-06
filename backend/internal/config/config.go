// Package config loads process configuration from the environment,
// optionally seeded by a .env file at the repository root.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

const (
	DefaultAPIPort = 8788
	DefaultDataDir = "./tmp/app-data"

	// DefaultEnvFile is relative to the backend module directory.
	DefaultEnvFile = "../.env"
)

type Config struct {
	APIPort          int
	DataDir          string
	OpenRouterAPIKey string
	OpenAIAPIKey     string
	TavilyAPIKey     string
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
		APIPort:          DefaultAPIPort,
		DataDir:          DefaultDataDir,
		OpenRouterAPIKey: os.Getenv("OPENROUTER_API_KEY"),
		OpenAIAPIKey:     os.Getenv("OPENAI_API_KEY"),
		TavilyAPIKey:     os.Getenv("TAVILY_API_KEY"),
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

	return cfg, nil
}
