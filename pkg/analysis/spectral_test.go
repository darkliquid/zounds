package analysis_test

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/zounds/pkg/analysis"
	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
)

func TestSpectralAnalyzerDetectsSineCharacteristics(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sine.wav")
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

	analyzer, err := analysis.NewSpectralAnalyzer(nil)
	if err != nil {
		t.Fatalf("create spectral analyzer: %v", err)
	}

	result, err := analyzer.Analyze(context.Background(), core.Sample{
		Path:      path,
		Extension: "wav",
		Format:    core.FormatWAV,
		SizeBytes: info.Size(),
	})
	if err != nil {
		t.Fatalf("analyze spectral features: %v", err)
	}

	if math.Abs(result.Metrics["dominant_frequency_hz"]-440) > 30 {
		t.Fatalf("unexpected dominant frequency: %f", result.Metrics["dominant_frequency_hz"])
	}
	if result.Metrics["spectral_centroid_hz"] <= 0 {
		t.Fatalf("expected positive centroid, got %f", result.Metrics["spectral_centroid_hz"])
	}
	if result.Metrics["zero_crossing_rate"] < 0.015 || result.Metrics["zero_crossing_rate"] > 0.025 {
		t.Fatalf("unexpected zero crossing rate: %f", result.Metrics["zero_crossing_rate"])
	}
	if result.Metrics["spectral_bandwidth_hz"] <= 0 {
		t.Fatalf("expected positive spectral bandwidth, got %f", result.Metrics["spectral_bandwidth_hz"])
	}
	for i := 0; i < 7; i++ {
		key := fmt.Sprintf("spectral_contrast_%d", i)
		if _, ok := result.Metrics[key]; !ok {
			t.Fatalf("missing %s metric", key)
		}
	}
}

func TestSpectralAnalyzerDistinguishesBandwidthAndContrast(t *testing.T) {
	t.Parallel()

	sinePath := filepath.Join(t.TempDir(), "sine.wav")
	writeAnalysisWAV(t, sinePath, zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       sineBuffer(44100, 440, 44100),
	})
	noisePath := filepath.Join(t.TempDir(), "noise.wav")
	writeAnalysisWAV(t, noisePath, zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       randomNoiseBuffer(44100, 1337),
	})

	sineInfo, err := os.Stat(sinePath)
	if err != nil {
		t.Fatalf("stat sine wav: %v", err)
	}
	noiseInfo, err := os.Stat(noisePath)
	if err != nil {
		t.Fatalf("stat noise wav: %v", err)
	}

	analyzer, err := analysis.NewSpectralAnalyzer(nil)
	if err != nil {
		t.Fatalf("create spectral analyzer: %v", err)
	}

	sineResult, err := analyzer.Analyze(context.Background(), core.Sample{
		Path:      sinePath,
		Extension: "wav",
		Format:    core.FormatWAV,
		SizeBytes: sineInfo.Size(),
	})
	if err != nil {
		t.Fatalf("analyze sine spectral features: %v", err)
	}
	noiseResult, err := analyzer.Analyze(context.Background(), core.Sample{
		Path:      noisePath,
		Extension: "wav",
		Format:    core.FormatWAV,
		SizeBytes: noiseInfo.Size(),
	})
	if err != nil {
		t.Fatalf("analyze noise spectral features: %v", err)
	}

	if sineResult.Metrics["spectral_bandwidth_hz"] >= noiseResult.Metrics["spectral_bandwidth_hz"] {
		t.Fatalf("expected sine bandwidth < noise bandwidth, got sine=%f noise=%f",
			sineResult.Metrics["spectral_bandwidth_hz"], noiseResult.Metrics["spectral_bandwidth_hz"])
	}

	averageContrast := func(metrics map[string]float64) float64 {
		var sum float64
		for i := 0; i < 7; i++ {
			sum += metrics[fmt.Sprintf("spectral_contrast_%d", i)]
		}
		return sum / 7
	}

	if averageContrast(sineResult.Metrics) <= averageContrast(noiseResult.Metrics) {
		t.Fatalf("expected sine average contrast > noise average contrast, got sine=%f noise=%f",
			averageContrast(sineResult.Metrics), averageContrast(noiseResult.Metrics))
	}
}

func randomNoiseBuffer(length int, seed int64) []float64 {
	rng := rand.New(rand.NewSource(seed))
	data := make([]float64, length)
	for i := range data {
		data[i] = rng.Float64()*2 - 1
	}
	return data
}
