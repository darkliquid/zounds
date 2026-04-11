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

func TestLoudnessAnalyzerComputesExpectedLevels(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "constant.wav")
	writeAnalysisWAV(t, path, zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       fillBuffer(44100, 0.5),
	})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat wav: %v", err)
	}

	analyzer, err := analysis.NewLoudnessAnalyzer(nil)
	if err != nil {
		t.Fatalf("create loudness analyzer: %v", err)
	}

	result, err := analyzer.Analyze(context.Background(), core.Sample{
		Path:      path,
		Extension: "wav",
		Format:    core.FormatWAV,
		SizeBytes: info.Size(),
	})
	if err != nil {
		t.Fatalf("analyze loudness: %v", err)
	}

	if math.Abs(result.Metrics["peak"]-0.5) > 0.001 {
		t.Fatalf("unexpected peak: %f", result.Metrics["peak"])
	}
	if math.Abs(result.Metrics["rms"]-0.5) > 0.001 {
		t.Fatalf("unexpected rms: %f", result.Metrics["rms"])
	}
}
