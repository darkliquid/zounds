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

func TestSpliceAnalyzerDetectsAbruptSpectralChange(t *testing.T) {
	t.Parallel()

	spliced := append(sineBuffer(22050, 440, 44100), sineBuffer(22050, 880, 44100)...)
	smooth := sineBuffer(44100, 440, 44100)

	splicedPath := filepath.Join(t.TempDir(), "spliced.wav")
	writeAnalysisWAV(t, splicedPath, zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       spliced,
	})
	smoothPath := filepath.Join(t.TempDir(), "smooth.wav")
	writeAnalysisWAV(t, smoothPath, zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       smooth,
	})

	splicedInfo, err := os.Stat(splicedPath)
	if err != nil {
		t.Fatalf("stat spliced wav: %v", err)
	}
	smoothInfo, err := os.Stat(smoothPath)
	if err != nil {
		t.Fatalf("stat smooth wav: %v", err)
	}

	analyzer, err := analysis.NewSpliceAnalyzer(nil)
	if err != nil {
		t.Fatalf("create splice analyzer: %v", err)
	}

	splicedResult, err := analyzer.Analyze(context.Background(), core.Sample{
		Path:      splicedPath,
		Extension: "wav",
		Format:    core.FormatWAV,
		SizeBytes: splicedInfo.Size(),
	})
	if err != nil {
		t.Fatalf("analyze spliced sample: %v", err)
	}
	smoothResult, err := analyzer.Analyze(context.Background(), core.Sample{
		Path:      smoothPath,
		Extension: "wav",
		Format:    core.FormatWAV,
		SizeBytes: smoothInfo.Size(),
	})
	if err != nil {
		t.Fatalf("analyze smooth sample: %v", err)
	}

	if splicedResult.Metrics["splice_max_strength"] <= smoothResult.Metrics["splice_max_strength"] {
		t.Fatalf("expected spliced sample to have higher max splice strength, got spliced=%f smooth=%f",
			splicedResult.Metrics["splice_max_strength"], smoothResult.Metrics["splice_max_strength"])
	}
}
