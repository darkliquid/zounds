package analysis

import (
	"math"
	"testing"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
)

func TestComputePerceptualHashIsStableAcrossSampleRates(t *testing.T) {
	t.Parallel()

	hashA, err := computePerceptualHash(testPCMBuffer(44100, testSineBuffer(44100, 440, 44100)))
	if err != nil {
		t.Fatalf("compute perceptual hash A: %v", err)
	}
	hashB, err := computePerceptualHash(testPCMBuffer(22050, testSineBuffer(22050, 440, 22050)))
	if err != nil {
		t.Fatalf("compute perceptual hash B: %v", err)
	}

	distance, err := HammingDistanceHex(hashA.Value, hashB.Value)
	if err != nil {
		t.Fatalf("hamming distance: %v", err)
	}
	if distance > 8 {
		t.Fatalf("expected similar hashes, got distance %d (%s vs %s)", distance, hashA.Value, hashB.Value)
	}
}

func TestComputePerceptualHashSeparatesDifferentSignals(t *testing.T) {
	t.Parallel()

	hashA, err := computePerceptualHash(testPCMBuffer(44100, testSineBuffer(44100, 110, 44100)))
	if err != nil {
		t.Fatalf("compute perceptual hash A: %v", err)
	}
	hashB, err := computePerceptualHash(testPCMBuffer(44100, testSineBuffer(44100, 1760, 44100)))
	if err != nil {
		t.Fatalf("compute perceptual hash B: %v", err)
	}

	distance, err := HammingDistanceHex(hashA.Value, hashB.Value)
	if err != nil {
		t.Fatalf("hamming distance: %v", err)
	}
	if distance < 2 {
		t.Fatalf("expected distinct hashes, got distance %d (%s vs %s)", distance, hashA.Value, hashB.Value)
	}
}

func TestHammingDistanceHex(t *testing.T) {
	t.Parallel()

	distance, err := HammingDistanceHex("f0", "0f")
	if err != nil {
		t.Fatalf("hamming distance: %v", err)
	}
	if distance != 8 {
		t.Fatalf("expected distance 8, got %d", distance)
	}
}

func testPCMBuffer(sampleRate int, data []float64) zaudio.PCMBuffer {
	return zaudio.PCMBuffer{
		SampleRate: sampleRate,
		Channels:   1,
		BitDepth:   16,
		Data:       data,
	}
}

func testSineBuffer(length int, frequency float64, sampleRate int) []float64 {
	data := make([]float64, length)
	for i := range data {
		data[i] = math.Sin(2 * math.Pi * frequency * float64(i) / float64(sampleRate))
	}
	return data
}
