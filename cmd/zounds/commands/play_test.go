package commands_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/zounds/cmd/zounds/commands"
)

func TestPlayCommandDryRunResolvesDirectFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "tone.wav")
	writeWAVFixture(t, path)

	cmd := commands.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--dry-run", "play", path})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute play dry-run: %v", err)
	}

	if !strings.Contains(out.String(), path) {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestPlayCommandDryRunResolvesTagQuery(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "play.db")
	samplePath := filepath.Join(root, "Drums", "Punch", "kick.wav")
	writeWAVFixture(t, samplePath)

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	tag := commands.NewRootCommand()
	tag.SetArgs([]string{"--db", dbPath, "tag", "--auto"})
	if err := tag.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute tag: %v", err)
	}

	play := commands.NewRootCommand()
	var out bytes.Buffer
	play.SetOut(&out)
	play.SetErr(&out)
	play.SetArgs([]string{"--db", dbPath, "--dry-run", "play", "punch"})
	if err := play.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute play tag dry-run: %v", err)
	}
	if !strings.Contains(out.String(), samplePath) {
		t.Fatalf("unexpected output: %q", out.String())
	}
}
