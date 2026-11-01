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

func TestRenameCommandDryRunPrintsRenderedTarget(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "rename.db")
	samplePath := filepath.Join(root, "Drums", "impact.wav")
	writeWAVFixture(t, samplePath)

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	tag := commands.NewRootCommand()
	tag.SetArgs([]string{"--db", dbPath, "tag", "--add", "dark", "--path", samplePath})
	if err := tag.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute tag add: %v", err)
	}

	cmd := commands.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--db", dbPath, "--dry-run", "rename", "--path", samplePath, "--template", `renamed/{{slug .Stem}}_{{join .Tags "_" }}`})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute rename dry-run: %v", err)
	}

	if !strings.Contains(out.String(), "renamed/impact_dark.wav") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestRenameCommandRenamesFileAndUpdatesDB(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "rename-apply.db")
	samplePath := filepath.Join(root, "Synth", "tone.wav")
	writeWAVFixture(t, samplePath)

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	cmd := commands.NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "rename", "--path", samplePath, "--template", `moved/{{slug .Stem}}_copy`})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute rename: %v", err)
	}

	newPath := filepath.Join(root, "moved", "tone_copy.wav")
	repo := openRepoForTest(t, dbPath)
	if _, err := repo.FindSampleByPath(context.Background(), newPath); err != nil {
		t.Fatalf("find renamed sample: %v", err)
	}
}

func TestRenameCommandPreservesDottedStemExtension(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "rename-dotted.db")
	samplePath := filepath.Join(root, "Drums", "kick.v2.wav")
	writeWAVFixture(t, samplePath)

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	cmd := commands.NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "rename", "--path", samplePath, "--template", `moved/{{.Stem}}`})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute rename: %v", err)
	}

	wantPath := filepath.Join(root, "moved", "kick.v2.wav")
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("expected renamed file at %s: %v", wantPath, err)
	}
}

func TestRenameCommandAbortsBatchBeforeMovingOnCollision(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "rename-collision.db")
	first := filepath.Join(root, "A", "one.wav")
	second := filepath.Join(root, "B", "two.wav")
	writeWAVFixture(t, first)
	writeWAVFixture(t, second)

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	cmd := commands.NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "rename", "--template", `flat/colliding_name`})
	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("expected collision error for duplicate batch targets")
	}

	for _, orig := range []string{first, second} {
		if _, err := os.Stat(orig); err != nil {
			t.Fatalf("expected original file %s untouched after failed preflight: %v", orig, err)
		}
	}
}

func TestRenameCommandRefusesToOverwriteExistingFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "rename-overwrite.db")
	source := filepath.Join(root, "source.wav")
	existing := filepath.Join(root, "target.wav")
	writeWAVFixture(t, source)
	if err := os.WriteFile(existing, []byte("precious-unindexed-file"), 0o644); err != nil {
		t.Fatalf("write existing target: %v", err)
	}

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	cmd := commands.NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "rename", "--path", source, "--template", `target.wav`})
	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("expected error refusing to overwrite existing target.wav")
	}

	gotBytes, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("read existing target: %v", err)
	}
	if string(gotBytes) != "precious-unindexed-file" {
		t.Fatalf("existing file was overwritten: %q", string(gotBytes))
	}
}
