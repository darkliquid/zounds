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

func TestMFCCAnalyzerProducesFeatureVector(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "tone.wav")
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

	analyzer, err := analysis.NewMFCCAnalyzer(nil)
	if err != nil {
		t.Fatalf("create mfcc analyzer: %v", err)
	}

	result, err := analyzer.Analyze(context.Background(), core.Sample{
		Path:      path,
		Extension: "wav",
		Format:    core.FormatWAV,
		SizeBytes: info.Size(),
	})
	if err != nil {
		t.Fatalf("analyze mfcc: %v", err)
	}

	if len(result.FeatureVectors) != 1 {
		t.Fatalf("expected 1 feature vector, got %d", len(result.FeatureVectors))
	}
	if result.FeatureVectors[0].Dimensions != 13 {
		t.Fatalf("unexpected mfcc dimension: %d", result.FeatureVectors[0].Dimensions)
	}
	for i, value := range result.FeatureVectors[0].Values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			t.Fatalf("unexpected coefficient at %d: %f", i, value)
		}
	}
}
