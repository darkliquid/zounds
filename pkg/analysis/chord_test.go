package analysis_test

import (
	"context"
	"math"
	"math/rand"
	"testing"

	"github.com/darkliquid/zounds/pkg/analysis"
	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
)

func synthBuffer(sampleRate int, duration float64, freqs ...float64) zaudio.PCMBuffer {
	frames := int(float64(sampleRate) * duration)
	data := make([]float64, frames)
	for i := range data {
		t := float64(i) / float64(sampleRate)
		var val float64
		for _, f := range freqs {
			val += math.Sin(2 * math.Pi * f * t)
		}
		data[i] = val / float64(len(freqs))
	}
	return zaudio.PCMBuffer{
		SampleRate: sampleRate,
		Channels:   1,
		BitDepth:   16,
		Data:       data,
	}
}

func TestChordAnalyzerDetectsMajorTriad(t *testing.T) {
	t.Parallel()

	// C Major triad: C4 (261.63 Hz), E4 (329.63 Hz), G4 (392.00 Hz)
	buffer := synthBuffer(44100, 1.0, 261.63, 329.63, 392.00)

	analyzer, err := analysis.NewChordAnalyzer(nil)
	if err != nil {
		t.Fatalf("NewChordAnalyzer: %v", err)
	}

	ctx := analysis.ContextWithDecodedSample(context.Background(), "c_major.wav", zaudio.DecodeResult{
		Buffer: buffer,
		Info:   zaudio.StreamInfo{SampleRate: 44100, Channels: 1, BitDepth: 16, Format: core.FormatWAV},
	})

	result, err := analyzer.Analyze(ctx, core.Sample{Path: "c_major.wav"})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if result.Attributes["chord_root"] != "C" {
		t.Errorf("expected chord_root 'C', got %q", result.Attributes["chord_root"])
	}
	if result.Attributes["chord_quality"] != "major" {
		t.Errorf("expected chord_quality 'major', got %q", result.Attributes["chord_quality"])
	}
	if result.Attributes["chord"] != "C major" {
		t.Errorf("expected chord 'C major', got %q", result.Attributes["chord"])
	}
	if result.Metrics["chord_confidence"] < 0.65 {
		t.Errorf("expected chord_confidence >= 0.65, got %f", result.Metrics["chord_confidence"])
	}
}

func TestChordAnalyzerDetectsMinorTriad(t *testing.T) {
	t.Parallel()

	// A Minor triad: A3 (220.00 Hz), C4 (261.63 Hz), E4 (329.63 Hz)
	buffer := synthBuffer(44100, 1.0, 220.00, 261.63, 329.63)

	analyzer, err := analysis.NewChordAnalyzer(nil)
	if err != nil {
		t.Fatalf("NewChordAnalyzer: %v", err)
	}

	ctx := analysis.ContextWithDecodedSample(context.Background(), "a_minor.wav", zaudio.DecodeResult{
		Buffer: buffer,
		Info:   zaudio.StreamInfo{SampleRate: 44100, Channels: 1, BitDepth: 16, Format: core.FormatWAV},
	})

	result, err := analyzer.Analyze(ctx, core.Sample{Path: "a_minor.wav"})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if result.Attributes["chord_root"] != "A" {
		t.Errorf("expected chord_root 'A', got %q", result.Attributes["chord_root"])
	}
	if result.Attributes["chord_quality"] != "minor" {
		t.Errorf("expected chord_quality 'minor', got %q", result.Attributes["chord_quality"])
	}
	if result.Attributes["chord"] != "A minor" {
		t.Errorf("expected chord 'A minor', got %q", result.Attributes["chord"])
	}
	if result.Metrics["chord_confidence"] < 0.65 {
		t.Errorf("expected chord_confidence >= 0.65, got %f", result.Metrics["chord_confidence"])
	}
}

func TestChordAnalyzerDetectsDominantSeventh(t *testing.T) {
	t.Parallel()

	// G7: G3 (196.00 Hz), B3 (246.94 Hz), D4 (293.66 Hz), F4 (349.23 Hz)
	buffer := synthBuffer(44100, 1.0, 196.00, 246.94, 293.66, 349.23)

	analyzer, err := analysis.NewChordAnalyzer(nil)
	if err != nil {
		t.Fatalf("NewChordAnalyzer: %v", err)
	}

	ctx := analysis.ContextWithDecodedSample(context.Background(), "g7.wav", zaudio.DecodeResult{
		Buffer: buffer,
		Info:   zaudio.StreamInfo{SampleRate: 44100, Channels: 1, BitDepth: 16, Format: core.FormatWAV},
	})

	result, err := analyzer.Analyze(ctx, core.Sample{Path: "g7.wav"})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if result.Attributes["chord_root"] != "G" {
		t.Errorf("expected chord_root 'G', got %q", result.Attributes["chord_root"])
	}
	if result.Attributes["chord_quality"] != "7" {
		t.Errorf("expected chord_quality '7', got %q", result.Attributes["chord_quality"])
	}
	if result.Attributes["chord"] != "G 7" {
		t.Errorf("expected chord 'G 7', got %q", result.Attributes["chord"])
	}
}

func TestChordAnalyzerDetectsSus4(t *testing.T) {
	t.Parallel()

	// Dsus4: D3 (146.83 Hz), G3 (196.00 Hz), A3 (220.00 Hz)
	buffer := synthBuffer(44100, 1.0, 146.83, 196.00, 220.00)

	analyzer, err := analysis.NewChordAnalyzer(nil)
	if err != nil {
		t.Fatalf("NewChordAnalyzer: %v", err)
	}

	ctx := analysis.ContextWithDecodedSample(context.Background(), "dsus4.wav", zaudio.DecodeResult{
		Buffer: buffer,
		Info:   zaudio.StreamInfo{SampleRate: 44100, Channels: 1, BitDepth: 16, Format: core.FormatWAV},
	})

	result, err := analyzer.Analyze(ctx, core.Sample{Path: "dsus4.wav"})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if result.Attributes["chord_root"] != "D" {
		t.Errorf("expected chord_root 'D', got %q", result.Attributes["chord_root"])
	}
	if result.Attributes["chord_quality"] != "sus4" {
		t.Errorf("expected chord_quality 'sus4', got %q", result.Attributes["chord_quality"])
	}
}

func TestChordAnalyzerRejectsSingleNote(t *testing.T) {
	t.Parallel()

	// Single note C4 (261.63 Hz) with acoustic overtones (523.26, 784.89, 1046.52, 1308.15 Hz)
	frames := 44100
	data := make([]float64, frames)
	f0 := 261.63
	for i := range data {
		time := float64(i) / 44100.0
		// Harmonics 1 to 5 with natural acoustic rolloff (1/k)
		data[i] = 1.0*math.Sin(2*math.Pi*f0*time) +
			0.5*math.Sin(2*math.Pi*2*f0*time) +
			0.3*math.Sin(2*math.Pi*3*f0*time) + // G5
			0.2*math.Sin(2*math.Pi*4*f0*time) +
			0.15*math.Sin(2*math.Pi*5*f0*time) // E6
	}

	buffer := zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       data,
	}

	analyzer, err := analysis.NewChordAnalyzer(nil)
	if err != nil {
		t.Fatalf("NewChordAnalyzer: %v", err)
	}

	ctx := analysis.ContextWithDecodedSample(context.Background(), "single_c.wav", zaudio.DecodeResult{
		Buffer: buffer,
		Info:   zaudio.StreamInfo{SampleRate: 44100, Channels: 1, BitDepth: 16, Format: core.FormatWAV},
	})

	result, err := analyzer.Analyze(ctx, core.Sample{Path: "single_c.wav"})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	// Because it's a single note, it must NOT be identified as a chord!
	if chord := result.Attributes["chord"]; chord != "" {
		t.Errorf("single note misidentified as chord %q", chord)
	}
	if result.Metrics["chord_confidence"] > 0.5 {
		t.Errorf("single note confidence must be low, got %f", result.Metrics["chord_confidence"])
	}
}

func TestChordAnalyzerRejectsNoise(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(42))
	frames := 44100
	data := make([]float64, frames)
	for i := range data {
		data[i] = r.Float64()*2.0 - 1.0
	}

	buffer := zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       data,
	}

	analyzer, err := analysis.NewChordAnalyzer(nil)
	if err != nil {
		t.Fatalf("NewChordAnalyzer: %v", err)
	}

	ctx := analysis.ContextWithDecodedSample(context.Background(), "noise.wav", zaudio.DecodeResult{
		Buffer: buffer,
		Info:   zaudio.StreamInfo{SampleRate: 44100, Channels: 1, BitDepth: 16, Format: core.FormatWAV},
	})

	result, err := analyzer.Analyze(ctx, core.Sample{Path: "noise.wav"})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if chord := result.Attributes["chord"]; chord != "" {
		t.Errorf("noise misidentified as chord %q", chord)
	}
	if result.Metrics["chord_confidence"] > 0.4 {
		t.Errorf("noise chord confidence must be low, got %f", result.Metrics["chord_confidence"])
	}
}
