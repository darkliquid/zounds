package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Options struct {
	Path   string
	Logger *log.Logger
}

func DefaultPath() string {
	if dataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); dataHome != "" && filepath.IsAbs(dataHome) {
		return filepath.Join(dataHome, "zounds", "zounds.db")
	}

	homeDir, err := os.UserHomeDir()
	if err == nil && strings.TrimSpace(homeDir) != "" {
		return filepath.Join(homeDir, ".local", "share", "zounds", "zounds.db")
	}

	return "zounds.db"
}

func Open(ctx context.Context, opts Options) (*sql.DB, error) {
	path := strings.TrimSpace(opts.Path)
	if path == "" {
		path = DefaultPath()
	}

	logf(opts.Logger, "setting up database at %s", path)

	if path != ":memory:" {
		dir := filepath.Dir(path)
		if dir != "." {
			logf(opts.Logger, "ensuring database directory %s", dir)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("create database directory: %w", err)
			}
		}
	}

	logf(opts.Logger, "opening sqlite connection")
	database, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	database.SetMaxOpenConns(1)

	logf(opts.Logger, "pinging sqlite database")
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}

	if err := Migrate(ctx, database, opts.Logger); err != nil {
		_ = database.Close()
		return nil, err
	}

	logf(opts.Logger, "database ready")

	return database, nil
}

func Migrate(ctx context.Context, database *sql.DB, logger *log.Logger) error {
	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return fmt.Errorf("list migration files: %w", err)
	}

	sort.Strings(names)

	for _, name := range names {
		logf(logger, "applying migration %s", name)
		content, err := migrationFiles.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		if _, err := database.ExecContext(ctx, string(content)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
	}

	return nil
}

func sqliteDSN(path string) string {
	if path == ":memory:" {
		return "file::memory:?cache=shared&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	}

	return fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", path)
}

func logf(logger *log.Logger, format string, args ...any) {
	if logger != nil {
		logger.Printf(format, args...)
	}
}
