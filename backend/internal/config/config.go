// Package config loads process configuration from the environment,
// optionally seeded by a .env file at the repository root.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

	// DefaultLLMReserveTokens is the estimated token hold for one in-flight LLM
	// invocation.
	DefaultLLMReserveTokens int64 = 50_000
	// Sandbox runtime defaults (S2). The concurrency cap follows the VM sizing
	// in the sandboxing design: 4 sandboxes at 0.25 vCPU / 1 GiB each.
	DefaultSandboxImage          = "protean-sandbox:dev"
	DefaultSandboxExecTimeout    = 120 * time.Second
	DefaultSandboxIdleTimeout    = 15 * time.Minute
	DefaultSandboxMaxRunning     = 4
	DefaultSandboxOutputCapBytes = 256 * 1024
	DefaultSandboxReapInterval   = time.Minute

	// MaxSandboxExecTimeout is the hard ceiling on a single sandbox command.
	// Configuring more than this is a configuration error, not a clamp.
	MaxSandboxExecTimeout = 10 * time.Minute

	// LLM provider defaults (S5/D4). OpenRouter is the default provider and
	// OpenAI the secondary; both speak the same API, so they differ only by
	// base URL, key, and model.
	DefaultLLMProvider = ProviderOpenRouter

	// DefaultLLMModel is an OpenRouter model slug, so it only applies to
	// OpenRouter: choosing another provider means naming its model too.
	DefaultLLMModel = "xiaomi/mimo-v2.5-pro"

	// DefaultHarnessMaxTurns bounds one agent run. It is the runaway guard;
	// the token budget is the entitlements engine's business. It must stay in
	// step with harness.DefaultMaxTurns, which applies when the harness is
	// built without this setting.
	DefaultHarnessMaxTurns = 40

	// MaxHarnessMaxTurns is the ceiling on the runaway guard.
	MaxHarnessMaxTurns = 500
)

// Supported LLM provider names for PROTEAN_LLM_PROVIDER.
const (
	ProviderOpenRouter = "openrouter"
	ProviderOpenAI     = "openai"
)

// Base URLs used when PROTEAN_LLM_BASE_URL does not override them. The
// provider mapping lives here rather than in the adapter, because the adapter
// is not "OpenRouter" or "OpenAI" — it is whatever OpenAI-compatible endpoint
// it is pointed at.
const (
	OpenRouterBaseURL = "https://openrouter.ai/api/v1"
	OpenAIBaseURL     = "https://api.openai.com/v1"
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
	LLMReserveTokens          int64

	SandboxImage          string
	SandboxExecTimeout    time.Duration
	SandboxIdleTimeout    time.Duration
	SandboxMaxRunning     int
	SandboxOutputCapBytes int
	SandboxReapInterval   time.Duration

	// LLMProvider selects which key and which base URL the harness uses.
	LLMProvider string
	LLMModel    string
	LLMBaseURL  string

	// OpenRouterSiteURL and OpenRouterSiteName are OpenRouter's optional
	// attribution headers. They are only sent when set.
	OpenRouterSiteURL  string
	OpenRouterSiteName string

	HarnessMaxTurns int
}

func (c Config) Addr() string { return fmt.Sprintf(":%d", c.APIPort) }

// LLMAPIKey is the key for the selected provider. It is empty when the
// provider is not configured, which is not a boot failure: the process serves
// fine without an agent, it just cannot invoke one.
func (c Config) LLMAPIKey() string {
	switch c.LLMProvider {
	case ProviderOpenAI:
		return c.OpenAIAPIKey
	default:
		return c.OpenRouterAPIKey
	}
}

// LLMAPIKeyEnv names the variable LLMAPIKey reads, so an operator can be told
// exactly what to set.
func (c Config) LLMAPIKeyEnv() string {
	if c.LLMProvider == ProviderOpenAI {
		return "OPENAI_API_KEY"
	}
	return "OPENROUTER_API_KEY"
}

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
		LLMReserveTokens:          DefaultLLMReserveTokens,
		SandboxImage:              DefaultSandboxImage,
		SandboxExecTimeout:        DefaultSandboxExecTimeout,
		SandboxIdleTimeout:        DefaultSandboxIdleTimeout,
		SandboxMaxRunning:         DefaultSandboxMaxRunning,
		SandboxOutputCapBytes:     DefaultSandboxOutputCapBytes,
		SandboxReapInterval:       DefaultSandboxReapInterval,
		LLMProvider:               DefaultLLMProvider,
		LLMModel:                  DefaultLLMModel,
		OpenRouterSiteURL:         os.Getenv("OPENROUTER_SITE_URL"),
		OpenRouterSiteName:        os.Getenv("OPENROUTER_SITE_NAME"),
		HarnessMaxTurns:           DefaultHarnessMaxTurns,
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
	if v := os.Getenv("PROTEAN_LLM_RESERVE_TOKENS"); v != "" {
		tokens, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("PROTEAN_LLM_RESERVE_TOKENS %q: %w", v, err)
		}
		if tokens <= 0 {
			return Config{}, fmt.Errorf("PROTEAN_LLM_RESERVE_TOKENS %d must be positive", tokens)
		}
		cfg.LLMReserveTokens = tokens
	}

	if v := os.Getenv("PROTEAN_SANDBOX_IMAGE"); v != "" {
		cfg.SandboxImage = v
	}
	if err := envDuration("PROTEAN_SANDBOX_EXEC_TIMEOUT", &cfg.SandboxExecTimeout); err != nil {
		return Config{}, err
	}
	if cfg.SandboxExecTimeout > MaxSandboxExecTimeout {
		return Config{}, fmt.Errorf("PROTEAN_SANDBOX_EXEC_TIMEOUT %s is above the %s ceiling",
			cfg.SandboxExecTimeout, MaxSandboxExecTimeout)
	}
	if err := envDuration("PROTEAN_SANDBOX_IDLE_TIMEOUT", &cfg.SandboxIdleTimeout); err != nil {
		return Config{}, err
	}
	if err := envDuration("PROTEAN_SANDBOX_REAP_INTERVAL", &cfg.SandboxReapInterval); err != nil {
		return Config{}, err
	}
	if err := envInt("PROTEAN_SANDBOX_MAX_RUNNING", &cfg.SandboxMaxRunning, 1, 64); err != nil {
		return Config{}, err
	}
	if err := envInt("PROTEAN_SANDBOX_OUTPUT_CAP_BYTES", &cfg.SandboxOutputCapBytes, 1024, 64*1024*1024); err != nil {
		return Config{}, err
	}

	if v := os.Getenv("PROTEAN_LLM_PROVIDER"); v != "" {
		switch v {
		case ProviderOpenRouter, ProviderOpenAI:
			cfg.LLMProvider = v
		default:
			return Config{}, fmt.Errorf("PROTEAN_LLM_PROVIDER %q must be %q or %q", v, ProviderOpenRouter, ProviderOpenAI)
		}
	}
	if v := os.Getenv("PROTEAN_LLM_MODEL"); v != "" {
		cfg.LLMModel = v
	} else if cfg.LLMProvider != ProviderOpenRouter {
		// The default model is an OpenRouter slug. Letting it stand for
		// another provider would boot cleanly and then fail on the first
		// invocation with a model-not-found nobody expected.
		return Config{}, fmt.Errorf("PROTEAN_LLM_MODEL must be set when PROTEAN_LLM_PROVIDER is %q", cfg.LLMProvider)
	}
	cfg.LLMBaseURL = defaultLLMBaseURL(cfg.LLMProvider)
	if v := os.Getenv("PROTEAN_LLM_BASE_URL"); v != "" {
		cfg.LLMBaseURL = strings.TrimRight(v, "/")
	}
	if err := envInt("PROTEAN_HARNESS_MAX_TURNS", &cfg.HarnessMaxTurns, 1, MaxHarnessMaxTurns); err != nil {
		return Config{}, err
	}

	cfg.SQLiteDBPath = filepath.Join(cfg.DataDir, DefaultSQLiteDBFile)
	if v := os.Getenv("PROTEAN_SQLITE_DB_PATH"); v != "" {
		cfg.SQLiteDBPath = filepath.Clean(v)
	}

	return cfg, nil
}

func defaultLLMBaseURL(provider string) string {
	if provider == ProviderOpenAI {
		return OpenAIBaseURL
	}
	return OpenRouterBaseURL
}

// envDuration overwrites dst with the positive duration in key, leaving dst
// alone when the variable is unset.
func envDuration(key string, dst *time.Duration) error {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fmt.Errorf("%s %q: %w", key, v, err)
	}
	if d <= 0 {
		return fmt.Errorf("%s %s must be positive", key, d)
	}
	*dst = d
	return nil
}

// envInt overwrites dst with the integer in key when it falls inside
// [min, max], leaving dst alone when the variable is unset.
func envInt(key string, dst *int, min, max int) error {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("%s %q: %w", key, v, err)
	}
	if n < min || n > max {
		return fmt.Errorf("%s %d out of range", key, n)
	}
	*dst = n
	return nil
}
