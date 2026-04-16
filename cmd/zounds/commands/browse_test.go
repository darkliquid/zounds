package commands_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/zounds/cmd/zounds/commands"
)

func TestBrowseCommandListsFilteredSamplesWithTags(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "browse.db")
	matchPath := filepath.Join(root, "Synth", "Dark", "glitch.wav")
	otherPath := filepath.Join(root, "Drums", "Clean", "kick.wav")
	writeWAVFixture(t, matchPath)
	writeWAVFixture(t, otherPath)

	scanAndAutoTagForTest(t, dbPath, root)
	addManualTagsForTest(t, dbPath, matchPath, "dark", "glitch")

	cmd := commands.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--db", dbPath, "browse", "glitch", "--tags"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute browse: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, matchPath) {
		t.Fatalf("expected match path in output, got %q", output)
	}
	if strings.Contains(output, otherPath) {
		t.Fatalf("expected filtered output, got %q", output)
	}
	if !strings.Contains(output, "dark") || !strings.Contains(output, "glitch") {
		t.Fatalf("expected tags in output, got %q", output)
	}
}

func TestBrowseCommandDryRunPreviewResolvesFirstMatch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "browse-preview.db")
	samplePath := filepath.Join(root, "Pads", "Epic", "swell.wav")
	writeWAVFixture(t, samplePath)

	scanAndAutoTagForTest(t, dbPath, root)
	addManualTagsForTest(t, dbPath, samplePath, "epic")

	cmd := commands.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--db", dbPath, "--dry-run", "browse", "--tag", "epic", "--play"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute browse preview dry-run: %v", err)
	}

	if !strings.Contains(out.String(), samplePath) {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func scanAndAutoTagForTest(t *testing.T, dbPath string, roots ...string) {
	t.Helper()

	scan := commands.NewRootCommand()
	args := []string{"--db", dbPath, "scan"}
	args = append(args, roots...)
	scan.SetArgs(args)
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	tag := commands.NewRootCommand()
	tag.SetArgs([]string{"--db", dbPath, "tag", "--auto"})
	if err := tag.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute tag: %v", err)
	}
}

func addManualTagsForTest(t *testing.T, dbPath, samplePath string, tags ...string) {
	t.Helper()

	args := []string{"--db", dbPath, "tag", "--path", samplePath}
	for _, tag := range tags {
		args = append(args, "--add", tag)
	}

	cmd := commands.NewRootCommand()
	cmd.SetArgs(args)
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute manual tag add: %v", err)
	}
}
