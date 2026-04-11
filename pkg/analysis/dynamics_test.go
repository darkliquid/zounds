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

func TestDynamicsAnalyzerCapturesRangeAndTransients(t *testing.T) {
	t.Parallel()

	data := append(fillBuffer(2048, 0.0), fillBuffer(2048, 0.9)...)
	data = append(data, fillBuffer(2048, 0.2)...)

	path := filepath.Join(t.TempDir(), "dynamic.wav")
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

	analyzer, err := analysis.NewDynamicsAnalyzer(nil)
	if err != nil {
		t.Fatalf("create dynamics analyzer: %v", err)
	}

	result, err := analyzer.Analyze(context.Background(), core.Sample{
		Path:      path,
		Extension: "wav",
		Format:    core.FormatWAV,
		SizeBytes: info.Size(),
	})
	if err != nil {
		t.Fatalf("analyze dynamics: %v", err)
	}

	if result.Metrics["dynamic_range_db"] <= 0 {
		t.Fatalf("expected positive dynamic range, got %f", result.Metrics["dynamic_range_db"])
	}
	if result.Metrics["attack_sharpness"] <= 0 {
		t.Fatalf("expected positive attack sharpness, got %f", result.Metrics["attack_sharpness"])
	}
	if result.Metrics["transient_rate"] <= 0 {
		t.Fatalf("expected positive transient rate, got %f", result.Metrics["transient_rate"])
	}
	if result.Metrics["temporal_centroid"] <= 0.45 {
		t.Fatalf("expected temporal centroid later in file, got %f", result.Metrics["temporal_centroid"])
	}
}
