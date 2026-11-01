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

func TestBeatAnalyzerEstimatesImpulseTrainTempo(t *testing.T) {
	t.Parallel()

	const (
		sampleRate = 44100
		bpm        = 120.0
		duration   = 4.0
	)

	path := filepath.Join(t.TempDir(), "beat.wav")
	writeAnalysisWAV(t, path, zaudio.PCMBuffer{
		SampleRate: sampleRate,
		Channels:   1,
		BitDepth:   16,
		Data:       impulseTrainBuffer(int(duration*sampleRate), bpm, sampleRate),
	})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat wav: %v", err)
	}

	analyzer, err := analysis.NewBeatAnalyzer(nil)
	if err != nil {
		t.Fatalf("create beat analyzer: %v", err)
	}

	result, err := analyzer.Analyze(context.Background(), core.Sample{
		Path:      path,
		Extension: "wav",
		Format:    core.FormatWAV,
		SizeBytes: info.Size(),
	})
	if err != nil {
		t.Fatalf("analyze beat: %v", err)
	}

	if math.Abs(result.Metrics["tempo_bpm"]-bpm) > 2.0 {
		t.Fatalf("unexpected bpm: %f (expected ~%.1f)", result.Metrics["tempo_bpm"], bpm)
	}
	if result.Metrics["tempo_confidence"] < 0.60 {
		t.Fatalf("expected tempo_confidence >= 0.60, got %f", result.Metrics["tempo_confidence"])
	}
	if result.Attributes["tempo_bpm"] == "" {
		t.Fatalf("expected attribute tempo_bpm to be set, got empty")
	}
	if result.Metrics["beat_count_estimate"] <= 0 {
		t.Fatalf("expected positive beat count, got %f", result.Metrics["beat_count_estimate"])
	}
}

func TestBeatAnalyzerRejectsOneShot(t *testing.T) {
	t.Parallel()

	// A short 0.3s one-shot hit (decaying transient)
	const sampleRate = 44100
	const duration = 0.3
	frames := int(duration * sampleRate)
	data := make([]float64, frames)
	for i := range data {
		tSec := float64(i) / float64(sampleRate)
		data[i] = math.Sin(2*math.Pi*60*tSec) * math.Exp(-tSec*30.0)
	}

	analyzer, err := analysis.NewBeatAnalyzer(nil)
	if err != nil {
		t.Fatalf("create beat analyzer: %v", err)
	}

	ctx := analysis.ContextWithDecodedSample(context.Background(), "oneshot.wav", zaudio.DecodeResult{
		Buffer: zaudio.PCMBuffer{
			SampleRate: sampleRate,
			Channels:   1,
			BitDepth:   16,
			Data:       data,
		},
		Info: zaudio.StreamInfo{SampleRate: sampleRate, Channels: 1, BitDepth: 16, Format: core.FormatWAV},
	})

	result, err := analyzer.Analyze(ctx, core.Sample{Path: "oneshot.wav"})
	if err != nil {
		t.Fatalf("analyze beat: %v", err)
	}

	if conf := result.Metrics["tempo_confidence"]; conf >= 0.50 {
		t.Fatalf("expected low confidence (< 0.50) on one-shot, got %f", conf)
	}
	if tempo := result.Metrics["tempo_bpm"]; tempo != 0 {
		t.Fatalf("expected tempo_bpm == 0 on one-shot, got %f", tempo)
	}
	if attr, exists := result.Attributes["tempo_bpm"]; exists && attr != "" {
		t.Fatalf("expected no tempo_bpm attribute on one-shot, got %q", attr)
	}
}

func TestBeatAnalyzerRejectsNoise(t *testing.T) {
	t.Parallel()

	const sampleRate = 44100
	const duration = 2.0
	frames := int(duration * sampleRate)
	data := make([]float64, frames)
	var state uint64 = 987654321
	for i := range data {
		state = state*6364136223846793005 + 1442695040888963407
		data[i] = (float64(state>>33)/2147483648.0)*2.0 - 1.0
	}

	analyzer, err := analysis.NewBeatAnalyzer(nil)
	if err != nil {
		t.Fatalf("create beat analyzer: %v", err)
	}

	ctx := analysis.ContextWithDecodedSample(context.Background(), "noise.wav", zaudio.DecodeResult{
		Buffer: zaudio.PCMBuffer{
			SampleRate: sampleRate,
			Channels:   1,
			BitDepth:   16,
			Data:       data,
		},
		Info: zaudio.StreamInfo{SampleRate: sampleRate, Channels: 1, BitDepth: 16, Format: core.FormatWAV},
	})

	result, err := analyzer.Analyze(ctx, core.Sample{Path: "noise.wav"})
	if err != nil {
		t.Fatalf("analyze beat: %v", err)
	}

	if conf := result.Metrics["tempo_confidence"]; conf >= 0.50 {
		t.Fatalf("expected low confidence (< 0.50) on noise, got %f", conf)
	}
	if attr, exists := result.Attributes["tempo_bpm"]; exists && attr != "" {
		t.Fatalf("expected no tempo_bpm attribute on noise, got %q", attr)
	}
}

func TestBeatAnalyzerDetectsVariousTemposWithoutBias(t *testing.T) {
	t.Parallel()

	testTempos := []float64{80.0, 128.0, 174.0}
	const sampleRate = 44100
	const duration = 5.0

	analyzer, err := analysis.NewBeatAnalyzer(nil)
	if err != nil {
		t.Fatalf("create beat analyzer: %v", err)
	}

	for _, targetBPM := range testTempos {
		data := impulseTrainBuffer(int(duration*sampleRate), targetBPM, sampleRate)
		ctx := analysis.ContextWithDecodedSample(context.Background(), "train.wav", zaudio.DecodeResult{
			Buffer: zaudio.PCMBuffer{
				SampleRate: sampleRate,
				Channels:   1,
				BitDepth:   16,
				Data:       data,
			},
			Info: zaudio.StreamInfo{SampleRate: sampleRate, Channels: 1, BitDepth: 16, Format: core.FormatWAV},
		})

		result, err := analyzer.Analyze(ctx, core.Sample{Path: "train.wav"})
		if err != nil {
			t.Fatalf("analyze beat for %.1f BPM: %v", targetBPM, err)
		}

		detectedBPM := result.Metrics["tempo_bpm"]
		if math.Abs(detectedBPM-targetBPM) > 3.0 {
			t.Errorf("target %.1f BPM: detected %.2f BPM (error too large; possible bias)", targetBPM, detectedBPM)
		}
		if result.Metrics["tempo_confidence"] < 0.60 {
			t.Errorf("target %.1f BPM: expected confidence >= 0.60, got %f", targetBPM, result.Metrics["tempo_confidence"])
		}
	}
}
