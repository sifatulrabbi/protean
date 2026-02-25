package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port          string
	WorkspaceBase string
	// ServiceTokens maps token → service name
	ServiceTokens map[string]string
}

func Load() (*Config, error) {
	if os.Getenv("GOENV") != "production" {
		if err := godotenv.Load(); err != nil {
			panic(err)
		}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	base := os.Getenv("VFS_WORKSPACE_BASE")
	if base == "" {
		return nil, fmt.Errorf("VFS_WORKSPACE_BASE is required")
	}

	tokensRaw := os.Getenv("VFS_SERVICE_TOKENS")
	if tokensRaw == "" {
		return nil, fmt.Errorf("VFS_SERVICE_TOKENS is required")
	}

	tokens, err := parseTokens(tokensRaw)
	if err != nil {
		return nil, err
	}

	return &Config{
		Port:          port,
		WorkspaceBase: base,
		ServiceTokens: tokens,
	}, nil
}

// parseTokens parses "webapp:token1,agent:token2" into map[token]serviceName.
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
		serviceName := parts[0]
		token := parts[1]
		m[token] = serviceName
	}
	if len(m) == 0 {
		return nil, fmt.Errorf("no valid service tokens found")
	}
	return m, nil
}
