package commands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/darkliquid/zounds/pkg/cluster"
	"github.com/darkliquid/zounds/pkg/db"
)

func newClusterCommand(cfg *Config) *cobra.Command {
	var (
		method string
		k      int
	)

	cmd := &cobra.Command{
		Use:   "cluster",
		Short: "Cluster related sounds from stored feature vectors",
		RunE: func(cmd *cobra.Command, args []string) error {
			if method != "kmeans" {
				return fmt.Errorf("unsupported clustering method %q", method)
			}

			ctx := cmd.Context()
			repo, closer, err := openRepository(ctx, cfg)
			if err != nil {
				return err
			}
			defer closer.Close()

			vectors, err := repo.ListFeatureVectors(ctx, "analysis")
			if err != nil {
				return err
			}
			if len(vectors) == 0 {
				return fmt.Errorf("no analysis feature vectors found; run zounds analyze first")
			}

			model := cluster.NewKMeans(cluster.KMeansOptions{K: k})
			result, err := model.Fit(vectors)
			if err != nil {
				return err
			}

			if cfg.DryRun {
				return printClusterSummary(cmd, result)
			}

			if err := persistKMeansResult(ctx, repo, result); err != nil {
				return err
			}

			return printClusterSummary(cmd, result)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&method, "method", "kmeans", "clustering method")
	flags.IntVar(&k, "k", 8, "number of clusters for kmeans")

	return cmd
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

func printClusterSummary(cmd *cobra.Command, result cluster.KMeansResult) error {
	for _, item := range result.Clusters {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%d\n", item.Label, item.Size); err != nil {
			return err
		}
	}
	return nil
}
