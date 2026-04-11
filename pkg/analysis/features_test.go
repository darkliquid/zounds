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

func TestFeatureVectorBuilderUsesStableOrder(t *testing.T) {
	t.Parallel()

	builder := analysis.NewFeatureVectorBuilder(nil)
	vector, err := builder.Build(42,
		core.AnalysisResult{Metrics: map[string]float64{"tempo_bpm": 128, "mfcc_0": 1.5}},
		core.AnalysisResult{Metrics: map[string]float64{"rms": 0.25}},
	)
	if err != nil {
		t.Fatalf("build feature vector: %v", err)
	}

	names := analysis.FeatureNames()
	if vector.Dimensions != len(names) {
		t.Fatalf("unexpected vector dimensions: got=%d want=%d", vector.Dimensions, len(names))
	}

	index := func(name string) int {
		for i, candidate := range names {
			if candidate == name {
				return i
			}
		}
		return -1
	}

	if got := vector.Values[index("tempo_bpm")]; got != 128 {
		t.Fatalf("unexpected tempo value: %f", got)
	}
	if got := vector.Values[index("mfcc_0")]; got != 1.5 {
		t.Fatalf("unexpected mfcc_0 value: %f", got)
	}
	if got := vector.Values[index("rms")]; got != 0.25 {
		t.Fatalf("unexpected rms value: %f", got)
	}
}

func TestFeatureVectorBuilderWorksWithAnalyzerOutputs(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sample.wav")
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

	sample := core.Sample{
		ID:        7,
		Path:      path,
		Extension: "wav",
		Format:    core.FormatWAV,
		SizeBytes: info.Size(),
	}

	ctx := context.Background()
	metadataAnalyzer, err := analysis.NewMetadataAnalyzer(nil)
	if err != nil {
		t.Fatalf("create metadata analyzer: %v", err)
	}
	spectralAnalyzer, err := analysis.NewSpectralAnalyzer(nil)
	if err != nil {
		t.Fatalf("create spectral analyzer: %v", err)
	}
	pitchAnalyzer, err := analysis.NewPitchAnalyzer(nil)
	if err != nil {
		t.Fatalf("create pitch analyzer: %v", err)
	}
	loudnessAnalyzer, err := analysis.NewLoudnessAnalyzer(nil)
	if err != nil {
		t.Fatalf("create loudness analyzer: %v", err)
	}
	mfccAnalyzer, err := analysis.NewMFCCAnalyzer(nil)
	if err != nil {
		t.Fatalf("create mfcc analyzer: %v", err)
	}

	metadataResult, err := metadataAnalyzer.Analyze(ctx, sample)
	if err != nil {
		t.Fatalf("metadata analyze: %v", err)
	}
	spectralResult, err := spectralAnalyzer.Analyze(ctx, sample)
	if err != nil {
		t.Fatalf("spectral analyze: %v", err)
	}
	pitchResult, err := pitchAnalyzer.Analyze(ctx, sample)
	if err != nil {
		t.Fatalf("pitch analyze: %v", err)
	}
	loudnessResult, err := loudnessAnalyzer.Analyze(ctx, sample)
	if err != nil {
		t.Fatalf("loudness analyze: %v", err)
	}
	mfccResult, err := mfccAnalyzer.Analyze(ctx, sample)
	if err != nil {
		t.Fatalf("mfcc analyze: %v", err)
	}

	builder := analysis.NewFeatureVectorBuilder(nil)
	vector, err := builder.Build(sample.ID, metadataResult, spectralResult, pitchResult, loudnessResult, mfccResult)
	if err != nil {
		t.Fatalf("build vector: %v", err)
	}

	if vector.SampleID != sample.ID {
		t.Fatalf("unexpected sample id: %d", vector.SampleID)
	}
	if vector.Dimensions != len(vector.Values) {
		t.Fatalf("dimension/value length mismatch: %d vs %d", vector.Dimensions, len(vector.Values))
	}
	if vector.Values[0] == 0 {
		t.Fatalf("expected non-zero leading metadata feature")
	}
}
