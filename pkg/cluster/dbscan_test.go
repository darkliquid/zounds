package cluster_test

import (
	"context"
	"testing"

	"github.com/darkliquid/zounds/pkg/cluster"
	"github.com/darkliquid/zounds/pkg/core"
)

func TestDBSCANFitFindsDenseGroupsAndNoise(t *testing.T) {
	t.Parallel()

	vectors := []core.FeatureVector{
		{SampleID: 1, Values: []float64{0, 0}},
		{SampleID: 2, Values: []float64{0.1, 0.1}},
		{SampleID: 3, Values: []float64{5, 5}},
		{SampleID: 4, Values: []float64{5.1, 5.1}},
		{SampleID: 5, Values: []float64{20, 20}},
	}

	model := cluster.NewDBSCAN(cluster.DBSCANOptions{Epsilon: 0.3, MinPoints: 2})
	result, err := model.Fit(vectors)
	if err != nil {
		t.Fatalf("fit dbscan: %v", err)
	}

	if len(result.Clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(result.Clusters))
	}
	if len(result.Noise) != 1 || result.Noise[0].SampleID != 5 {
		t.Fatalf("expected sample 5 as noise, got %+v", result.Noise)
	}
}

func TestDBSCANImplementsClusterer(t *testing.T) {
	t.Parallel()

	vectors := []core.FeatureVector{
		{SampleID: 1, Values: []float64{0, 0}},
		{SampleID: 2, Values: []float64{0.1, 0.1}},
	}

	model := cluster.NewDBSCAN(cluster.DBSCANOptions{Epsilon: 0.5, MinPoints: 2})
	clusters, err := model.Cluster(context.Background(), vectors)
	if err != nil {
		t.Fatalf("cluster dbscan: %v", err)
	}
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}
	if clusters[0].Method != "dbscan" {
		t.Fatalf("unexpected method: %+v", clusters[0])
	}
}
