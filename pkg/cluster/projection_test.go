package cluster_test

import (
	"math"
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

func TestProjectTSNE2DProducesFiniteCoordinates(t *testing.T) {
	t.Parallel()

	points, err := cluster.ProjectTSNE2D([]core.FeatureVector{
		{SampleID: 1, Values: []float64{0, 0, 0}},
		{SampleID: 2, Values: []float64{5, 0, 0}},
		{SampleID: 3, Values: []float64{0, 5, 0}},
		{SampleID: 4, Values: []float64{5, 5, 0}},
	}, cluster.TSNEOptions{Iterations: 250, Seed: 7})
	if err != nil {
		t.Fatalf("project tsne 2d: %v", err)
	}
	if len(points) != 4 {
		t.Fatalf("expected 4 points, got %d", len(points))
	}
	for _, point := range points {
		if math.IsNaN(point.X) || math.IsNaN(point.Y) || math.IsInf(point.X, 0) || math.IsInf(point.Y, 0) {
			t.Fatalf("unexpected point %+v", point)
		}
	}
}

func TestProjectTSNE2DSmallN(t *testing.T) {
	t.Parallel()

	for _, count := range []int{1, 2, 3, 5} {
		vectors := make([]core.FeatureVector, count)
		for i := 0; i < count; i++ {
			vectors[i] = core.FeatureVector{
				SampleID: int64(i + 1),
				Values:   []float64{float64(i * 10), float64(i * 5)},
			}
		}

		// Use default options where Perplexity defaults to 20, exceeding N-1.
		points, err := cluster.ProjectTSNE2D(vectors, cluster.TSNEOptions{Iterations: 100})
		if err != nil {
			t.Fatalf("N=%d: unexpected error: %v", count, err)
		}
		if len(points) != count {
			t.Fatalf("N=%d: expected %d points, got %d", count, count, len(points))
		}
		for _, p := range points {
			if math.IsNaN(p.X) || math.IsNaN(p.Y) || math.IsInf(p.X, 0) || math.IsInf(p.Y, 0) {
				t.Fatalf("N=%d: non-finite coordinate in point %+v", count, p)
			}
		}
	}
}

func TestProjectTSNE2DLargeDistances(t *testing.T) {
	t.Parallel()

	// Points separated by Euclidean distance > 50 (squared distance > 2500 >> 745)
	vectors := []core.FeatureVector{
		{SampleID: 1, Values: []float64{0, 0}},
		{SampleID: 2, Values: []float64{500, 0}},
		{SampleID: 3, Values: []float64{0, 500}},
		{SampleID: 4, Values: []float64{500, 500}},
	}

	points, err := cluster.ProjectTSNE2D(vectors, cluster.TSNEOptions{Iterations: 200})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != 4 {
		t.Fatalf("expected 4 points, got %d", len(points))
	}
	for _, p := range points {
		if math.IsNaN(p.X) || math.IsNaN(p.Y) || math.IsInf(p.X, 0) || math.IsInf(p.Y, 0) {
			t.Fatalf("non-finite coordinate in point %+v", p)
		}
	}
}
