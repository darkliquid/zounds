package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/darkliquid/zounds/pkg/core"
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
			defer closer.Close()

			samples, err := selectSamplesForTagging(ctx, repo, targetPath)
			if err != nil {
				return err
			}

			for _, sample := range samples {
				tags, err := repo.ListTagsForSample(ctx, sample.ID)
				if err != nil {
					return err
				}
				rendered, err := zrename.RenderTemplate(templateText, zrename.BuildTemplateData(sample, tags, nil))
				if err != nil {
					return err
				}

				destination, err := resolveRenameTarget(sample, rendered)
				if err != nil {
					return err
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s\n", sample.Path, destination); err != nil {
					return err
				}

				if cfg.DryRun || sample.Path == destination {
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
				if err := repo.UpdateSampleLocation(ctx, sample.ID, destination, relativePath, filepath.Base(destination), core.DetectFormatFromExtension(destination), info.Size(), info.ModTime()); err != nil {
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

func resolveRenameTarget(sample core.Sample, rendered string) (string, error) {
	rendered = strings.TrimSpace(rendered)
	if rendered == "" {
		return "", fmt.Errorf("rename template rendered an empty path")
	}
	if filepath.Ext(rendered) == "" && sample.Extension != "" {
		rendered += "." + sample.Extension
	}

	if filepath.IsAbs(rendered) {
		return filepath.Clean(rendered), nil
	}
	return filepath.Clean(filepath.Join(sample.SourceRoot, rendered)), nil
}
