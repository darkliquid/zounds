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

func TestComputePerceptualHashSpansFullDuration(t *testing.T) {
	t.Parallel()

	// 2 seconds at 44100 Hz = 88200 samples.
	// Both signals start with identical 440 Hz sine for the first 1.2s (> 1.024s).
	// In the second half (1.2s to 2.0s), signal A continues 440 Hz while signal B switches to 1760 Hz.
	sr := 44100
	totalSamples := 2 * sr
	splitSample := int(1.2 * float64(sr))

	dataA := make([]float64, totalSamples)
	dataB := make([]float64, totalSamples)
	for i := 0; i < totalSamples; i++ {
		dataA[i] = math.Sin(2 * math.Pi * 440 * float64(i) / float64(sr))
		if i < splitSample {
			dataB[i] = dataA[i]
		} else {
			dataB[i] = math.Sin(2 * math.Pi * 1760 * float64(i) / float64(sr))
		}
	}

	hashA, err := computePerceptualHash(testPCMBuffer(sr, dataA))
	if err != nil {
		t.Fatalf("hash A: %v", err)
	}
	hashB, err := computePerceptualHash(testPCMBuffer(sr, dataB))
	if err != nil {
		t.Fatalf("hash B: %v", err)
	}

	distance, err := HammingDistanceHex(hashA.Value, hashB.Value)
	if err != nil {
		t.Fatalf("distance: %v", err)
	}
	// Under the old 1.024s slice, hashA and hashB would be identical (distance 0).
	// With full-duration 2D fingerprint, they MUST differ.
	if distance < 4 {
		t.Fatalf("expected full-duration hashes to differ for tail differences, got distance %d (%s vs %s)", distance, hashA.Value, hashB.Value)
	}
}

func TestComputePerceptualHashGainInvariant(t *testing.T) {
	t.Parallel()

	sr := 44100
	data100 := testSineBuffer(sr, 440, sr)
	data20 := make([]float64, len(data100))
	for i, v := range data100 {
		data20[i] = v * 0.2 // 20% amplitude / -14 dB gain
	}

	hash100, err := computePerceptualHash(testPCMBuffer(sr, data100))
	if err != nil {
		t.Fatalf("hash 100: %v", err)
	}
	hash20, err := computePerceptualHash(testPCMBuffer(sr, data20))
	if err != nil {
		t.Fatalf("hash 20: %v", err)
	}

	distance, err := HammingDistanceHex(hash100.Value, hash20.Value)
	if err != nil {
		t.Fatalf("distance: %v", err)
	}
	if distance != 0 {
		t.Fatalf("expected gain-invariant hash, got distance %d (%s vs %s)", distance, hash100.Value, hash20.Value)
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
