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

	if math.Abs(result.Metrics["frequency_hz"]-440) > 0.5 {
		t.Fatalf("unexpected frequency: %f", result.Metrics["frequency_hz"])
	}
	if result.Attributes["note_name"] != "A4" {
		t.Fatalf("unexpected note name: %s", result.Attributes["note_name"])
	}
	if result.Metrics["confidence"] < 0.95 {
		t.Fatalf("expected high confidence > 0.95, got %f", result.Metrics["confidence"])
	}
	if math.Abs(result.Metrics["cents_from_note"]) > 1.0 {
		t.Fatalf("expected cents offset < 1.0, got %f", result.Metrics["cents_from_note"])
	}
}

func TestPitchAnalyzerDetectsSubBass808(t *testing.T) {
	t.Parallel()

	// 32.703 Hz is C1 (typical sub-bass / 808 pitch)
	freq := 32.703
	sampleRate := 44100
	buf := zaudio.PCMBuffer{
		SampleRate: sampleRate,
		Channels:   1,
		BitDepth:   16,
		Data:       sineBuffer(sampleRate*2, freq, sampleRate),
	}

	analyzer, err := analysis.NewPitchAnalyzer(nil)
	if err != nil {
		t.Fatalf("create pitch analyzer: %v", err)
	}

	ctx := analysis.ContextWithDecodedSample(context.Background(), "sub_bass.wav", zaudio.DecodeResult{
		Buffer: buf,
		Info:   zaudio.StreamInfo{SampleRate: sampleRate, Channels: 1, BitDepth: 16, Format: core.FormatWAV},
	})

	result, err := analyzer.Analyze(ctx, core.Sample{Path: "sub_bass.wav"})
	if err != nil {
		t.Fatalf("analyze pitch: %v", err)
	}

	detectedFreq := result.Metrics["frequency_hz"]
	if math.Abs(detectedFreq-freq) > 0.5 {
		t.Fatalf("expected ~%.2f Hz for sub-bass C1, got %.2f Hz", freq, detectedFreq)
	}
	if result.Attributes["note_name"] != "C1" {
		t.Fatalf("expected note name 'C1', got %q", result.Attributes["note_name"])
	}
	if result.Metrics["confidence"] < 0.90 {
		t.Fatalf("expected confidence > 0.90, got %f", result.Metrics["confidence"])
	}
}

func TestPitchAnalyzerParabolicInterpolationAccuracy(t *testing.T) {
	t.Parallel()

	// Test 1000 Hz, 1500 Hz, and 2050 Hz where integer lags have fractional remainders
	// (e.g. 44100 / 1000 = 44.1 samples, previously causing ~40 cent error)
	frequencies := []float64{1000.0, 1500.0, 2050.0}
	sampleRate := 44100

	analyzer, err := analysis.NewPitchAnalyzer(nil)
	if err != nil {
		t.Fatalf("create pitch analyzer: %v", err)
	}

	for _, freq := range frequencies {
		buf := zaudio.PCMBuffer{
			SampleRate: sampleRate,
			Channels:   1,
			BitDepth:   16,
			Data:       sineBuffer(sampleRate, freq, sampleRate),
		}

		ctx := analysis.ContextWithDecodedSample(context.Background(), "tone.wav", zaudio.DecodeResult{
			Buffer: buf,
			Info:   zaudio.StreamInfo{SampleRate: sampleRate, Channels: 1, BitDepth: 16, Format: core.FormatWAV},
		})

		result, err := analyzer.Analyze(ctx, core.Sample{Path: "tone.wav"})
		if err != nil {
			t.Fatalf("analyze pitch for %.1f Hz: %v", freq, err)
		}

		detectedFreq := result.Metrics["frequency_hz"]
		freqDiff := math.Abs(detectedFreq - freq)
		if freqDiff > 1.5 {
			t.Errorf("frequency %.1f Hz: detected %.2f Hz (diff %.2f Hz too large)", freq, detectedFreq, freqDiff)
		}

		// Calculate cents error between detected frequency and ground truth
		centsDiff := math.Abs(1200.0 * math.Log2(detectedFreq/freq))
		if centsDiff > 2.0 {
			t.Errorf("frequency %.1f Hz: cents error %.2f exceeds 2 cents threshold", freq, centsDiff)
		}
	}
}

func TestPitchAnalyzerRejectsNoise(t *testing.T) {
	t.Parallel()

	sampleRate := 44100
	noiseData := make([]float64, sampleRate)
	// Seed pseudo-random noise
	var state uint64 = 123456789
	for i := range noiseData {
		state = state*6364136223846793005 + 1442695040888963407
		noiseData[i] = (float64(state>>33)/2147483648.0)*2.0 - 1.0
	}

	buf := zaudio.PCMBuffer{
		SampleRate: sampleRate,
		Channels:   1,
		BitDepth:   16,
		Data:       noiseData,
	}

	analyzer, err := analysis.NewPitchAnalyzer(nil)
	if err != nil {
		t.Fatalf("create pitch analyzer: %v", err)
	}

	ctx := analysis.ContextWithDecodedSample(context.Background(), "noise.wav", zaudio.DecodeResult{
		Buffer: buf,
		Info:   zaudio.StreamInfo{SampleRate: sampleRate, Channels: 1, BitDepth: 16, Format: core.FormatWAV},
	})

	result, err := analyzer.Analyze(ctx, core.Sample{Path: "noise.wav"})
	if err != nil {
		t.Fatalf("analyze pitch on noise: %v", err)
	}

	if noteName, exists := result.Attributes["note_name"]; exists && noteName != "" {
		t.Fatalf("expected no note_name attribute on noise, got %q", noteName)
	}
	if conf := result.Metrics["confidence"]; conf >= 0.60 {
		t.Fatalf("expected low confidence (< 0.60) on noise, got %f", conf)
	}
	if freq := result.Metrics["frequency_hz"]; freq != 0 {
		t.Fatalf("expected frequency_hz == 0 on noise, got %f", freq)
	}
}

func TestPitchAnalyzerRejectsSilence(t *testing.T) {
	t.Parallel()

	sampleRate := 44100
	buf := zaudio.PCMBuffer{
		SampleRate: sampleRate,
		Channels:   1,
		BitDepth:   16,
		Data:       make([]float64, sampleRate),
	}

	analyzer, err := analysis.NewPitchAnalyzer(nil)
	if err != nil {
		t.Fatalf("create pitch analyzer: %v", err)
	}

	ctx := analysis.ContextWithDecodedSample(context.Background(), "silence.wav", zaudio.DecodeResult{
		Buffer: buf,
		Info:   zaudio.StreamInfo{SampleRate: sampleRate, Channels: 1, BitDepth: 16, Format: core.FormatWAV},
	})

	result, err := analyzer.Analyze(ctx, core.Sample{Path: "silence.wav"})
	if err != nil {
		t.Fatalf("analyze pitch on silence: %v", err)
	}

	if noteName, exists := result.Attributes["note_name"]; exists && noteName != "" {
		t.Fatalf("expected no note_name attribute on silence, got %q", noteName)
	}
	if conf := result.Metrics["confidence"]; conf != 0 {
		t.Fatalf("expected 0 confidence on silence, got %f", conf)
	}
}
