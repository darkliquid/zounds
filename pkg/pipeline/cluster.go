package pipeline

import (
	"context"
	"fmt"

	"github.com/darkliquid/zounds/pkg/cluster"
	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/db"
)

// ClusterOptions controls a clustering run.
type ClusterOptions struct {
	// Namespace selects the feature-vector namespace to cluster over
	// (default "acoustic").
	Namespace string
	// Metric selects the metric space: "acoustic", "clap", "timbre", "harmonic",
	// "rhythm", "perceptual", "hybrid".
	Metric string
	// Method is the clustering algorithm; only "kmeans" is supported.
	Method string
	// K is the cluster count for kmeans.
	K int
	// Projection annotates clusters with centroid coordinates using "pca" or
	// "tsne" (default "pca").
	Projection string
	// DryRun runs clustering without persisting results.
	DryRun bool
}

// Cluster runs KMeans over stored feature vectors, annotates cluster
// centroids with projection coordinates, and persists the result unless
// DryRun is set.
func Cluster(ctx context.Context, repo *db.Repository, opts ClusterOptions) (cluster.KMeansResult, error) {
	if repo == nil {
		return cluster.KMeansResult{}, fmt.Errorf("pipeline: nil repository")
	}

	method := opts.Method
	if method == "" {
		method = "kmeans"
	}
	if method != "kmeans" {
		return cluster.KMeansResult{}, fmt.Errorf("unsupported clustering method %q", method)
	}
	projection := opts.Projection
	if projection == "" {
		projection = "pca"
	}

	vectors, err := LoadMetricVectors(ctx, repo, opts.Metric, opts.Namespace)
	if err != nil {
		return cluster.KMeansResult{}, err
	}

	model := cluster.NewKMeans(cluster.KMeansOptions{K: opts.K})
	result, err := model.Fit(vectors)
	if err != nil {
		return cluster.KMeansResult{}, err
	}
	if err := annotateClusterProjection(result.Clusters, result.Memberships, vectors, projection); err != nil {
		return cluster.KMeansResult{}, err
	}

	if opts.DryRun {
		return result, nil
	}

	if err := persistKMeansResult(ctx, repo, result); err != nil {
		return cluster.KMeansResult{}, err
	}

	return result, nil
}

func annotateClusterProjection(clusters []core.Cluster, memberships []cluster.Membership, vectors []core.FeatureVector, method string) error {
	points, err := cluster.Project2DByMethod(vectors, method)
	if err != nil {
		return err
	}
	pointBySample := make(map[int64]cluster.ProjectionPoint, len(points))
	for _, point := range points {
		pointBySample[point.SampleID] = point
	}

	type accum struct {
		x, y  float64
		count float64
	}
	accums := make([]accum, len(clusters))
	for _, membership := range memberships {
		point, ok := pointBySample[membership.SampleID]
		if !ok || membership.ClusterIndex < 0 || membership.ClusterIndex >= len(clusters) {
			continue
		}
		accums[membership.ClusterIndex].x += point.X
		accums[membership.ClusterIndex].y += point.Y
		accums[membership.ClusterIndex].count++
	}

	for i := range clusters {
		if clusters[i].Parameters == nil {
			clusters[i].Parameters = map[string]float64{}
		}
		if accums[i].count > 0 {
			clusters[i].Parameters["projection_x"] = accums[i].x / accums[i].count
			clusters[i].Parameters["projection_y"] = accums[i].y / accums[i].count
		}
	}
	return nil
}

func persistKMeansResult(ctx context.Context, repo *db.Repository, result cluster.KMeansResult) error {
	if err := repo.DeleteClustersByMethod(ctx, "kmeans"); err != nil {
		return err
	}

	clusterIDs := make([]int64, len(result.Clusters))
	for i, item := range result.Clusters {
		clusterID, err := repo.InsertCluster(ctx, item)
		if err != nil {
			return err
		}
		clusterIDs[i] = clusterID
	}

	for _, membership := range result.Memberships {
		score := 1 / (1 + membership.Distance)
		if err := repo.InsertClusterMember(ctx, db.ClusterMember{
			ClusterID: clusterIDs[membership.ClusterIndex],
			SampleID:  membership.SampleID,
			Score:     score,
		}); err != nil {
			return err
		}
	}

	return nil
}
