package wav_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	zwav "github.com/darkliquid/zounds/pkg/audio/wav"
	"github.com/darkliquid/zounds/pkg/core"
)

func TestWAVCodecRoundTrip(t *testing.T) {
	t.Parallel()

	codec := zwav.New()
	buffer := zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       []float64{-1.0, -0.5, 0.0, 0.5, 0.999},
	}

	filePath := filepath.Join(t.TempDir(), "roundtrip.wav")
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("create temp wav file: %v", err)
	}

	if err := codec.Encode(context.Background(), file, buffer); err != nil {
		t.Fatalf("encode wav: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close temp wav file: %v", err)
	}

	opened, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("open temp wav file: %v", err)
	}
	defer func() { _ = opened.Close() }()

	decoded, err := codec.Decode(context.Background(), opened)
	if err != nil {
		t.Fatalf("decode wav: %v", err)
	}

	if decoded.Info.Format != core.FormatWAV {
		t.Fatalf("unexpected format: %s", decoded.Info.Format)
	}
	if decoded.Info.SampleRate != buffer.SampleRate {
		t.Fatalf("unexpected sample rate: %d", decoded.Info.SampleRate)
	}
	if decoded.Info.Channels != buffer.Channels {
		t.Fatalf("unexpected channels: %d", decoded.Info.Channels)
	}
	if decoded.Buffer.Frames() != len(buffer.Data) {
		t.Fatalf("unexpected frame count: %d", decoded.Buffer.Frames())
	}
}

func TestRegisterAddsWAVCodecToRegistry(t *testing.T) {
	t.Parallel()

	registry := zaudio.NewRegistry()
	if err := zwav.Register(registry); err != nil {
		t.Fatalf("register wav codec: %v", err)
	}

	if _, ok := registry.Decoder(core.FormatWAV); !ok {
		t.Fatalf("expected wav decoder")
	}
	if _, ok := registry.Encoder(core.FormatWAV); !ok {
		t.Fatalf("expected wav encoder")
	}
}
