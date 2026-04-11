package aiff_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	zaiff "github.com/darkliquid/zounds/pkg/audio/aiff"
	"github.com/darkliquid/zounds/pkg/core"
)

func TestAIFFCodecRoundTrip(t *testing.T) {
	t.Parallel()

	codec := zaiff.New()
	buffer := zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   2,
		BitDepth:   16,
		Data:       []float64{-1.0, -0.25, 0.0, 0.25, 0.5, 0.75},
	}

	filePath := filepath.Join(t.TempDir(), "roundtrip.aiff")
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("create temp aiff file: %v", err)
	}

	if err := codec.Encode(context.Background(), file, buffer); err != nil {
		t.Fatalf("encode aiff: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close temp aiff file: %v", err)
	}

	opened, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("open temp aiff file: %v", err)
	}
	defer opened.Close()

	decoded, err := codec.Decode(context.Background(), opened)
	if err != nil {
		t.Fatalf("decode aiff: %v", err)
	}

	if decoded.Info.Format != core.FormatAIFF {
		t.Fatalf("unexpected format: %s", decoded.Info.Format)
	}
	if decoded.Info.SampleRate != buffer.SampleRate {
		t.Fatalf("unexpected sample rate: %d", decoded.Info.SampleRate)
	}
	if decoded.Info.Channels != buffer.Channels {
		t.Fatalf("unexpected channels: %d", decoded.Info.Channels)
	}
}
