package db_test

import (
	"bytes"
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"strings"
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

func TestDefaultPathUsesXDGDataHome(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/tmp/zounds-data")

	if got := db.DefaultPath(); got != filepath.Join("/tmp/zounds-data", "zounds", "zounds.db") {
		t.Fatalf("unexpected default path: %s", got)
	}
}

func TestDefaultPathFallsBackToLocalShare(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "/tmp/zounds-home")

	if got := db.DefaultPath(); got != filepath.Join("/tmp/zounds-home", ".local", "share", "zounds", "zounds.db") {
		t.Fatalf("unexpected fallback path: %s", got)
	}
}

func TestOpenLogsSetupStepsWhenLoggerConfigured(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nested", "zounds.db")
	var out bytes.Buffer

	database, err := db.Open(ctx, db.Options{
		Path:   path,
		Logger: log.New(&out, "", 0),
	})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	output := out.String()
	for _, want := range []string{
		"setting up database at " + path,
		"ensuring database directory " + filepath.Dir(path),
		"opening sqlite connection",
		"pinging sqlite database",
		"applying migration migrations/0001_initial.sql",
		"database ready",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in log output %q", want, output)
		}
	}
}

func TestOpenUsesDefaultPathWhenUnset(t *testing.T) {
	ctx := context.Background()
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)

	database, err := db.Open(ctx, db.Options{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	if _, err := os.Stat(filepath.Join(dataHome, "zounds", "zounds.db")); err != nil {
		t.Fatalf("expected default database file to exist: %v", err)
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
