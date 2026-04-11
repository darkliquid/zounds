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

func TestBeatAnalyzerEstimatesImpulseTrainTempo(t *testing.T) {
	t.Parallel()

	const (
		sampleRate = 44100
		bpm        = 120.0
		duration   = 4.0
	)

	path := filepath.Join(t.TempDir(), "beat.wav")
	writeAnalysisWAV(t, path, zaudio.PCMBuffer{
		SampleRate: sampleRate,
		Channels:   1,
		BitDepth:   16,
		Data:       impulseTrainBuffer(int(duration*sampleRate), bpm, sampleRate),
	})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat wav: %v", err)
	}

	analyzer, err := analysis.NewBeatAnalyzer(nil)
	if err != nil {
		t.Fatalf("create beat analyzer: %v", err)
	}

	result, err := analyzer.Analyze(context.Background(), core.Sample{
		Path:      path,
		Extension: "wav",
		Format:    core.FormatWAV,
		SizeBytes: info.Size(),
	})
	if err != nil {
		t.Fatalf("analyze beat: %v", err)
	}

	if math.Abs(result.Metrics["tempo_bpm"]-bpm) > 5 {
		t.Fatalf("unexpected bpm: %f", result.Metrics["tempo_bpm"])
	}
	if result.Metrics["beat_count_estimate"] <= 0 {
		t.Fatalf("expected positive beat count, got %f", result.Metrics["beat_count_estimate"])
	}
}
