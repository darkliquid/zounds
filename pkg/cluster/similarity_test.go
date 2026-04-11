package cluster_test

import (
	"math"
	"testing"

	"github.com/darkliquid/zounds/pkg/cluster"
	"github.com/darkliquid/zounds/pkg/core"
)

func TestCosineSimilarity(t *testing.T) {
	t.Parallel()

	similarity, err := cluster.CosineSimilarity([]float64{1, 0}, []float64{0, 1})
	if err != nil {
		t.Fatalf("cosine similarity: %v", err)
	}
	if similarity != 0 {
		t.Fatalf("expected orthogonal cosine similarity 0, got %f", similarity)
	}
}

func TestEuclideanDistance(t *testing.T) {
	t.Parallel()

	distance, err := cluster.EuclideanDistance([]float64{0, 0}, []float64{3, 4})
	if err != nil {
		t.Fatalf("euclidean distance: %v", err)
	}
	if distance != 5 {
		t.Fatalf("expected distance 5, got %f", distance)
	}
}

func TestPairwiseSortsMostSimilarPairsFirst(t *testing.T) {
	t.Parallel()

	vectors := []core.FeatureVector{
		{SampleID: 1, Values: []float64{1, 0}},
		{SampleID: 2, Values: []float64{0.9, 0.1}},
		{SampleID: 3, Values: []float64{0, 1}},
	}

	pairs, err := cluster.Pairwise(vectors, cluster.MetricCosine)
	if err != nil {
		t.Fatalf("pairwise: %v", err)
	}
	if len(pairs) != 3 {
		t.Fatalf("expected 3 pairs, got %d", len(pairs))
	}
	if pairs[0].LeftSampleID != 1 || pairs[0].RightSampleID != 2 {
		t.Fatalf("expected most similar pair to be 1-2, got %+v", pairs[0])
	}
}

func TestNearestNeighborsReturnsClosestMatches(t *testing.T) {
	t.Parallel()

	target := core.FeatureVector{SampleID: 1, Values: []float64{1, 1}}
	candidates := []core.FeatureVector{
		{SampleID: 1, Values: []float64{1, 1}},
		{SampleID: 2, Values: []float64{0.9, 1.1}},
		{SampleID: 3, Values: []float64{-1, -1}},
		{SampleID: 4, Values: []float64{1.1, 0.95}},
	}

	neighbors, err := cluster.NearestNeighbors(target, candidates, 2, cluster.MetricEuclidean)
	if err != nil {
		t.Fatalf("nearest neighbors: %v", err)
	}
	if len(neighbors) != 2 {
		t.Fatalf("expected 2 neighbors, got %d", len(neighbors))
	}
	if neighbors[0].Vector.SampleID != 4 || neighbors[1].Vector.SampleID != 2 {
		t.Fatalf("unexpected neighbor order: %+v", neighbors)
	}
	if math.Abs(neighbors[0].Distance-0.1118033989) > 0.0001 {
		t.Fatalf("unexpected first neighbor distance: %f", neighbors[0].Distance)
	}
}
