package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/darkliquid/zounds/pkg/pipeline"
)

func newSimilarCommand(cfg *Config) *cobra.Command {
	var (
		limit     int
		threshold float64
		order     string
	)

	cmd := &cobra.Command{
		Use:   "similar <sample-ref> [sample-ref...]",
		Short: "List indexed sounds similar to one or more references",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			repo, closer, err := openRepository(ctx, cfg)
			if err != nil {
				return err
			}
			defer func() { _ = closer.Close() }()

			matches, err := pipeline.FindSimilar(ctx, repo, args, pipeline.SimilarOptions{
				Threshold: threshold,
				Order:     order,
				Limit:     limit,
			})
			if err != nil {
				return err
			}
			if len(matches) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "no similar samples matched the current threshold")
				return err
			}

			for _, match := range matches {
				if _, err := fmt.Fprintf(
					cmd.OutOrStdout(),
					"%.4f\t%s\t%s\n",
					match.Similarity,
					match.Reference.Path,
					match.Sample.Path,
				); err != nil {
					return err
				}
			}

			return nil
		},
	}

	flags := cmd.Flags()
	flags.IntVar(&limit, "limit", 25, "maximum number of similar samples to display (0 for unlimited)")
	flags.Float64Var(&threshold, "threshold", 0, "minimum cosine similarity required")
	flags.StringVar(&order, "order", "desc", "sort order for similarity: asc or desc")

	return cmd
}
