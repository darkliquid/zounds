package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/darkliquid/zounds/pkg/cluster"
	"github.com/darkliquid/zounds/pkg/pipeline"
)

func newClusterCommand(cfg *Config) *cobra.Command {
	var (
		method     string
		k          int
		projection string
		metric     string
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
				Metric:     metric,
				Method:     method,
				K:          k,
				Projection: projection,
				DryRun:     cfg.DryRun,
			})
			if err != nil {
				return err
			}

			return printClusterSummary(cmd, result)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&metric, "metric", "", "metric space: acoustic, clap, timbre, harmonic, rhythm, perceptual, hybrid")
	flags.StringVar(&method, "method", "kmeans", "clustering method")
	flags.IntVar(&k, "k", 8, "number of clusters for kmeans")
	flags.StringVar(&projection, "projection", "pca", "projection method for cluster visualization (pca or tsne)")

	return cmd
}

func printClusterSummary(cmd *cobra.Command, result cluster.KMeansResult) error {
	for _, item := range result.Clusters {
		x := item.Parameters["projection_x"]
		y := item.Parameters["projection_y"]
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%d\t%.4f\t%.4f\n", item.Label, item.Size, x, y); err != nil {
			return err
		}
	}
	return nil
}
