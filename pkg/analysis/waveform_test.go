package analysis

import (
	"math/rand"
	"testing"
)

func TestClassifyWaveformTonal(t *testing.T) {
	t.Parallel()

	buffer := testPCMBuffer(44100, testSineBuffer(44100, 440, 44100))
	spectral := computeSpectral(buffer)
	dynamics := computeDynamics(buffer)
	pitch := computePitch(buffer)

	classification, texture := classifyWaveform(spectral, dynamics, pitch)
	if classification != "tonal" {
		t.Fatalf("expected tonal classification, got %q", classification)
	}
	if texture != "sustained" {
		t.Fatalf("expected sustained texture, got %q", texture)
	}
}

func TestClassifyWaveformPercussive(t *testing.T) {
	t.Parallel()

	buffer := testPCMBuffer(44100, testImpulseTrainBuffer(44100, 2205))
	spectral := computeSpectral(buffer)
	dynamics := computeDynamics(buffer)
	pitch := computePitch(buffer)

	classification, texture := classifyWaveform(spectral, dynamics, pitch)
	if classification != "percussive" {
		t.Fatalf("expected percussive classification, got %q", classification)
	}
	if texture != "transient" {
		t.Fatalf("expected transient texture, got %q", texture)
	}
}

func TestClassifyWaveformNoise(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(42))
	data := make([]float64, 44100)
	for i := range data {
		data[i] = rng.Float64()*2 - 1
	}

	buffer := testPCMBuffer(44100, data)
	spectral := computeSpectral(buffer)
	dynamics := computeDynamics(buffer)
	pitch := computePitch(buffer)

	classification, texture := classifyWaveform(spectral, dynamics, pitch)
	if classification != "noise" {
		t.Fatalf("expected noise classification, got %q", classification)
	}
	if texture != "noisy" {
		t.Fatalf("expected noisy texture, got %q", texture)
	}
}

func testImpulseTrainBuffer(length int, interval int) []float64 {
	data := make([]float64, length)
	if interval <= 0 {
		return data
	}
	for i := 0; i < length; i += interval {
		data[i] = 1
		if i+1 < length {
			data[i+1] = 0.5
		}
	}
	return data
}
