package commands_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/zounds/cmd/zounds/commands"
)

func TestRootCommandShowsHelpByDefault(t *testing.T) {
	t.Parallel()

	cmd := commands.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute root command: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "zounds scans sample libraries") {
		t.Fatalf("expected help output, got %q", output)
	}
}

func TestRootCommandRegistersGlobalFlags(t *testing.T) {
	t.Parallel()

	cmd := commands.NewRootCommand()

	for _, name := range []string{"db", "verbose", "dry-run", "concurrency"} {
		if flag := cmd.PersistentFlags().Lookup(name); flag == nil {
			t.Fatalf("missing persistent flag %q", name)
		}
	}
}

func TestRootCommandIncludesPlannedSubcommands(t *testing.T) {
	t.Parallel()

	cmd := commands.NewRootCommand()

	expected := []string{"scan", "analyze", "tag", "cluster", "dedup", "convert", "rename", "serve", "export", "info", "play", "browse"}
	seen := make(map[string]struct{}, len(cmd.Commands()))
	for _, subcommand := range cmd.Commands() {
		seen[subcommand.Name()] = struct{}{}
	}

	for _, name := range expected {
		if _, ok := seen[name]; !ok {
			t.Fatalf("missing subcommand %q", name)
		}
	}
}

func TestDefaultConfigUsesXDGDataDir(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/tmp/zounds-data")

	cfg := commands.DefaultConfig()
	want := filepath.Join("/tmp/zounds-data", "zounds", "zounds.db")
	if cfg.DatabasePath != want {
		t.Fatalf("expected database path %q, got %q", want, cfg.DatabasePath)
	}
}
