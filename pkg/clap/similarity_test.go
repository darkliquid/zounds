package clap

import (
	"math"
	"testing"
)

func TestL2Normalize(t *testing.T) {
	tests := []struct {
		name  string
		input []float32
		want  float32 // expected norm after normalisation
	}{
		{"unit vector", []float32{1, 0, 0}, 1},
		{"scaled vector", []float32{3, 4, 0}, 1},
		{"all zeros", []float32{0, 0, 0}, 0},
		{"uniform", []float32{1, 1}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := append([]float32(nil), tt.input...)
			l2Normalize(v)
			var norm float64
			for _, x := range v {
				norm += float64(x) * float64(x)
			}
			norm = math.Sqrt(norm)
			if math.Abs(norm-float64(tt.want)) > 1e-5 {
				t.Errorf("after normalisation: norm = %v, want %v", norm, tt.want)
			}
		})
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []float32
		wantAppr float32
	}{
		{"identical", []float32{1, 0, 0}, []float32{1, 0, 0}, 1},
		{"orthogonal", []float32{1, 0}, []float32{0, 1}, 0},
		{"opposite", []float32{1, 0}, []float32{-1, 0}, -1},
		{"angle 60", []float32{1, 0}, []float32{0.5, 0.866}, 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cosineSimilarity(tt.a, tt.b)
			if math.Abs(float64(got-tt.wantAppr)) > 0.01 {
				t.Errorf("cosineSimilarity = %v, want ~%v", got, tt.wantAppr)
			}
		})
	}

	// Empty / mismatched
	if cosineSimilarity([]float32{1}, []float32{1, 2}) != 0 {
		t.Error("mismatched lengths should return 0")
	}
	if cosineSimilarity(nil, nil) != 0 {
		t.Error("nil inputs should return 0")
	}
}
