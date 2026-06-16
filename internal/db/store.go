// Package db opens the SQLite database, applies goose migrations, and exposes
// the sqlc-generated type-safe query API.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jvmeir/familyplanner/internal/db/dbgen"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // pure-Go SQLite driver (no CGO)
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Store bundles the generated queries with the underlying *sql.DB (needed by
// the scs session store and for graceful shutdown).
type Store struct {
	*dbgen.Queries
	DB *sql.DB
}

// Open creates the data directory if needed, opens the SQLite file with sane
// pragmas, runs migrations, and returns a ready Store.
func Open(ctx context.Context, path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create data dir: %w", err)
		}
	}

	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// A single connection keeps a small self-hosted app free of "database is
	// locked" races; load is trivially low.
	sqlDB.SetMaxOpenConns(1)

	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if err := migrate(sqlDB); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{Queries: dbgen.New(sqlDB), DB: sqlDB}, nil
}

func migrate(sqlDB *sql.DB) error {
	goose.SetBaseFS(migrationFS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}
	return goose.Up(sqlDB, "migrations")
}
