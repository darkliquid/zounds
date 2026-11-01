package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

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

			plan, err := pipeline.PlanRename(ctx, repo, samples, templateText)
			if err != nil {
				return err
			}

			_, err = pipeline.ExecuteRename(ctx, repo, plan, pipeline.RenameOptions{
				DryRun: cfg.DryRun,
				Logger: newVerboseLogger(cfg, cmd.ErrOrStderr()),
				OnRename: func(op zrename.RenameOp) {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s\n", op.Sample.Path, op.Destination)
				},
			})
			return err
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&templateText, "template", "", "Go template for the new relative path or file name")
	flags.StringVar(&targetPath, "path", "", "rename only one indexed sample path")

	return cmd
}
