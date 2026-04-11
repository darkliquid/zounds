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

func TestFormantAnalyzerFindsResonantPeaks(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "formants.wav")
	writeAnalysisWAV(t, path, zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       mixedSineBuffer(44100, 44100, 500, 1500, 2500),
	})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat wav: %v", err)
	}

	analyzer, err := analysis.NewFormantAnalyzer(nil)
	if err != nil {
		t.Fatalf("create formant analyzer: %v", err)
	}

	result, err := analyzer.Analyze(context.Background(), core.Sample{
		Path:      path,
		Extension: "wav",
		Format:    core.FormatWAV,
		SizeBytes: info.Size(),
	})
	if err != nil {
		t.Fatalf("analyze formants: %v", err)
	}

	if result.Metrics["formant_1_hz"] < 300 || result.Metrics["formant_1_hz"] > 700 {
		t.Fatalf("unexpected formant_1_hz %f", result.Metrics["formant_1_hz"])
	}
	if result.Metrics["formant_2_hz"] < 1000 || result.Metrics["formant_2_hz"] > 2000 {
		t.Fatalf("unexpected formant_2_hz %f", result.Metrics["formant_2_hz"])
	}
}
