package db_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/db"
)

func TestOpenAppliesMigrations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "zounds.db")})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	expectedTables := []string{
		"samples",
		"tags",
		"sample_tags",
		"feature_vectors",
		"clusters",
		"cluster_members",
		"perceptual_hashes",
	}

	for _, table := range expectedTables {
		var name string
		err := database.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Fatalf("table %s missing: %v", table, err)
		}
	}
}

func TestRepositoryUpsertAndListSamples(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "zounds.db")})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	repo := db.NewRepository(database)
	modifiedAt := time.Date(2026, 4, 11, 16, 0, 0, 0, time.UTC)
	scannedAt := modifiedAt.Add(2 * time.Minute)

	id, err := repo.UpsertSample(ctx, core.Sample{
		SourceRoot:   "/library",
		Path:         "/library/kicks/deep.wav",
		RelativePath: "kicks/deep.wav",
		FileName:     "deep.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    2048,
		ModifiedAt:   modifiedAt,
		ScannedAt:    scannedAt,
	})
	if err != nil {
		t.Fatalf("upsert sample: %v", err)
	}
	if id == 0 {
		t.Fatalf("expected non-zero sample id")
	}

	samples, err := repo.ListSamples(ctx)
	if err != nil {
		t.Fatalf("list samples: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(samples))
	}

	got := samples[0]
	if got.Path != "/library/kicks/deep.wav" {
		t.Fatalf("unexpected sample path: %s", got.Path)
	}
	if got.Format != core.FormatWAV {
		t.Fatalf("unexpected sample format: %s", got.Format)
	}
}

func TestFindSampleByPathReturnsNoRows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "zounds.db")})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	repo := db.NewRepository(database)
	_, err = repo.FindSampleByPath(ctx, "/missing.wav")
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}
