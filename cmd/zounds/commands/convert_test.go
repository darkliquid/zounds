package commands_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/zounds/cmd/zounds/commands"
	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/audio/codecs"
	"github.com/darkliquid/zounds/pkg/core"
)

func TestConvertCommandConvertsAndTransformsAudio(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.wav")
	targetPath := filepath.Join(dir, "converted.aiff")
	writeWAVFixture(t, sourcePath)

	cmd := commands.NewRootCommand()
	cmd.SetArgs([]string{
		"convert",
		sourcePath,
		"--output", targetPath,
		"--format", "aiff",
		"--samplerate", "22050",
		"--channels", "2",
	})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute convert: %v", err)
	}

	registry, err := codecs.NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	decoded, err := zaudio.DecodeFile(context.Background(), registry, targetPath)
	if err != nil {
		t.Fatalf("decode converted file: %v", err)
	}

	if decoded.Info.Format != core.FormatAIFF {
		t.Fatalf("expected AIFF output, got %s", decoded.Info.Format)
	}
	if decoded.Buffer.SampleRate != 22050 {
		t.Fatalf("expected sample rate 22050, got %d", decoded.Buffer.SampleRate)
	}
	if decoded.Buffer.Channels != 2 {
		t.Fatalf("expected 2 channels, got %d", decoded.Buffer.Channels)
	}
}

func TestConvertCommandDryRunPrintsPlanWithoutWritingOutput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.wav")
	targetPath := filepath.Join(dir, "preview.aiff")
	writeWAVFixture(t, sourcePath)

	cmd := commands.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"--dry-run",
		"convert",
		sourcePath,
		"--output", targetPath,
		"--format", "aiff",
		"--normalize", "peak",
		"--target-dbfs", "-6",
	})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute convert dry-run: %v", err)
	}

	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("expected no output file, stat err=%v", err)
	}

	output := out.String()
	if !strings.Contains(output, sourcePath) || !strings.Contains(output, targetPath) || !strings.Contains(output, "normalize=peak@-6.00dbfs") {
		t.Fatalf("unexpected output: %q", output)
	}
}
