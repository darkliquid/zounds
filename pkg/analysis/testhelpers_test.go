package analysis_test

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/audio/wav"
)

func writeAnalysisWAV(t *testing.T, path string, buffer zaudio.PCMBuffer) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create wav file: %v", err)
	}
	if err := wav.New().Encode(context.Background(), file, buffer); err != nil {
		t.Fatalf("encode wav file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close wav file: %v", err)
	}
}

func fillBuffer(length int, value float64) []float64 {
	data := make([]float64, length)
	for i := range data {
		data[i] = value
	}
	return data
}

func sineBuffer(length int, frequency float64, sampleRate int) []float64 {
	data := make([]float64, length)
	for i := range data {
		data[i] = math.Sin(2 * math.Pi * frequency * float64(i) / float64(sampleRate))
	}
	return data
}

func impulseTrainBuffer(length int, bpm float64, sampleRate int) []float64 {
	data := make([]float64, length)
	interval := int(math.Round(float64(sampleRate) * 60.0 / bpm))
	if interval <= 0 {
		return data
	}

	for i := 0; i < length; i += interval {
		data[i] = 1.0
		if i+1 < length {
			data[i+1] = 0.5
		}
	}

	return data
}

func mixedSineBuffer(length int, sampleRate int, freqs ...float64) []float64 {
	data := make([]float64, length)
	if len(freqs) == 0 {
		return data
	}

	for _, freq := range freqs {
		for i := range data {
			data[i] += math.Sin(2 * math.Pi * freq * float64(i) / float64(sampleRate))
		}
	}

	scale := 1 / float64(len(freqs))
	for i := range data {
		data[i] *= scale
	}
	return data
}
