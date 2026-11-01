package cluster

import (
	"math"
	"testing"
)

func TestPerplexityRowLargeDistanceUnderflow(t *testing.T) {
	t.Parallel()

	// Squared distances > 745 cause math.Exp(-distance * 1.0) to underflow to 0.0.
	distances := []float64{0, 1000, 1050, 1100}
	row := perplexityRow(distances, 0, 2.0)

	if row[0] != 0 {
		t.Errorf("expected self probability to be 0, got %f", row[0])
	}

	var sum float64
	for _, p := range row {
		if math.IsNaN(p) || math.IsInf(p, 0) {
			t.Fatalf("unexpected non-finite probability: %f", p)
		}
		sum += p
	}

	if math.Abs(sum-1.0) > 1e-5 {
		t.Fatalf("expected probabilities to sum to 1.0, got %f", sum)
	}

	if row[1] <= row[2] || row[2] <= row[3] {
		t.Fatalf("expected monotonic decrease in probability with distance: %+v", row)
	}
}

func TestPerplexityRowSmallNClamping(t *testing.T) {
	t.Parallel()

	// N=2 with requested perplexity=20 (must be clamped to < N-1 = 1)
	row2 := perplexityRow([]float64{0, 10}, 0, 20)
	if row2[0] != 0 || math.Abs(row2[1]-1.0) > 1e-5 {
		t.Fatalf("expected [0, 1] for N=2, got %+v", row2)
	}

	// N=3 with requested perplexity=20 (must be clamped to < N-1 = 2)
	row3 := perplexityRow([]float64{0, 5, 10}, 0, 20)
	if row3[0] != 0 {
		t.Errorf("expected self prob 0, got %f", row3[0])
	}
	var sum3 float64
	for _, p := range row3 {
		sum3 += p
	}
	if math.Abs(sum3-1.0) > 1e-5 {
		t.Fatalf("expected probabilities to sum to 1.0, got %f", sum3)
	}
	if row3[1] <= row3[2] {
		t.Fatalf("expected closer point to have higher probability: %+v", row3)
	}
}

func TestPerplexityRowAstronomicDistanceFallback(t *testing.T) {
	t.Parallel()

	distances := []float64{0, 1e20, 2e20}
	row := perplexityRow(distances, 0, 1.5)

	var sum float64
	for _, p := range row {
		if math.IsNaN(p) || math.IsInf(p, 0) {
			t.Fatalf("unexpected non-finite probability: %f", p)
		}
		sum += p
	}
	if math.Abs(sum-1.0) > 1e-5 {
		t.Fatalf("expected probabilities to sum to 1.0, got %f", sum)
	}
}
