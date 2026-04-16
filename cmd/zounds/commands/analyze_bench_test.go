package commands

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/zounds/pkg/analysis"
	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/audio/wav"
	"github.com/darkliquid/zounds/pkg/core"
)

func BenchmarkAnalyzeSampleEndToEnd(b *testing.B) {
	path := filepath.Join(b.TempDir(), "bench.wav")
	buffer := benchmarkAudioBuffer(44100, 440, 880)
	writeBenchmarkWAV(b, path, buffer)
	sample := core.Sample{
		ID:        1,
		Path:      path,
		Extension: "wav",
		Format:    core.FormatWAV,
		SizeBytes: int64(len(buffer.Data) * 2),
	}
	builder := analysis.NewFeatureVectorBuilder(nil)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := analyzeSample(ctx, sample, builder); err != nil {
			b.Fatalf("analyzeSample returned error: %v", err)
		}
	}
}

func benchmarkAudioBuffer(sampleRate int, frequencies ...int) zaudio.PCMBuffer {
	total := sampleRate
	data := make([]float64, total*2)
	for i := 0; i < total; i++ {
		var value float64
		for _, frequency := range frequencies {
			value += 0.25 * math.Sin(2*math.Pi*float64(frequency)*float64(i)/float64(sampleRate))
		}
		data[i*2] = value
		data[i*2+1] = value
	}
	return zaudio.PCMBuffer{
		SampleRate: sampleRate,
		Channels:   2,
		BitDepth:   16,
		Data:       data,
	}
}

func writeBenchmarkWAV(tb testing.TB, path string, buffer zaudio.PCMBuffer) {
	tb.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	file, err := os.Create(path)
	if err != nil {
		tb.Fatalf("create wav fixture: %v", err)
	}
	if err := wav.New().Encode(context.Background(), file, buffer); err != nil {
		_ = file.Close()
		tb.Fatalf("encode wav fixture: %v", err)
	}
	if err := file.Close(); err != nil {
		tb.Fatalf("close wav fixture: %v", err)
	}
}
