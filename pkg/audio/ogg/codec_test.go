package ogg

import (
	"math"
	"testing"
)

func TestClampDecodedSamplesClampsOvershootAndRejectsNaN(t *testing.T) {
	t.Parallel()

	data, err := clampVorbisSamples([]float32{-1.04, -0.5, 0.0, 0.5, 1.003})
	if err != nil {
		t.Fatalf("clampVorbisSamples: %v", err)
	}
	want := []float64{-1.0, -0.5, 0.0, 0.5, 1.0}
	for i := range want {
		if math.Abs(data[i]-want[i]) > 1e-6 {
			t.Fatalf("data[%d] = %v, want %v", i, data[i], want[i])
		}
	}

	if _, err := clampVorbisSamples([]float32{0.1, float32(math.NaN())}); err == nil {
		t.Fatal("expected error when Vorbis stream produces NaN")
	}
}
