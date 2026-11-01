package commands

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"sync"

	"github.com/spf13/cobra"

	"github.com/darkliquid/zounds/pkg/analysis"
	zaudio "github.com/darkliquid/zounds/pkg/audio"
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
			switch keepStrategy {
			case dedup.KeepFirstPath, dedup.KeepOldest, dedup.KeepNewest, dedup.KeepBestQuality:
			default:
				return fmt.Errorf("invalid --keep strategy %q: must be one of first-path, oldest, newest, best-quality", strategy)
			}

			var actions []dedup.CullAction
			if exact {
				finder := dedup.NewExactFinder(cfg.Concurrency, logger)
				groups, err := finder.Find(ctx, samples)
				if err != nil {
					return err
				}
				actions = dedup.PlanCull(groups, keepStrategy)
			} else {
				hashes, err := buildPerceptualHashes(ctx, repo, samples, cmd.ErrOrStderr(), logger, cfg.Concurrency)
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
	flags.StringVar(&strategy, "keep", string(dedup.KeepFirstPath), "keep strategy: first-path, oldest, newest, best-quality")

	return cmd
}

func buildPerceptualHashes(ctx context.Context, repo *db.Repository, samples []core.Sample, errOut io.Writer, logger *log.Logger, concurrency int) ([]dedup.PerceptualHash, error) {
	existingHashes, err := repo.ListPerceptualHashes(ctx, "spectral-8x8")
	existingBySampleID := make(map[int64]string)
	if err == nil {
		for _, h := range existingHashes {
			if h.Hash != "" {
				existingBySampleID[h.SampleID] = h.Hash
			}
		}
	}

	var (
		hashes         = make([]dedup.PerceptualHash, 0, len(samples))
		missingSamples []core.Sample
	)

	for _, sample := range samples {
		if hashVal, ok := existingBySampleID[sample.ID]; ok {
			hashes = append(hashes, dedup.PerceptualHash{
				Sample: sample,
				Hash:   hashVal,
			})
		} else {
			missingSamples = append(missingSamples, sample)
		}
	}

	if len(missingSamples) == 0 {
		return hashes, nil
	}

	analyzer, err := analysis.NewPerceptualHashAnalyzer(nil)
	if err != nil {
		return nil, err
	}

	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > len(missingSamples) {
		concurrency = len(missingSamples)
	}

	var (
		mu       sync.Mutex
		firstErr error
		errMu    sync.Mutex
	)

	setErr := func(err error) {
		errMu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		errMu.Unlock()
	}

	jobs := make(chan core.Sample)
	var wg sync.WaitGroup
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for sample := range jobs {
				verbosef(logger, "building perceptual hash for %s", sample.Path)
				result, err := analyzer.Analyze(ctx, sample)
				if err != nil {
					if zaudio.IsDecodeError(err) {
						_, _ = fmt.Fprintf(errOut, "warning: skipping undecodable sample %s: %v\n", sample.Path, err)
						continue
					}
					setErr(err)
					return
				}
				hashVal := result.Attributes["perceptual_hash"]
				algo := result.Attributes["hash_algorithm"]
				bits := int(result.Metrics["hash_bits"])
				if hashVal != "" && algo != "" {
					_ = repo.UpsertPerceptualHash(ctx, core.PerceptualHash{
						SampleID:  sample.ID,
						Algorithm: algo,
						Hash:      hashVal,
						HashBits:  bits,
					})
				}
				mu.Lock()
				hashes = append(hashes, dedup.PerceptualHash{
					Sample: sample,
					Hash:   hashVal,
				})
				mu.Unlock()
			}
		}()
	}

	for _, sample := range missingSamples {
		jobs <- sample
	}
	close(jobs)
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
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
