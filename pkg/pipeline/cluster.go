package pipeline

import (
	"context"
	"fmt"
	"sort"
	"strings"

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
	// Method is the clustering algorithm: "kmeans", "dbscan", or "hdbscan".
	Method string
	// K is the cluster count for kmeans.
	K int
	// Epsilon is the distance threshold for dbscan.
	Epsilon float64
	// MinPoints is the minimum neighbor count for dbscan or min cluster size for hdbscan.
	MinPoints int
	// MinClusterSize is the minimum cluster size for hdbscan.
	MinClusterSize int
	// MinSamples is the core distance sample count for hdbscan.
	MinSamples int
	// Projection annotates clusters with centroid coordinates using "pca" or
	// "tsne" (default "pca").
	Projection string
	// DryRun runs clustering without persisting results.
	DryRun bool
}

// ClusterResult holds the outcome of a clustering run.
type ClusterResult struct {
	Method      string
	Clusters    []core.Cluster
	Memberships []cluster.Membership
	Centroids   [][]float64
	Noise       []core.FeatureVector
}

// Cluster runs clustering (kmeans, dbscan, or hdbscan) over stored feature vectors,
// annotates cluster centroids with projection coordinates, auto-labels clusters
// from dominant member tags, and persists the result unless DryRun is set.
func Cluster(ctx context.Context, repo *db.Repository, opts ClusterOptions) (ClusterResult, error) {
	if repo == nil {
		return ClusterResult{}, fmt.Errorf("pipeline: nil repository")
	}

	method := strings.ToLower(strings.TrimSpace(opts.Method))
	if method == "" {
		method = "kmeans"
	}
	projection := opts.Projection
	if projection == "" {
		projection = "pca"
	}

	vectors, err := LoadMetricVectors(ctx, repo, opts.Metric, opts.Namespace)
	if err != nil {
		return ClusterResult{}, err
	}

	var (
		clusters    []core.Cluster
		memberships []cluster.Membership
		centroids   [][]float64
		noise       []core.FeatureVector
	)

	switch method {
	case "kmeans":
		k := opts.K
		if k <= 0 {
			k = 8
		}
		model := cluster.NewKMeans(cluster.KMeansOptions{K: k})
		res, err := model.Fit(vectors)
		if err != nil {
			return ClusterResult{}, err
		}
		clusters = res.Clusters
		memberships = res.Memberships
		centroids = res.Centroids

	case "dbscan":
		eps := opts.Epsilon
		if eps <= 0 {
			eps = 0.5
		}
		minPts := opts.MinPoints
		if minPts <= 0 && opts.MinClusterSize > 0 {
			minPts = opts.MinClusterSize
		}
		if minPts <= 0 {
			minPts = 2
		}
		model := cluster.NewDBSCAN(cluster.DBSCANOptions{Epsilon: eps, MinPoints: minPts})
		res, err := model.Fit(vectors)
		if err != nil {
			return ClusterResult{}, err
		}
		clusters = res.Clusters
		memberships = res.Memberships
		noise = res.Noise

	case "hdbscan":
		minClusterSize := opts.MinClusterSize
		if minClusterSize <= 0 && opts.MinPoints > 0 {
			minClusterSize = opts.MinPoints
		}
		if minClusterSize <= 0 {
			minClusterSize = 5
		}
		minSamples := opts.MinSamples
		if minSamples <= 0 {
			minSamples = minClusterSize
		}
		model := cluster.NewHDBSCAN(cluster.HDBSCANOptions{
			MinClusterSize: minClusterSize,
			MinSamples:     minSamples,
		})
		res, err := model.Fit(vectors)
		if err != nil {
			return ClusterResult{}, err
		}
		clusters = res.Clusters
		memberships = res.Memberships
		noise = res.Noise

	default:
		return ClusterResult{}, fmt.Errorf("unsupported clustering method %q", method)
	}

	if err := annotateClusterProjection(clusters, memberships, vectors, projection); err != nil {
		return ClusterResult{}, err
	}

	if err := autoLabelClusters(ctx, repo, clusters, memberships); err != nil {
		return ClusterResult{}, err
	}

	result := ClusterResult{
		Method:      method,
		Clusters:    clusters,
		Memberships: memberships,
		Centroids:   centroids,
		Noise:       noise,
	}

	if opts.DryRun {
		return result, nil
	}

	if err := persistClusterResult(ctx, repo, method, clusters, memberships); err != nil {
		return ClusterResult{}, err
	}

	return result, nil
}

func autoLabelClusters(ctx context.Context, repo *db.Repository, clusters []core.Cluster, memberships []cluster.Membership) error {
	if repo == nil || len(clusters) == 0 || len(memberships) == 0 {
		return nil
	}

	clusterSamples := make(map[int][]int64, len(clusters))
	uniqueSampleIDs := make(map[int64]bool, len(memberships))
	for _, m := range memberships {
		if m.ClusterIndex >= 0 && m.ClusterIndex < len(clusters) {
			clusterSamples[m.ClusterIndex] = append(clusterSamples[m.ClusterIndex], m.SampleID)
			uniqueSampleIDs[m.SampleID] = true
		}
	}

	sampleIDs := make([]int64, 0, len(uniqueSampleIDs))
	for id := range uniqueSampleIDs {
		sampleIDs = append(sampleIDs, id)
	}

	sampleTags, err := repo.ListTagsForSampleIDs(ctx, sampleIDs)
	if err != nil {
		return err
	}

	type tagStat struct {
		name       string
		count      int
		confidence float64
	}

	for i := range clusters {
		members := clusterSamples[i]
		if len(members) == 0 {
			continue
		}

		counts := make(map[string]*tagStat)
		for _, sid := range members {
			tags := sampleTags[sid]
			seenOnSample := make(map[string]bool)
			for _, t := range tags {
				normalized := strings.TrimSpace(t.NormalizedName)
				if normalized == "" {
					normalized = strings.TrimSpace(t.Name)
				}
				if normalized == "" || seenOnSample[normalized] {
					continue
				}
				seenOnSample[normalized] = true
				st, ok := counts[normalized]
				if !ok {
					st = &tagStat{name: normalized}
					counts[normalized] = st
				}
				st.count++
				st.confidence += t.Confidence
			}
		}

		if len(counts) == 0 {
			continue
		}

		stats := make([]*tagStat, 0, len(counts))
		for _, s := range counts {
			stats = append(stats, s)
		}
		sort.Slice(stats, func(a, b int) bool {
			if stats[a].count != stats[b].count {
				return stats[a].count > stats[b].count
			}
			if stats[a].confidence != stats[b].confidence {
				return stats[a].confidence > stats[b].confidence
			}
			return stats[a].name < stats[b].name
		})

		top := stats[0]
		label := fmt.Sprintf("cluster-%d: %s", i+1, top.name)
		if len(stats) > 1 && stats[1].count >= len(members)/2 && stats[1].count > 1 {
			label = fmt.Sprintf("cluster-%d: %s, %s", i+1, top.name, stats[1].name)
		}
		clusters[i].Label = label
	}

	return nil
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

func persistClusterResult(ctx context.Context, repo *db.Repository, method string, clusters []core.Cluster, memberships []cluster.Membership) error {
	if err := repo.DeleteClustersByMethod(ctx, method); err != nil {
		return err
	}

	clusterIDs := make([]int64, len(clusters))
	for i, item := range clusters {
		clusterID, err := repo.InsertCluster(ctx, item)
		if err != nil {
			return err
		}
		clusterIDs[i] = clusterID
	}

	for _, membership := range memberships {
		if membership.ClusterIndex < 0 || membership.ClusterIndex >= len(clusterIDs) {
			continue
		}
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
