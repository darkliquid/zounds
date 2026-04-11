package analysis_test

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/zounds/pkg/analysis"
	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
)

func TestKeyAnalyzerDetectsCMajorTriad(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "c-major.wav")
	writeAnalysisWAV(t, path, zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       mixedSineBuffer(44100, 44100, 261.63, 329.63, 392.00),
	})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat wav: %v", err)
	}

	analyzer, err := analysis.NewKeyAnalyzer(nil)
	if err != nil {
		t.Fatalf("create key analyzer: %v", err)
	}

	result, err := analyzer.Analyze(context.Background(), core.Sample{
		Path:      path,
		Extension: "wav",
		Format:    core.FormatWAV,
		SizeBytes: info.Size(),
	})
	if err != nil {
		t.Fatalf("analyze key: %v", err)
	}

	if result.Attributes["key"] != "C major" {
		t.Fatalf("unexpected key: %s", result.Attributes["key"])
	}
	if result.Metrics["key_confidence"] <= 0 {
		t.Fatalf("expected positive confidence, got %f", result.Metrics["key_confidence"])
	}
	if result.Metrics["chroma_0"] <= result.Metrics["chroma_1"] {
		t.Fatalf("expected C chroma prominence, got C=%f C#=%f", result.Metrics["chroma_0"], result.Metrics["chroma_1"])
	}
	if result.Metrics["chroma_4"] <= result.Metrics["chroma_3"] {
		t.Fatalf("expected E chroma prominence, got E=%f D#=%f", result.Metrics["chroma_4"], result.Metrics["chroma_3"])
	}
	if result.Metrics["chroma_7"] <= result.Metrics["chroma_6"] {
		t.Fatalf("expected G chroma prominence, got G=%f F#=%f", result.Metrics["chroma_7"], result.Metrics["chroma_6"])
	}
	for i := 0; i < 6; i++ {
		value := result.Metrics[fmt.Sprintf("tonnetz_%d", i)]
		if math.IsNaN(value) || math.IsInf(value, 0) {
			t.Fatalf("unexpected tonnetz_%d value: %f", i, value)
		}
	}
}
