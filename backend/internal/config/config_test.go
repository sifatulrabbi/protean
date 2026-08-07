package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseLine(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		wantKey       string
		wantValue     string
		wantRecognize bool
	}{
		{"simple", "FOO=bar", "FOO", "bar", true},
		{"spaces", "  FOO = bar  ", "FOO", "bar", true},
		{"export prefix", "export FOO=bar", "FOO", "bar", true},
		{"double quoted", `FOO="bar baz"`, "FOO", "bar baz", true},
		{"single quoted", "FOO='bar baz'", "FOO", "bar baz", true},
		{"empty value", "FOO=", "FOO", "", true},
		{"inline comment", "FOO=bar # note", "FOO", "bar", true},
		{"quoted keeps hash", `FOO="bar # note"`, "FOO", "bar # note", true},
		{"comment", "# FOO=bar", "", "", false},
		{"blank", "   ", "", "", false},
		{"no equals", "FOOBAR", "", "", false},
		{"no key", "=bar", "", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key, value, ok := parseLine(tc.line)
			if ok != tc.wantRecognize || key != tc.wantKey || value != tc.wantValue {
				t.Fatalf("parseLine(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tc.line, key, value, ok, tc.wantKey, tc.wantValue, tc.wantRecognize)
			}
		})
	}
}

func TestApplyEnvDoesNotOverride(t *testing.T) {
	t.Setenv("PROTEAN_TEST_PRESET", "from-env")
	t.Setenv("PROTEAN_TEST_UNSET", "")
	os.Unsetenv("PROTEAN_TEST_UNSET")

	in := "PROTEAN_TEST_PRESET=from-file\nPROTEAN_TEST_UNSET=from-file\n"
	if err := applyEnv(strings.NewReader(in)); err != nil {
		t.Fatalf("applyEnv: %v", err)
	}

	if got := os.Getenv("PROTEAN_TEST_PRESET"); got != "from-env" {
		t.Errorf("preset var = %q, want %q", got, "from-env")
	}
	if got := os.Getenv("PROTEAN_TEST_UNSET"); got != "from-file" {
		t.Errorf("unset var = %q, want %q", got, "from-file")
	}
}

func TestLoadFrom(t *testing.T) {
	clearEnv := func(t *testing.T) {
		t.Helper()
		for _, k := range []string{
			"PROTEAN_API_PORT", "PROTEAN_DATA_DIR", "PROTEAN_SQLITE_DB_PATH",
			"PROTEAN_ENTITLEMENTS_WATCH_INTERVAL", "PROTEAN_HOST_DISK_WATERMARK_PCT",
			"PROTEAN_LLM_RESERVE_TOKENS",
			"OPENROUTER_API_KEY", "OPENAI_API_KEY", "TAVILY_API_KEY",
		} {
			t.Setenv(k, "")
			os.Unsetenv(k)
		}
	}

	t.Run("defaults with no env file", func(t *testing.T) {
		clearEnv(t)
		cfg, err := LoadFrom(filepath.Join(t.TempDir(), "missing.env"))
		if err != nil {
			t.Fatalf("LoadFrom: %v", err)
		}
		if cfg.APIPort != DefaultAPIPort {
			t.Errorf("APIPort = %d, want %d", cfg.APIPort, DefaultAPIPort)
		}
		if cfg.DataDir != filepath.Clean(DefaultDataDir) {
			t.Errorf("DataDir = %q, want %q", cfg.DataDir, filepath.Clean(DefaultDataDir))
		}
		if cfg.Addr() != ":8788" {
			t.Errorf("Addr() = %q, want %q", cfg.Addr(), ":8788")
		}
		wantDB := filepath.Join(filepath.Clean(DefaultDataDir), DefaultSQLiteDBFile)
		if cfg.SQLiteDBPath != wantDB {
			t.Errorf("SQLiteDBPath = %q, want %q", cfg.SQLiteDBPath, wantDB)
		}
		if cfg.EntitlementsWatchInterval != DefaultEntitlementsWatchInterval {
			t.Errorf("EntitlementsWatchInterval = %s, want %s", cfg.EntitlementsWatchInterval, DefaultEntitlementsWatchInterval)
		}
		if cfg.HostDiskWatermarkPct != DefaultHostDiskWatermarkPct {
			t.Errorf("HostDiskWatermarkPct = %d, want %d", cfg.HostDiskWatermarkPct, DefaultHostDiskWatermarkPct)
		}
		if cfg.LLMReserveTokens != DefaultLLMReserveTokens {
			t.Errorf("LLMReserveTokens = %d, want %d", cfg.LLMReserveTokens, DefaultLLMReserveTokens)
		}
	})

	t.Run("entitlements overrides", func(t *testing.T) {
		clearEnv(t)
		path := writeEnv(t, "PROTEAN_DATA_DIR=/var/protean\nPROTEAN_ENTITLEMENTS_WATCH_INTERVAL=5s\nPROTEAN_HOST_DISK_WATERMARK_PCT=90\nPROTEAN_LLM_RESERVE_TOKENS=12345\n")
		cfg, err := LoadFrom(path)
		if err != nil {
			t.Fatalf("LoadFrom: %v", err)
		}
		if cfg.EntitlementsWatchInterval != 5*time.Second {
			t.Errorf("EntitlementsWatchInterval = %s, want 5s", cfg.EntitlementsWatchInterval)
		}
		if cfg.HostDiskWatermarkPct != 90 {
			t.Errorf("HostDiskWatermarkPct = %d, want 90", cfg.HostDiskWatermarkPct)
		}
		if cfg.LLMReserveTokens != 12345 {
			t.Errorf("LLMReserveTokens = %d, want 12345", cfg.LLMReserveTokens)
		}
		if want := filepath.Join("/var/protean", DefaultSQLiteDBFile); cfg.SQLiteDBPath != want {
			t.Errorf("SQLiteDBPath = %q, want %q", cfg.SQLiteDBPath, want)
		}
	})

	t.Run("explicit sqlite path wins over data dir", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("PROTEAN_SQLITE_DB_PATH", "/srv/db/protean.db")
		cfg, err := LoadFrom("")
		if err != nil {
			t.Fatalf("LoadFrom: %v", err)
		}
		if cfg.SQLiteDBPath != "/srv/db/protean.db" {
			t.Errorf("SQLiteDBPath = %q, want /srv/db/protean.db", cfg.SQLiteDBPath)
		}
	})

	t.Run("invalid entitlements values", func(t *testing.T) {
		for _, tc := range []struct{ key, value string }{
			{"PROTEAN_ENTITLEMENTS_WATCH_INTERVAL", "soon"},
			{"PROTEAN_ENTITLEMENTS_WATCH_INTERVAL", "-1s"},
			{"PROTEAN_HOST_DISK_WATERMARK_PCT", "many"},
			{"PROTEAN_HOST_DISK_WATERMARK_PCT", "0"},
			{"PROTEAN_HOST_DISK_WATERMARK_PCT", "101"},
			{"PROTEAN_LLM_RESERVE_TOKENS", "many"},
			{"PROTEAN_LLM_RESERVE_TOKENS", "0"},
			{"PROTEAN_LLM_RESERVE_TOKENS", "-1"},
		} {
			t.Run(tc.key+"="+tc.value, func(t *testing.T) {
				clearEnv(t)
				t.Setenv(tc.key, tc.value)
				if _, err := LoadFrom(""); err == nil {
					t.Fatalf("want error for %s=%s", tc.key, tc.value)
				}
			})
		}
	})

	t.Run("env file fills unset values", func(t *testing.T) {
		clearEnv(t)
		path := writeEnv(t, "# comment\n\nPROTEAN_API_PORT=9001\nPROTEAN_DATA_DIR=/var/protean/\nTAVILY_API_KEY=tvly-1\n")
		cfg, err := LoadFrom(path)
		if err != nil {
			t.Fatalf("LoadFrom: %v", err)
		}
		if cfg.APIPort != 9001 {
			t.Errorf("APIPort = %d, want 9001", cfg.APIPort)
		}
		if cfg.DataDir != "/var/protean" {
			t.Errorf("DataDir = %q, want /var/protean", cfg.DataDir)
		}
		if cfg.TavilyAPIKey != "tvly-1" {
			t.Errorf("TavilyAPIKey = %q, want tvly-1", cfg.TavilyAPIKey)
		}
	})

	t.Run("environment wins over env file", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("PROTEAN_API_PORT", "7000")
		path := writeEnv(t, "PROTEAN_API_PORT=9001\n")
		cfg, err := LoadFrom(path)
		if err != nil {
			t.Fatalf("LoadFrom: %v", err)
		}
		if cfg.APIPort != 7000 {
			t.Errorf("APIPort = %d, want 7000", cfg.APIPort)
		}
	})

	t.Run("invalid port", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("PROTEAN_API_PORT", "not-a-port")
		if _, err := LoadFrom(""); err == nil {
			t.Fatal("want error for invalid port")
		}
	})
}

func writeEnv(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	return path
}
