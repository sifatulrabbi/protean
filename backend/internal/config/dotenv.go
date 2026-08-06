package config

import (
	"bufio"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"
)

// loadEnvFile sets KEY=VALUE pairs from path into the process environment
// without overriding variables that are already set. A missing file is a no-op.
func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()
	return applyEnv(f)
}

func applyEnv(r io.Reader) error {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		key, value, ok := parseLine(sc.Text())
		if !ok {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return sc.Err()
}

func parseLine(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimPrefix(line, "export ")

	key, value, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", false
	}
	return key, unquote(strings.TrimSpace(value)), true
}

func unquote(v string) string {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1]
		}
	}
	// Unquoted values may carry a trailing inline comment.
	if i := strings.Index(v, " #"); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	return v
}
