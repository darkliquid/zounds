package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/darkliquid/zounds/pkg/scanner"
)

func newScanCommand(cfg *Config) *cobra.Command {
	var includeHidden bool
	var followSymlinks bool

	cmd := &cobra.Command{
		Use:   "scan <dir> [dir...]",
		Short: "Scan directories and register audio files",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			s := scanner.New(scanner.Options{
				Workers:       cfg.Concurrency,
				IncludeHidden: includeHidden,
				FollowSymlink: followSymlinks,
			})

			samples, err := s.Scan(ctx, args...)
			if err != nil {
				return err
			}

			if cfg.DryRun {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "discovered %d audio files (dry run)\n", len(samples))
				return err
			}

			repo, closer, err := openRepository(ctx, cfg)
			if err != nil {
				return err
			}
			defer closer.Close()

			for _, sample := range samples {
				if _, err := repo.UpsertSample(ctx, sample); err != nil {
					return err
				}
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "indexed %d audio files into %s\n", len(samples), cfg.DatabasePath)
			return err
		},
	}

	flags := cmd.Flags()
	flags.BoolVar(&includeHidden, "include-hidden", false, "include hidden files and directories")
	flags.BoolVar(&followSymlinks, "follow-symlinks", false, "follow symlinked files while scanning")

	return cmd
}
