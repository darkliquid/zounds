package aiff_test

import (
	"bytes"
	"context"
	"math"
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
	defer func() { _ = opened.Close() }()

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

func TestAIFFCodec8BitSignedRoundTrip(t *testing.T) {
	t.Parallel()

	codec := zaiff.New()
	want := []float64{-1.0, -0.5, 0.0, 0.5, 127.0 / 128.0}
	buffer := zaudio.PCMBuffer{
		SampleRate: 22050,
		Channels:   1,
		BitDepth:   8,
		Data:       want,
	}

	filePath := filepath.Join(t.TempDir(), "eightbit.aiff")
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("create 8-bit aiff: %v", err)
	}
	if err := codec.Encode(context.Background(), file, buffer); err != nil {
		t.Fatalf("encode 8-bit aiff: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close 8-bit aiff: %v", err)
	}

	raw, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read raw 8-bit aiff: %v", err)
	}
	ssndIdx := bytes.Index(raw, []byte("SSND"))
	if ssndIdx < 0 || ssndIdx+16+len(want) > len(raw) {
		t.Fatalf("SSND chunk missing or truncated in 8-bit aiff")
	}
	// Sample index 2 is 0.0 -> must be 0x00 in signed two's complement AIFF (not 0x80).
	if raw[ssndIdx+16+2] != 0x00 {
		t.Fatalf("expected 0x00 silence byte in 8-bit AIFF SSND, got 0x%02x", raw[ssndIdx+16+2])
	}

	opened, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("open 8-bit aiff: %v", err)
	}
	defer func() { _ = opened.Close() }()

	decoded, err := codec.Decode(context.Background(), opened)
	if err != nil {
		t.Fatalf("decode 8-bit aiff: %v", err)
	}
	if err := decoded.Buffer.Validate(); err != nil {
		t.Fatalf("validate decoded 8-bit aiff: %v", err)
	}
	for i := range want {
		if math.Abs(decoded.Buffer.Data[i]-want[i]) > 0.02 {
			t.Fatalf("sample[%d] = %v, want %v", i, decoded.Buffer.Data[i], want[i])
		}
	}
}
