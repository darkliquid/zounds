package commands_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/zounds/cmd/zounds/commands"
)

func TestDedupCommandReportsExactDuplicates(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "dedup.db")
	one := filepath.Join(root, "one.wav")
	two := filepath.Join(root, "two.wav")
	writeWAVFixture(t, one)
	writeWAVFixture(t, two)

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	cmd := commands.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--db", dbPath, "--dry-run", "dedup", "--exact"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute dedup exact: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, one) || !strings.Contains(output, two) {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestDedupCommandVerboseShowsPerFileProgress(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "dedup-verbose.db")
	one := filepath.Join(root, "one.wav")
	two := filepath.Join(root, "two.wav")
	writeWAVFixture(t, one)
	writeWAVFixture(t, two)

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	cmd := commands.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--verbose", "--db", dbPath, "--dry-run", "dedup", "--exact"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute dedup exact: %v", err)
	}

	output := out.String()
	for _, want := range []string{
		"verbose: hashing file " + one,
		"verbose: hashing file " + two,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output %q", want, output)
		}
	}
}

func TestDedupCommandDeletesExactDuplicateFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "dedup-delete.db")
	one := filepath.Join(root, "one.wav")
	two := filepath.Join(root, "two.wav")
	writeWAVFixture(t, one)
	writeWAVFixture(t, two)

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	cmd := commands.NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "dedup", "--exact", "--delete"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute dedup delete: %v", err)
	}

	repo := openRepoForTest(t, dbPath)
	samples, err := repo.ListSamples(context.Background())
	if err != nil {
		t.Fatalf("list samples: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample after delete, got %d", len(samples))
	}

	existing := 0
	for _, path := range []string{one, two} {
		if _, err := os.Stat(path); err == nil {
			existing++
		}
	}
	if existing != 1 {
		t.Fatalf("expected exactly one file to remain, got %d", existing)
	}
}
