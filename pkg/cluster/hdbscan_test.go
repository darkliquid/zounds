package cluster_test

import (
	"context"
	"testing"

	"github.com/darkliquid/zounds/pkg/cluster"
	"github.com/darkliquid/zounds/pkg/core"
)

func TestHDBSCANFitFindsDenseGroupsAndNoise(t *testing.T) {
	t.Parallel()

	vectors := []core.FeatureVector{
		// Group 1: 3 dense points around (0, 0)
		{SampleID: 1, Values: []float64{0, 0}},
		{SampleID: 2, Values: []float64{0.05, 0.05}},
		{SampleID: 3, Values: []float64{-0.05, -0.05}},
		// Group 2: 3 dense points around (10, 10)
		{SampleID: 4, Values: []float64{10, 10}},
		{SampleID: 5, Values: []float64{10.05, 10.05}},
		{SampleID: 6, Values: []float64{9.95, 9.95}},
		// Noise: outlier at (100, 100)
		{SampleID: 7, Values: []float64{100, 100}},
	}

	model := cluster.NewHDBSCAN(cluster.HDBSCANOptions{MinClusterSize: 3, MinSamples: 2})
	result, err := model.Fit(vectors)
	if err != nil {
		t.Fatalf("fit hdbscan: %v", err)
	}

	if len(result.Clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(result.Clusters))
	}

	noiseFound := false
	for _, n := range result.Noise {
		if n.SampleID == 7 {
			noiseFound = true
			break
		}
	}
	if !noiseFound {
		t.Fatalf("expected sample 7 as noise, got noise: %+v", result.Noise)
	}
}

func TestHDBSCANImplementsClusterer(t *testing.T) {
	t.Parallel()

	vectors := []core.FeatureVector{
		{SampleID: 1, Values: []float64{0, 0}},
		{SampleID: 2, Values: []float64{0.1, 0.1}},
		{SampleID: 3, Values: []float64{-0.1, -0.1}},
	}

	var clusterer core.Clusterer = cluster.NewHDBSCAN(cluster.HDBSCANOptions{MinClusterSize: 2})
	if clusterer.Name() != "hdbscan" {
		t.Fatalf("expected name 'hdbscan', got %q", clusterer.Name())
	}

	clusters, err := clusterer.Cluster(context.Background(), vectors)
	if err != nil {
		t.Fatalf("cluster hdbscan: %v", err)
	}
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}
	if clusters[0].Method != "hdbscan" {
		t.Fatalf("unexpected method: %+v", clusters[0])
	}
}
