package cluster_test

import (
	"math"
	"testing"

	"github.com/darkliquid/zounds/pkg/cluster"
	"github.com/darkliquid/zounds/pkg/core"
)

func TestStandardizeVectorsZScore(t *testing.T) {
	t.Parallel()

	vectors := []core.FeatureVector{
		{SampleID: 1, Dimensions: 2, Values: []float64{10.0, 100.0}},
		{SampleID: 2, Dimensions: 2, Values: []float64{20.0, 200.0}},
		{SampleID: 3, Dimensions: 2, Values: []float64{30.0, 300.0}},
	}

	std := cluster.StandardizeVectors(vectors, cluster.StandardizeZScore)
	if len(std) != 3 {
		t.Fatalf("expected 3 vectors, got %d", len(std))
	}

	// For [10, 20, 30], mean is 20, std is sqrt((100+0+100)/3) = sqrt(200/3) = 8.1649658
	// Sample 2 (value 20.0) should have standardized value 0.0
	if math.Abs(std[1].Values[0]) > 1e-6 {
		t.Errorf("expected centered value near 0, got %f", std[1].Values[0])
	}
	if math.Abs(std[1].Values[1]) > 1e-6 {
		t.Errorf("expected centered value near 0, got %f", std[1].Values[1])
	}
	// Sample 1 and 3 should be opposites
	if math.Abs(std[0].Values[0]+std[2].Values[0]) > 1e-6 {
		t.Errorf("expected symmetric z-scores, got %f and %f", std[0].Values[0], std[2].Values[0])
	}
}

func TestStandardizeVectorsIQR(t *testing.T) {
	t.Parallel()

	vectors := []core.FeatureVector{
		{SampleID: 1, Dimensions: 1, Values: []float64{1.0}},
		{SampleID: 2, Dimensions: 1, Values: []float64{2.0}},
		{SampleID: 3, Dimensions: 1, Values: []float64{3.0}},
		{SampleID: 4, Dimensions: 1, Values: []float64{4.0}},
		{SampleID: 5, Dimensions: 1, Values: []float64{100.0}}, // extreme outlier
	}

	std := cluster.StandardizeVectors(vectors, cluster.StandardizeIQR)
	if len(std) != 5 {
		t.Fatalf("expected 5 vectors, got %d", len(std))
	}

	// Median is 3.0, so sample 3 should have standardized value 0.0
	if math.Abs(std[2].Values[0]) > 1e-6 {
		t.Errorf("expected median to be centered at 0, got %f", std[2].Values[0])
	}
}

func TestStandardizeVectorsConstantDimension(t *testing.T) {
	t.Parallel()

	vectors := []core.FeatureVector{
		{SampleID: 1, Dimensions: 2, Values: []float64{5.0, 1.0}},
		{SampleID: 2, Dimensions: 2, Values: []float64{5.0, 2.0}},
		{SampleID: 3, Dimensions: 2, Values: []float64{5.0, 3.0}},
	}

	std := cluster.StandardizeVectors(vectors, cluster.StandardizeIQR)
	for i := range std {
		if std[i].Values[0] != 0.0 {
			t.Errorf("expected constant dimension to be 0, got %f", std[i].Values[0])
		}
	}
}
