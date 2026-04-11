package cluster_test

import (
	"testing"

	"github.com/darkliquid/zounds/pkg/cluster"
	"github.com/darkliquid/zounds/pkg/core"
)

func TestProject2DSeparatesDistinctDirections(t *testing.T) {
	t.Parallel()

	points, err := cluster.Project2D([]core.FeatureVector{
		{SampleID: 1, Values: []float64{0, 0, 0}},
		{SampleID: 2, Values: []float64{5, 0, 0}},
		{SampleID: 3, Values: []float64{0, 5, 0}},
	})
	if err != nil {
		t.Fatalf("project 2d: %v", err)
	}
	if len(points) != 3 {
		t.Fatalf("expected 3 points, got %d", len(points))
	}

	if points[0].X == points[1].X && points[0].Y == points[1].Y {
		t.Fatalf("expected sample 1 and 2 to separate, got %+v", points)
	}
	if points[1].X == points[2].X && points[1].Y == points[2].Y {
		t.Fatalf("expected sample 2 and 3 to separate, got %+v", points)
	}
}

func TestProject2DRejectsDimensionMismatch(t *testing.T) {
	t.Parallel()

	_, err := cluster.Project2D([]core.FeatureVector{
		{SampleID: 1, Values: []float64{1, 2}},
		{SampleID: 2, Values: []float64{1}},
	})
	if err == nil {
		t.Fatal("expected dimension mismatch error")
	}
}
