package analysis_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/zounds/pkg/analysis"
	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
)

func TestQualityAnalyzerDetectsClippingAndOffset(t *testing.T) {
	t.Parallel()

	data := append(fillBuffer(2048, 1.0), fillBuffer(2048, 0.002)...)
	for i := 0; i < 2048; i++ {
		data = append(data, 0.25)
	}

	path := filepath.Join(t.TempDir(), "quality.wav")
	writeAnalysisWAV(t, path, zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       data,
	})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat wav: %v", err)
	}

	analyzer, err := analysis.NewQualityAnalyzer(nil)
	if err != nil {
		t.Fatalf("create quality analyzer: %v", err)
	}

	result, err := analyzer.Analyze(context.Background(), core.Sample{
		Path:      path,
		Extension: "wav",
		Format:    core.FormatWAV,
		SizeBytes: info.Size(),
	})
	if err != nil {
		t.Fatalf("analyze quality: %v", err)
	}

	if result.Metrics["clipping_ratio"] <= 0 {
		t.Fatalf("expected clipping ratio > 0, got %f", result.Metrics["clipping_ratio"])
	}
	if result.Metrics["dc_offset"] <= 0 {
		t.Fatalf("expected positive dc offset, got %f", result.Metrics["dc_offset"])
	}
	if result.Metrics["estimated_snr_db"] <= 0 {
		t.Fatalf("expected positive estimated snr, got %f", result.Metrics["estimated_snr_db"])
	}
}
