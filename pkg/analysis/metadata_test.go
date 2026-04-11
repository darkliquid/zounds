package analysis_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/zounds/pkg/analysis"
	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/audio/wav"
	"github.com/darkliquid/zounds/pkg/core"
)

func TestMetadataAnalyzerExtractsWAVMetrics(t *testing.T) {
	t.Parallel()

	buffer := zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   2,
		BitDepth:   16,
		Data:       make([]float64, 44100*2),
	}

	path := filepath.Join(t.TempDir(), "tone.wav")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create wav file: %v", err)
	}
	if err := wav.New().Encode(context.Background(), file, buffer); err != nil {
		t.Fatalf("encode wav file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close wav file: %v", err)
	}

	analyzer, err := analysis.NewMetadataAnalyzer(nil)
	if err != nil {
		t.Fatalf("create metadata analyzer: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat wav file: %v", err)
	}

	result, err := analyzer.Analyze(context.Background(), core.Sample{
		Path:       path,
		Extension:  "wav",
		Format:     core.FormatWAV,
		SizeBytes:  info.Size(),
		ModifiedAt: info.ModTime(),
	})
	if err != nil {
		t.Fatalf("analyze metadata: %v", err)
	}

	if got := result.Attributes["format"]; got != "wav" {
		t.Fatalf("unexpected format attribute: %s", got)
	}
	if got := result.Attributes["channel_layout"]; got != "stereo" {
		t.Fatalf("unexpected channel layout: %s", got)
	}
	if got := result.Metrics["sample_rate"]; got != 44100 {
		t.Fatalf("unexpected sample rate: %f", got)
	}
	if got := result.Metrics["channels"]; got != 2 {
		t.Fatalf("unexpected channels: %f", got)
	}
}
