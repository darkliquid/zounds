package cluster_test

import (
	"context"
	"testing"

	"github.com/darkliquid/zounds/pkg/cluster"
	"github.com/darkliquid/zounds/pkg/core"
)

func TestKMeansFitGroupsSeparatedVectors(t *testing.T) {
	t.Parallel()

	vectors := []core.FeatureVector{
		{SampleID: 1, Values: []float64{0, 0}},
		{SampleID: 2, Values: []float64{0.1, 0.1}},
		{SampleID: 3, Values: []float64{10, 10}},
		{SampleID: 4, Values: []float64{10.1, 10.2}},
	}

	model := cluster.NewKMeans(cluster.KMeansOptions{K: 2, MaxIterations: 20})
	result, err := model.Fit(vectors)
	if err != nil {
		t.Fatalf("fit kmeans: %v", err)
	}

	if len(result.Clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(result.Clusters))
	}

	assignments := make(map[int64]int, len(result.Memberships))
	for _, membership := range result.Memberships {
		assignments[membership.SampleID] = membership.ClusterIndex
	}

	if assignments[1] != assignments[2] {
		t.Fatalf("expected samples 1 and 2 together, got %v", assignments)
	}
	if assignments[3] != assignments[4] {
		t.Fatalf("expected samples 3 and 4 together, got %v", assignments)
	}
	if assignments[1] == assignments[3] {
		t.Fatalf("expected distant groups to be split, got %v", assignments)
	}
}

func TestKMeansImplementsClusterer(t *testing.T) {
	t.Parallel()

	vectors := []core.FeatureVector{
		{SampleID: 1, Values: []float64{0, 0}},
		{SampleID: 2, Values: []float64{5, 5}},
	}

	model := cluster.NewKMeans(cluster.KMeansOptions{K: 2})
	clusters, err := model.Cluster(context.Background(), vectors)
	if err != nil {
		t.Fatalf("cluster: %v", err)
	}
	if len(clusters) != 2 {
		t.Fatalf("expected 2 cluster records, got %d", len(clusters))
	}
	for _, item := range clusters {
		if item.Method != "kmeans" {
			t.Fatalf("unexpected method: %+v", item)
		}
	}
}

func TestKMeansRejectsInvalidK(t *testing.T) {
	t.Parallel()

	model := cluster.NewKMeans(cluster.KMeansOptions{K: 0})
	if _, err := model.Fit([]core.FeatureVector{{SampleID: 1, Values: []float64{1}}}); err == nil {
		t.Fatal("expected invalid K error")
	}
}
