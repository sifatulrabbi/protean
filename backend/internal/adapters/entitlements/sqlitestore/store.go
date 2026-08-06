// Package sqlitestore implements entitlements.TokenUsageStore on SQLite.
package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"github.com/sifatulrabbi/protean/backend/internal/entitlements"
)

// schema is applied on every open; it is idempotent.
const schema = `
CREATE TABLE IF NOT EXISTS token_usage (
	org_id        TEXT    NOT NULL,
	month         TEXT    NOT NULL,
	input_tokens  INTEGER NOT NULL DEFAULT 0,
	output_tokens INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (org_id, month)
);
`

type Store struct {
	db *sql.DB
}

var _ entitlements.TokenUsageStore = (*Store)(nil)

// Open opens (creating if needed) the database at path and applies the schema.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create sqlite dir %q: %w", dir, err)
		}
	}

	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}
	// modernc/sqlite serializes writes badly under concurrency; one writer is
	// enough for the counters and avoids SQLITE_BUSY entirely.
	db.SetMaxOpenConns(1)

	if _, err := db.ExecContext(context.Background(), schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema to %q: %w", path, err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// DB exposes the handle so later slices can share the same database file.
func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) AddUsage(ctx context.Context, orgID, month string, inputTokens, outputTokens int64) error {
	const q = `
INSERT INTO token_usage (org_id, month, input_tokens, output_tokens)
VALUES (?, ?, ?, ?)
ON CONFLICT (org_id, month) DO UPDATE SET
	input_tokens  = input_tokens  + excluded.input_tokens,
	output_tokens = output_tokens + excluded.output_tokens;`

	if _, err := s.db.ExecContext(ctx, q, orgID, month, inputTokens, outputTokens); err != nil {
		return fmt.Errorf("add token usage %s/%s: %w", orgID, month, err)
	}
	return nil
}

func (s *Store) UsageForMonth(ctx context.Context, orgID, month string) (int64, int64, error) {
	const q = `SELECT input_tokens, output_tokens FROM token_usage WHERE org_id = ? AND month = ?;`

	var in, out int64
	err := s.db.QueryRowContext(ctx, q, orgID, month).Scan(&in, &out)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, 0, nil
	case err != nil:
		return 0, 0, fmt.Errorf("read token usage %s/%s: %w", orgID, month, err)
	}
	return in, out, nil
}
