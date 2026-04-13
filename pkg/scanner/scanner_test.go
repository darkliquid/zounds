package scanner_test

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/scanner"
)

func TestScanFindsSupportedAudioFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	mustWriteFile(t, filepath.Join(root, "kicks", "deep.wav"))
	mustWriteFile(t, filepath.Join(root, "snares", "tight.FLAC"))
	mustWriteFile(t, filepath.Join(root, "notes.txt"))
	mustWriteFile(t, filepath.Join(root, ".hidden", "ghost.wav"))

	s := scanner.New(scanner.Options{Workers: 2})
	samples, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(samples) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(samples))
	}

	if samples[0].Format != core.FormatWAV {
		t.Fatalf("expected first sample to be wav, got %s", samples[0].Format)
	}
	if samples[1].Format != core.FormatFLAC {
		t.Fatalf("expected second sample to be flac, got %s", samples[1].Format)
	}

	if samples[0].RelativePath != "kicks/deep.wav" {
		t.Fatalf("unexpected relative path: %s", samples[0].RelativePath)
	}
	if samples[1].RelativePath != "snares/tight.FLAC" {
		t.Fatalf("unexpected relative path: %s", samples[1].RelativePath)
	}
}

func TestSupportedExtensionsSorted(t *testing.T) {
	t.Parallel()

	extensions := scanner.SupportedExtensions()
	if len(extensions) == 0 {
		t.Fatalf("expected supported extensions")
	}

	for i := 1; i < len(extensions); i++ {
		if extensions[i] < extensions[i-1] {
			t.Fatalf("extensions not sorted: %v", extensions)
		}
	}
}

func TestScanLogsRootsAndFilesWhenLoggerConfigured(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "kicks", "deep.wav"))
	mustWriteFile(t, filepath.Join(root, "notes.txt"))

	var out bytes.Buffer
	s := scanner.New(scanner.Options{
		Workers: 1,
		Logger:  logBuffer(&out),
	})

	if _, err := s.Scan(context.Background(), root); err != nil {
		t.Fatalf("scan: %v", err)
	}

	output := out.String()
	for _, want := range []string{
		"starting scan with 1 root(s) and 1 worker(s)",
		"walking root " + root,
		"scanning file " + filepath.Join(root, "kicks", "deep.wav"),
		"scan complete: discovered 1 audio file(s)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in log output %q", want, output)
		}
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

func logBuffer(out *bytes.Buffer) *log.Logger {
	return log.New(out, "", 0)
}
