package commands

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/darkliquid/zounds/pkg/db"
)

func openRepository(ctx context.Context, cfg *Config, logger ...*log.Logger) (*db.Repository, io.Closer, error) {
	var configuredLogger *log.Logger
	if len(logger) > 0 {
		configuredLogger = logger[0]
	}

	database, err := db.Open(ctx, db.Options{Path: cfg.DatabasePath, Logger: configuredLogger})
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}

	return db.NewRepository(database), database, nil
}

func outputWriter(defaultWriter io.Writer, path string) (io.Writer, io.Closer, error) {
	if path == "" || path == "-" {
		return defaultWriter, noopCloser{}, nil
	}

	file, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("create output file: %w", err)
	}

	return file, file, nil
}

type noopCloser struct{}

func (noopCloser) Close() error { return nil }

func newVerboseLogger(cfg *Config, out io.Writer) *log.Logger {
	if !cfg.Verbose {
		return nil
	}

	return log.New(out, "verbose: ", 0)
}

func verbosef(logger *log.Logger, format string, args ...any) {
	if logger == nil {
		return
	}
	logger.Printf(format, args...)
}
