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

func TestHarmonicsAnalyzerSeparatesHarmonicAndInharmonicMaterial(t *testing.T) {
	t.Parallel()

	harmonicPath := filepath.Join(t.TempDir(), "harmonic.wav")
	writeAnalysisWAV(t, harmonicPath, zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       mixedSineBuffer(44100, 44100, 220, 440, 660),
	})

	inharmonicPath := filepath.Join(t.TempDir(), "inharmonic.wav")
	writeAnalysisWAV(t, inharmonicPath, zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       mixedSineBuffer(44100, 44100, 220, 311, 587),
	})

	harmonicInfo, err := os.Stat(harmonicPath)
	if err != nil {
		t.Fatalf("stat harmonic wav: %v", err)
	}
	inharmonicInfo, err := os.Stat(inharmonicPath)
	if err != nil {
		t.Fatalf("stat inharmonic wav: %v", err)
	}

	analyzer, err := analysis.NewHarmonicsAnalyzer(nil)
	if err != nil {
		t.Fatalf("create harmonics analyzer: %v", err)
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
	inharmonicResult, err := analyzer.Analyze(context.Background(), core.Sample{
		Path:      inharmonicPath,
		Extension: "wav",
		Format:    core.FormatWAV,
		SizeBytes: inharmonicInfo.Size(),
	})
	if err != nil {
		t.Fatalf("analyze inharmonic sample: %v", err)
	}

	if harmonicResult.Metrics["harmonic_ratio"] <= inharmonicResult.Metrics["harmonic_ratio"] {
		t.Fatalf("expected harmonic ratio to be higher for harmonic sample, got harmonic=%f inharmonic=%f",
			harmonicResult.Metrics["harmonic_ratio"], inharmonicResult.Metrics["harmonic_ratio"])
	}
	if harmonicResult.Metrics["inharmonicity"] >= inharmonicResult.Metrics["inharmonicity"] {
		t.Fatalf("expected inharmonicity to be lower for harmonic sample, got harmonic=%f inharmonic=%f",
			harmonicResult.Metrics["inharmonicity"], inharmonicResult.Metrics["inharmonicity"])
	}
}
