package convert_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/audio/codecs"
	"github.com/darkliquid/zounds/pkg/audio/wav"
	"github.com/darkliquid/zounds/pkg/convert"
	"github.com/darkliquid/zounds/pkg/core"
)

func TestTranscodeFileConvertsWAVToAIFF(t *testing.T) {
	t.Parallel()

	registry, err := codecs.NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.wav")
	targetPath := filepath.Join(dir, "target.aiff")
	writePCMFixture(t, sourcePath)

	if err := convert.TranscodeFile(context.Background(), registry, sourcePath, targetPath); err != nil {
		t.Fatalf("transcode file: %v", err)
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("expected non-empty transcoded file")
	}

	decoded, err := zaudio.DecodeFile(context.Background(), registry, targetPath)
	if err != nil {
		t.Fatalf("decode transcoded file: %v", err)
	}
	if decoded.Info.Format != core.FormatAIFF {
		t.Fatalf("expected AIFF format, got %s", decoded.Info.Format)
	}
	if decoded.Buffer.SampleRate != 44100 || decoded.Buffer.Channels != 1 {
		t.Fatalf("unexpected decoded buffer: %+v", decoded.Buffer)
	}
}

func TestSupportedTargetFormatsIncludesWritableFormats(t *testing.T) {
	t.Parallel()

	registry, err := codecs.NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	formats := convert.SupportedTargetFormats(registry)
	seen := make(map[core.AudioFormat]struct{}, len(formats))
	for _, format := range formats {
		seen[format] = struct{}{}
	}

	for _, expected := range []core.AudioFormat{core.FormatWAV, core.FormatAIFF} {
		if _, ok := seen[expected]; !ok {
			t.Fatalf("missing writable format %s in %v", expected, formats)
		}
	}
}

func TestTranscodeFileRejectsUnsupportedTargetFormat(t *testing.T) {
	t.Parallel()

	registry, err := codecs.NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.wav")
	writePCMFixture(t, sourcePath)

	targetPath := filepath.Join(dir, "target.flac")
	if err := convert.TranscodeFile(context.Background(), registry, sourcePath, targetPath); err == nil {
		t.Fatal("expected unsupported target format error")
	}
}

func writePCMFixture(t *testing.T, path string) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	defer file.Close()

	buffer := zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       []float64{-0.25, 0, 0.25, 0.5, 0.25, 0},
	}
	if err := wav.New().Encode(context.Background(), file, buffer); err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
}
