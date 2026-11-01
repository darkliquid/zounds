package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/pipeline"
)

func newAnalyzeCommand(cfg *Config) *cobra.Command {
	var (
		analyzeAll   bool
		id           int64
		force        bool
		clapModelDir string
		clapLib      string
	)

	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Run analyzers against indexed samples",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !analyzeAll && id == 0 {
				return fmt.Errorf("use --all or --id")
			}

			if clapModelDir == "" {
				clapModelDir = os.Getenv("ZOUNDS_CLAP_MODEL_DIR")
			}

			ctx := cmd.Context()
			repo, closer, err := openRepository(ctx, cfg)
			if err != nil {
				return err
			}
			defer func() { _ = closer.Close() }()

			samples, err := pipeline.SelectSamplesForAnalysis(ctx, repo, analyzeAll, id)
			if err != nil {
				return err
			}

			summary, err := pipeline.AnalyzeSamples(ctx, repo, samples, pipeline.AnalyzeOptions{
				DryRun:          cfg.DryRun,
				SkipUndecodable: analyzeAll,
				SkipUnchanged:   analyzeAll && !force,
				Concurrency:     cfg.Concurrency,
				Logger:          newVerboseLogger(cfg, cmd.ErrOrStderr()),
				CLAPModelDir:    clapModelDir,
				CLAPLib:         clapLib,
				OnSkip: func(sample core.Sample, err error) {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: skipping undecodable sample %s: %v\n", sample.Path, err)
				},
			})
			if err != nil {
				return err
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "analyzed %d samples", summary.Processed)
			if err != nil {
				return err
			}
			if summary.Unchanged > 0 {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), " (%d unchanged, skipped)", summary.Unchanged)
			}
			_, printErr := fmt.Fprintln(cmd.OutOrStdout())
			return errors.Join(err, printErr)
		},
	}

	flags := cmd.Flags()
	flags.BoolVar(&analyzeAll, "all", false, "analyze all indexed samples")
	flags.Int64Var(&id, "id", 0, "analyze a specific sample by id")
	flags.BoolVar(&force, "force", false, "re-analyze samples even when the stored analysis matches the current file mtime")
	flags.StringVar(&clapModelDir, "clap-model-dir", "", "path to directory containing CLAP ONNX models and tokenizer.json (enables 512-D CLAP embeddings)")
	flags.StringVar(&clapLib, "clap-lib", "", "path to the ONNX Runtime shared library (default: platform search path)")

	return cmd
}

func newInfoCommand(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info <file>",
		Short: "Show detailed information about a sample",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			info, err := pipeline.AnalyzeFileInfo(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(info)
		},
	}

	return cmd
}
