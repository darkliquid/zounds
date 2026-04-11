package commands_test

import (
	"bytes"
	"context"
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
