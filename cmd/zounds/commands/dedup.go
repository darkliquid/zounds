package commands

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"

	"github.com/darkliquid/zounds/pkg/analysis"
	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/db"
	"github.com/darkliquid/zounds/pkg/dedup"
)

func newDedupCommand(cfg *Config) *cobra.Command {
	var (
		exact      bool
		perceptual bool
		threshold  int
		deleteMode bool
		strategy   string
	)

	cmd := &cobra.Command{
		Use:   "dedup",
		Short: "Find exact or perceptual duplicates",
		RunE: func(cmd *cobra.Command, args []string) error {
			if (exact && perceptual) || (!exact && !perceptual) {
				return fmt.Errorf("choose exactly one of --exact or --perceptual")
			}

			ctx := cmd.Context()
			logger := newVerboseLogger(cfg, cmd.ErrOrStderr())
			repo, closer, err := openRepository(ctx, cfg)
			if err != nil {
				return err
			}
			defer func() { _ = closer.Close() }()

			samples, err := repo.ListSamples(ctx)
			if err != nil {
				return err
			}
			if len(samples) == 0 {
				return fmt.Errorf("no indexed samples found")
			}

			keepStrategy := dedup.KeepStrategy(strategy)
			var actions []dedup.CullAction
			if exact {
				finder := dedup.NewExactFinder(cfg.Concurrency, logger)
				groups, err := finder.Find(ctx, samples)
				if err != nil {
					return err
				}
				actions = dedup.PlanCull(groups, keepStrategy)
			} else {
				hashes, err := buildPerceptualHashes(ctx, samples, logger)
				if err != nil {
					return err
				}
				groups, err := dedup.NewPerceptualFinder(threshold).Find(hashes)
				if err != nil {
					return err
				}
				actions = dedup.PlanPerceptualCull(groups, keepStrategy)
			}

			if len(actions) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "no duplicates found")
				return err
			}

			if err := printCullActions(cmd, actions); err != nil {
				return err
			}
			if deleteMode && !cfg.DryRun {
				return applyCullActions(ctx, repo, actions, logger)
			}
			return nil
		},
	}

	flags := cmd.Flags()
	flags.BoolVar(&exact, "exact", false, "find exact duplicates by SHA-256")
	flags.BoolVar(&perceptual, "perceptual", false, "find near-duplicates by perceptual hash")
	flags.IntVar(&threshold, "threshold", 8, "maximum Hamming distance for perceptual dedup")
	flags.BoolVar(&deleteMode, "delete", false, "delete files selected for removal")
	flags.StringVar(&strategy, "keep", string(dedup.KeepFirstPath), "keep strategy: first-path, oldest, newest")

	return cmd
}

func buildPerceptualHashes(ctx context.Context, samples []core.Sample, logger *log.Logger) ([]dedup.PerceptualHash, error) {
	analyzer, err := analysis.NewPerceptualHashAnalyzer(nil)
	if err != nil {
		return nil, err
	}

	hashes := make([]dedup.PerceptualHash, 0, len(samples))
	for _, sample := range samples {
		verbosef(logger, "building perceptual hash for %s", sample.Path)
		result, err := analyzer.Analyze(ctx, sample)
		if err != nil {
			return nil, err
		}
		hashes = append(hashes, dedup.PerceptualHash{
			Sample: sample,
			Hash:   result.Attributes["perceptual_hash"],
		})
	}

	return hashes, nil
}

func printCullActions(cmd *cobra.Command, actions []dedup.CullAction) error {
	for _, action := range actions {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "keep\t%s\n", action.Keep.Path); err != nil {
			return err
		}
		for _, sample := range action.Remove {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "remove\t%s\n", sample.Path); err != nil {
				return err
			}
		}
	}
	return nil
}

func applyCullActions(ctx context.Context, repo *db.Repository, actions []dedup.CullAction, logger *log.Logger) error {
	for _, action := range actions {
		for _, sample := range action.Remove {
			verbosef(logger, "removing duplicate file %s", sample.Path)
			if err := os.Remove(sample.Path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove duplicate file %q: %w", sample.Path, err)
			}
			if err := repo.DeleteSampleByID(ctx, sample.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
