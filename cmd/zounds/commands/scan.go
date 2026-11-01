package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/db"
	"github.com/darkliquid/zounds/pkg/scanner"
)

func newScanCommand(cfg *Config) *cobra.Command {
	var includeHidden bool
	var followSymlinks bool
	var prune bool

	cmd := &cobra.Command{
		Use:   "scan <dir> [dir...]",
		Short: "Scan directories and register audio files",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			logger := newVerboseLogger(cfg, cmd.ErrOrStderr())

			s := scanner.New(scanner.Options{
				Workers:       cfg.Concurrency,
				IncludeHidden: includeHidden,
				FollowSymlink: followSymlinks,
				Logger:        logger,
			})

			samples, err := s.Scan(ctx, args...)
			if err != nil {
				return err
			}

			if cfg.DryRun {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "discovered %d audio files (dry run)\n", len(samples)); err != nil {
					return err
				}
				if prune {
					repo, closer, err := openRepository(ctx, cfg, logger)
					if err != nil {
						return err
					}
					defer func() { _ = closer.Close() }()

					if _, err := pruneMissingSamples(ctx, repo, args, true, cmd.OutOrStdout()); err != nil {
						return err
					}
				}
				return nil
			}

			repo, closer, err := openRepository(ctx, cfg, logger)
			if err != nil {
				return err
			}
			defer func() { _ = closer.Close() }()

			for _, sample := range samples {
				if _, err := repo.UpsertSample(ctx, sample); err != nil {
					return err
				}
			}

			pruned := 0
			if prune {
				removed, err := pruneMissingSamples(ctx, repo, args, cfg.DryRun, cmd.OutOrStdout())
				if err != nil {
					return err
				}
				pruned = removed
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "indexed %d audio files into %s", len(samples), cfg.DatabasePath)
			if err != nil {
				return err
			}
			if prune {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), " and pruned %d missing samples", pruned)
			}
			_, printErr := fmt.Fprintln(cmd.OutOrStdout())
			return errors.Join(err, printErr)
		},
	}

	flags := cmd.Flags()
	flags.BoolVar(&includeHidden, "include-hidden", false, "include hidden files and directories")
	flags.BoolVar(&followSymlinks, "follow-symlinks", false, "follow symlinked files while scanning")
	flags.BoolVar(&prune, "prune", false, "remove indexed samples whose files no longer exist under the scanned roots")

	return cmd
}

// pruneMissingSamples deletes DB entries for indexed samples under the given
// roots whose files no longer exist on disk.
func pruneMissingSamples(ctx context.Context, repo *db.Repository, roots []string, dryRun bool, out io.Writer) (int, error) {
	all, err := repo.ListSamples(ctx)
	if err != nil {
		return 0, err
	}

	pruned := make([]core.Sample, 0)
	for _, sample := range all {
		if !sampleBelongsToRoots(sample, roots) {
			continue
		}
		if _, err := os.Stat(sample.Path); err == nil {
			continue
		}
		pruned = append(pruned, sample)
	}

	for _, sample := range pruned {
		if dryRun {
			if _, err := fmt.Fprintf(out, "would prune missing sample %s\n", sample.Path); err != nil {
				return 0, err
			}
			continue
		}
		if err := repo.DeleteSampleByID(ctx, sample.ID); err != nil {
			return 0, err
		}
	}

	return len(pruned), nil
}

func sampleBelongsToRoots(sample core.Sample, roots []string) bool {
	for _, root := range roots {
		cleanRoot := filepath.Clean(root)
		if filepath.Clean(sample.SourceRoot) == cleanRoot {
			return true
		}
		if strings.HasPrefix(filepath.Clean(sample.Path), cleanRoot+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}
