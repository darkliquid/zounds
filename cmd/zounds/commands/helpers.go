package commands

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/darkliquid/zounds/pkg/db"
)

func openRepository(ctx context.Context, cfg *Config) (*db.Repository, io.Closer, error) {
	database, err := db.Open(ctx, db.Options{Path: cfg.DatabasePath})
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
