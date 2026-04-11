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

func TestPitchAnalyzerDetectsA440(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "a440.wav")
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

	analyzer, err := analysis.NewPitchAnalyzer(nil)
	if err != nil {
		t.Fatalf("create pitch analyzer: %v", err)
	}

	result, err := analyzer.Analyze(context.Background(), core.Sample{
		Path:      path,
		Extension: "wav",
		Format:    core.FormatWAV,
		SizeBytes: info.Size(),
	})
	if err != nil {
		t.Fatalf("analyze pitch: %v", err)
	}

	if math.Abs(result.Metrics["frequency_hz"]-440) > 10 {
		t.Fatalf("unexpected frequency: %f", result.Metrics["frequency_hz"])
	}
	if result.Attributes["note_name"] != "A4" {
		t.Fatalf("unexpected note name: %s", result.Attributes["note_name"])
	}
	if result.Metrics["confidence"] <= 0 {
		t.Fatalf("expected positive confidence, got %f", result.Metrics["confidence"])
	}
}
