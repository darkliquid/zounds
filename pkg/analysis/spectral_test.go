package analysis_test

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/zounds/pkg/analysis"
	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
)

func TestSpectralAnalyzerDetectsSineCharacteristics(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sine.wav")
	writeAnalysisWAV(t, path, zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       sineBuffer(44100, 440, 44100),
	})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat wav: %v", err)
	}

	analyzer, err := analysis.NewSpectralAnalyzer(nil)
	if err != nil {
		t.Fatalf("create spectral analyzer: %v", err)
	}

	result, err := analyzer.Analyze(context.Background(), core.Sample{
		Path:      path,
		Extension: "wav",
		Format:    core.FormatWAV,
		SizeBytes: info.Size(),
	})
	if err != nil {
		t.Fatalf("analyze spectral features: %v", err)
	}

	if math.Abs(result.Metrics["dominant_frequency_hz"]-440) > 30 {
		t.Fatalf("unexpected dominant frequency: %f", result.Metrics["dominant_frequency_hz"])
	}
	if result.Metrics["spectral_centroid_hz"] <= 0 {
		t.Fatalf("expected positive centroid, got %f", result.Metrics["spectral_centroid_hz"])
	}
	if result.Metrics["zero_crossing_rate"] < 0.015 || result.Metrics["zero_crossing_rate"] > 0.025 {
		t.Fatalf("unexpected zero crossing rate: %f", result.Metrics["zero_crossing_rate"])
	}
}
