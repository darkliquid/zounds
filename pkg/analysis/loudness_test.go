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

func TestLoudnessAnalyzerComputesITUBS1770LUFSAndTruePeak(t *testing.T) {
	t.Parallel()

	// 1 kHz sine wave at 0 dBFS (peak 1.0)
	sampleRate := 48000
	duration := 2.0
	frames := int(duration * float64(sampleRate))
	data := make([]float64, frames*2) // Stereo

	for i := 0; i < frames; i++ {
		val := math.Sin(2 * math.Pi * 1000.0 * float64(i) / float64(sampleRate))
		data[i*2] = val
		data[i*2+1] = val
	}

	analyzer, err := analysis.NewLoudnessAnalyzer(nil)
	if err != nil {
		t.Fatalf("create loudness analyzer: %v", err)
	}

	ctx := analysis.ContextWithDecodedSample(context.Background(), "test.wav", zaudio.DecodeResult{
		Buffer: zaudio.PCMBuffer{
			SampleRate: sampleRate,
			Channels:   2,
			BitDepth:   16,
			Data:       data,
		},
		Info: zaudio.StreamInfo{SampleRate: sampleRate, Channels: 2, BitDepth: 16, Format: core.FormatWAV},
	})

	result, err := analyzer.Analyze(ctx, core.Sample{Path: "test.wav"})
	if err != nil {
		t.Fatalf("analyze loudness: %v", err)
	}

	lufs := result.Metrics["integrated_lufs"]
	if math.Abs(lufs-0.0) > 0.15 {
		t.Errorf("expected ~0.00 LUFS for full-scale 1kHz stereo sine, got %.3f", lufs)
	}

	truePeak := result.Metrics["true_peak_dbtp"]
	if math.Abs(truePeak-0.0) > 0.1 {
		t.Errorf("expected ~0.0 dBTP, got %.3f", truePeak)
	}
}
