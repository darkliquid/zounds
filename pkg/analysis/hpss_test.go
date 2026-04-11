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

func TestHPSSAnalyzerDistinguishesHarmonicAndPercussiveSignals(t *testing.T) {
	t.Parallel()

	harmonicPath := filepath.Join(t.TempDir(), "harmonic.wav")
	writeAnalysisWAV(t, harmonicPath, zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       sineBuffer(44100, 440, 44100),
	})

	percussivePath := filepath.Join(t.TempDir(), "percussive.wav")
	writeAnalysisWAV(t, percussivePath, zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       impulseTrainBuffer(44100, 240, 44100),
	})

	harmonicInfo, err := os.Stat(harmonicPath)
	if err != nil {
		t.Fatalf("stat harmonic wav: %v", err)
	}
	percussiveInfo, err := os.Stat(percussivePath)
	if err != nil {
		t.Fatalf("stat percussive wav: %v", err)
	}

	analyzer, err := analysis.NewHPSSAnalyzer(nil)
	if err != nil {
		t.Fatalf("create hpss analyzer: %v", err)
	}

	harmonicResult, err := analyzer.Analyze(context.Background(), core.Sample{
		Path:      harmonicPath,
		Extension: "wav",
		Format:    core.FormatWAV,
		SizeBytes: harmonicInfo.Size(),
	})
	if err != nil {
		t.Fatalf("analyze harmonic sample: %v", err)
	}
	percussiveResult, err := analyzer.Analyze(context.Background(), core.Sample{
		Path:      percussivePath,
		Extension: "wav",
		Format:    core.FormatWAV,
		SizeBytes: percussiveInfo.Size(),
	})
	if err != nil {
		t.Fatalf("analyze percussive sample: %v", err)
	}

	if harmonicResult.Metrics["harmonic_energy_ratio"] <= percussiveResult.Metrics["harmonic_energy_ratio"] {
		t.Fatalf("expected harmonic sample to have higher harmonic ratio, got harmonic=%f percussive=%f",
			harmonicResult.Metrics["harmonic_energy_ratio"], percussiveResult.Metrics["harmonic_energy_ratio"])
	}
	if percussiveResult.Metrics["percussive_energy_ratio"] <= harmonicResult.Metrics["percussive_energy_ratio"] {
		t.Fatalf("expected percussive sample to have higher percussive ratio, got harmonic=%f percussive=%f",
			harmonicResult.Metrics["percussive_energy_ratio"], harmonicResult.Metrics["percussive_energy_ratio"])
	}
}
