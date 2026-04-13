package commands_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/zounds/cmd/zounds/commands"
	"github.com/darkliquid/zounds/pkg/core"
)

func TestScanCommandIndexesSamples(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "scan.db")
	mustWriteFile(t, filepath.Join(root, "kicks", "kick.wav"))
	mustWriteFile(t, filepath.Join(root, "drums", "hat.mp3"))

	cmd := commands.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--db", dbPath, "scan", root})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan command: %v", err)
	}

	if !strings.Contains(out.String(), "indexed 2 audio files") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestScanCommandVerboseShowsScanAndDatabaseProgress(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "scan.db")
	mustWriteFile(t, filepath.Join(root, "kicks", "kick.wav"))

	cmd := commands.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--verbose", "--db", dbPath, "scan", root})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan command: %v", err)
	}

	output := out.String()
	for _, want := range []string{
		"verbose: scanning file " + filepath.Join(root, "kicks", "kick.wav"),
		"verbose: setting up database at " + dbPath,
		"verbose: applying migration migrations/0001_initial.sql",
		"indexed 1 audio files into " + dbPath,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output %q", want, output)
		}
	}
}

func TestExportCommandOutputsJSON(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "export.db")
	mustWriteFile(t, filepath.Join(root, "fx", "zap.ogg"))

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan command: %v", err)
	}

	export := commands.NewRootCommand()
	var out bytes.Buffer
	export.SetOut(&out)
	export.SetErr(&out)
	export.SetArgs([]string{"--db", dbPath, "export", "--format", "json"})
	if err := export.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute export command: %v", err)
	}

	var samples []core.Sample
	if err := json.Unmarshal(out.Bytes(), &samples); err != nil {
		t.Fatalf("decode export JSON: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(samples))
	}
	if samples[0].Format != core.FormatOGG {
		t.Fatalf("unexpected sample format: %s", samples[0].Format)
	}
}

func mustWriteFile(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}

	if err := os.WriteFile(path, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
