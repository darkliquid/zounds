package analysis

import (
	"math"
	"testing"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
)

func BenchmarkFeatureVectorBuilderBuild(b *testing.B) {
	builder := NewFeatureVectorBuilder(nil)
	results := []core.AnalysisResult{
		{Metrics: map[string]float64{
			"sample_rate":          44100,
			"channels":             2,
			"duration_seconds":     1.25,
			"spectral_centroid_hz": 1250,
			"spectral_rolloff_hz":  4100,
			"spectral_flux":        0.14,
			"zero_crossing_rate":   0.08,
			"dominant_frequency_hz": 440,
			"peak":                  0.98,
			"rms":                   0.35,
			"tempo_bpm":             128,
			"frequency_hz":          440,
			"confidence":            0.92,
		}},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := builder.Build(1, results...); err != nil {
			b.Fatalf("Build returned error: %v", err)
		}
	}
}

func BenchmarkComputePerceptualHash(b *testing.B) {
	buffer := zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   2,
		BitDepth:   16,
		Data:       benchmarkSine(44100, 440),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := computePerceptualHash(buffer); err != nil {
			b.Fatalf("computePerceptualHash returned error: %v", err)
		}
	}
}

func BenchmarkAnalysisHotPathPipeline(b *testing.B) {
	buffer := benchmarkPCMBuffer(44100, 440, 880)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = computeSpectral(buffer)
		_ = computeBeatStats(buffer)
		_ = computeKey(buffer)
		_ = computeMFCC(buffer, 13, 26)
		_ = computeHPSSStats(buffer)
	}
}

func BenchmarkComputeSpectral(b *testing.B) {
	buffer := benchmarkPCMBuffer(44100, 440, 880)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = computeSpectral(buffer)
	}
}

func BenchmarkComputeBeatStats(b *testing.B) {
	buffer := benchmarkPCMBuffer(44100, 440, 880)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = computeBeatStats(buffer)
	}
}

func BenchmarkComputeKey(b *testing.B) {
	buffer := benchmarkPCMBuffer(44100, 440, 880)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = computeKey(buffer)
	}
}

func BenchmarkComputeMFCC(b *testing.B) {
	buffer := benchmarkPCMBuffer(44100, 440, 880)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = computeMFCC(buffer, 13, 26)
	}
}

func BenchmarkComputeHPSSStats(b *testing.B) {
	buffer := benchmarkPCMBuffer(44100, 440, 880)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = computeHPSSStats(buffer)
	}
}

func benchmarkPCMBuffer(sampleRate int, frequencies ...int) zaudio.PCMBuffer {
	total := sampleRate
	out := make([]float64, total*2)
	for i := 0; i < total; i++ {
		var value float64
		for _, frequency := range frequencies {
			value += 0.25 * math.Sin(2*math.Pi*float64(frequency)*float64(i)/float64(sampleRate))
		}
		out[i*2] = value
		out[i*2+1] = value
	}
	return zaudio.PCMBuffer{
		SampleRate: sampleRate,
		Channels:   2,
		BitDepth:   16,
		Data:       out,
	}
}

func benchmarkSine(sampleRate, frequency int) []float64 {
	total := sampleRate
	out := make([]float64, total*2)
	for i := 0; i < total; i++ {
		value := 0.4 * math.Sin(2*math.Pi*float64(frequency)*float64(i)/float64(sampleRate))
		out[i*2] = value
		out[i*2+1] = value
	}
	return out
}
