package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/pipeline"
	zrename "github.com/darkliquid/zounds/pkg/rename"
)

func newRenameCommand(cfg *Config) *cobra.Command {
	var (
		templateText string
		targetPath   string
	)

	cmd := &cobra.Command{
		Use:   "rename --template <template>",
		Short: "Mass rename and reorganize library files",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(templateText) == "" {
				return fmt.Errorf("--template is required")
			}

			ctx := cmd.Context()
			repo, closer, err := openRepository(ctx, cfg)
			if err != nil {
				return err
			}
			defer func() { _ = closer.Close() }()

			samples, err := pipeline.SelectSamplesByPath(ctx, repo, targetPath)
			if err != nil {
				return err
			}

			ops := make([]zrename.RenameOp, 0, len(samples))
			for _, sample := range samples {
				tags, err := repo.ListTagsForSample(ctx, sample.ID)
				if err != nil {
					return err
				}
				rendered, err := zrename.RenderTemplate(templateText, zrename.BuildTemplateData(sample, tags, nil))
				if err != nil {
					return err
				}
				destination, err := zrename.ResolveTarget(sample, rendered)
				if err != nil {
					return err
				}
				ops = append(ops, zrename.RenameOp{
					Sample:      sample,
					Destination: destination,
				})
			}

			if err := zrename.ValidatePlan(ops); err != nil {
				return err
			}

			for _, op := range ops {
				sample := op.Sample
				destination := op.Destination
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s\n", sample.Path, destination); err != nil {
					return err
				}

				if cfg.DryRun || filepath.Clean(sample.Path) == destination {
					continue
				}

				if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
					return fmt.Errorf("create target directory: %w", err)
				}
				if err := os.Rename(sample.Path, destination); err != nil {
					return fmt.Errorf("rename %q -> %q: %w", sample.Path, destination, err)
				}

				info, err := os.Stat(destination)
				if err != nil {
					return fmt.Errorf("stat renamed file %q: %w", destination, err)
				}
				relativePath, err := filepath.Rel(sample.SourceRoot, destination)
				if err != nil {
					return fmt.Errorf("compute relative path for %q: %w", destination, err)
				}
				if err := repo.UpdateSampleLocation(ctx, sample.ID, destination, filepath.ToSlash(relativePath), filepath.Base(destination), core.DetectFormatFromExtension(destination), info.Size(), info.ModTime()); err != nil {
					return err
				}
			}

			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&templateText, "template", "", "Go template for the new relative path or file name")
	flags.StringVar(&targetPath, "path", "", "rename only one indexed sample path")

	return cmd
}
