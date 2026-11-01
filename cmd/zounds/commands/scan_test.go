package commands_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/zounds/pkg/db"
)

func TestScanCommandPruneRemovesMissingFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "prune.db")

	keepPath := filepath.Join(root, "keep.wav")
	stalePath := filepath.Join(root, "stale.wav")
	writeSineWAVFixture(t, keepPath, 220, 0.2)
	writeSineWAVFixture(t, stalePath, 440, 0.2)

	runCommand(t, dbPath, "scan", root)

	if err := os.Remove(stalePath); err != nil {
		t.Fatalf("remove stale file: %v", err)
	}

	out := runCommand(t, dbPath, "scan", root, "--prune")
	if !strings.Contains(out, "pruned 1 missing samples") {
		t.Fatalf("expected prune count in output, got %q", out)
	}

	ctx := context.Background()
	database, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	repo := db.NewRepository(database)
	samples, err := repo.ListSamples(ctx)
	if err != nil {
		t.Fatalf("list samples: %v", err)
	}
	if len(samples) != 1 || samples[0].Path != keepPath {
		t.Fatalf("expected only %s to remain, got %+v", keepPath, samples)
	}
}

func TestScanCommandPruneDryRunKeepsRows(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "prune-dry.db")

	stalePath := filepath.Join(root, "stale.wav")
	writeSineWAVFixture(t, stalePath, 440, 0.2)
	runCommand(t, dbPath, "scan", root)

	if err := os.Remove(stalePath); err != nil {
		t.Fatalf("remove stale file: %v", err)
	}

	out := runCommand(t, dbPath, "--dry-run", "scan", root, "--prune")
	if !strings.Contains(out, "would prune missing sample") {
		t.Fatalf("expected dry-run prune message, got %q", out)
	}

	ctx := context.Background()
	database, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	samples, err := db.NewRepository(database).ListSamples(ctx)
	if err != nil {
		t.Fatalf("list samples: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("expected 1 stale sample to remain after dry run, got %+v", samples)
	}
}

func TestAnalyzeCommandSkipsUnchangedAndForceReanalyzes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "incremental.db")
	writeSineWAVFixture(t, filepath.Join(root, "tone.wav"), 440, 0.2)

	runCommand(t, dbPath, "scan", root)
	first := runCommand(t, dbPath, "analyze", "--all")
	if !strings.Contains(first, "analyzed 1 samples") {
		t.Fatalf("expected first analyze to process the sample, got %q", first)
	}

	second := runCommand(t, dbPath, "analyze", "--all")
	if !strings.Contains(second, "analyzed 0 samples") || !strings.Contains(second, "1 unchanged") {
		t.Fatalf("expected incremental skip, got %q", second)
	}

	third := runCommand(t, dbPath, "analyze", "--all", "--force")
	if !strings.Contains(third, "analyzed 1 samples") {
		t.Fatalf("expected --force to re-analyze, got %q", third)
	}
}
