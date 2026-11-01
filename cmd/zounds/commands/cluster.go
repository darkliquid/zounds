package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/darkliquid/zounds/pkg/pipeline"
)

func newClusterCommand(cfg *Config) *cobra.Command {
	var (
		method         string
		k              int
		epsilon        float64
		minPoints      int
		minClusterSize int
		projection     string
		metric         string
	)

	cmd := &cobra.Command{
		Use:   "cluster",
		Short: "Cluster related sounds from stored feature vectors",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			repo, closer, err := openRepository(ctx, cfg)
			if err != nil {
				return err
			}
			defer func() { _ = closer.Close() }()

			result, err := pipeline.Cluster(ctx, repo, pipeline.ClusterOptions{
				Metric:         metric,
				Method:         method,
				K:              k,
				Epsilon:        epsilon,
				MinPoints:      minPoints,
				MinClusterSize: minClusterSize,
				Projection:     projection,
				DryRun:         cfg.DryRun,
			})
			if err != nil {
				return err
			}

			return printClusterSummary(cmd, result)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&metric, "metric", "", "metric space: acoustic, clap, timbre, harmonic, rhythm, perceptual, hybrid")
	flags.StringVar(&method, "method", "kmeans", "clustering method (kmeans, dbscan, hdbscan)")
	flags.IntVar(&k, "k", 8, "number of clusters for kmeans")
	flags.Float64Var(&epsilon, "eps", 0.5, "epsilon neighborhood distance for dbscan")
	flags.Float64Var(&epsilon, "epsilon", 0.5, "epsilon neighborhood distance for dbscan")
	flags.IntVar(&minPoints, "min-points", 0, "minimum points for dbscan core points or hdbscan cluster size")
	flags.IntVar(&minClusterSize, "min-cluster-size", 0, "minimum cluster size for hdbscan")
	flags.StringVar(&projection, "projection", "pca", "projection method for cluster visualization (pca or tsne)")

	return cmd
}

func printClusterSummary(cmd *cobra.Command, result pipeline.ClusterResult) error {
	for _, item := range result.Clusters {
		x := item.Parameters["projection_x"]
		y := item.Parameters["projection_y"]
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%d\t%.4f\t%.4f\n", item.Label, item.Size, x, y); err != nil {
			return err
		}
	}
	return nil
}
