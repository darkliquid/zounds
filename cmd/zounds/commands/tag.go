package commands

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/db"
	"github.com/darkliquid/zounds/pkg/pipeline"
)

func newTagCommand(cfg *Config) *cobra.Command {
	var (
		auto             bool
		list             bool
		path             string
		addTags          []string
		removeTags       []string
		ruleFile         string
		clapModelDir     string
		clapLib          string
		clapLabels       []string
		clapMinScore     float32
		clapMaxPredicted int
	)

	cmd := &cobra.Command{
		Use:   "tag",
		Short: "Manage and infer sample tags",
		RunE: func(cmd *cobra.Command, args []string) error {
			modeCount := 0
			for _, enabled := range []bool{auto, list, len(addTags) > 0, len(removeTags) > 0} {
				if enabled {
					modeCount++
				}
			}
			if modeCount != 1 {
				return fmt.Errorf("choose exactly one of --auto, --list, --add, or --remove")
			}

			ctx := cmd.Context()
			repo, closer, err := openRepository(ctx, cfg)
			if err != nil {
				return err
			}
			defer func() { _ = closer.Close() }()

			switch {
			case auto:
				return runAutoTagging(ctx, cmd, repo, cfg, path, ruleFile, clapModelDir, clapLib, clapLabels, clapMinScore, clapMaxPredicted)
			case list:
				return runListTags(ctx, cmd, repo)
			case len(addTags) > 0:
				return runManualTagUpdate(ctx, cmd, repo, cfg, path, addTags, true, newVerboseLogger(cfg, cmd.ErrOrStderr()))
			case len(removeTags) > 0:
				return runManualTagUpdate(ctx, cmd, repo, cfg, path, removeTags, false, newVerboseLogger(cfg, cmd.ErrOrStderr()))
			default:
				return fmt.Errorf("no tag action selected")
			}
		},
	}

	flags := cmd.Flags()
	flags.BoolVar(&auto, "auto", false, "infer and attach metadata/rule tags")
	flags.BoolVar(&list, "list", false, "list known tags and usage counts")
	flags.StringVar(&path, "path", "", "sample path to target for add/remove or auto-tagging one file")
	flags.StringSliceVar(&addTags, "add", nil, "manually add one or more tags")
	flags.StringSliceVar(&removeTags, "remove", nil, "manually remove one or more tags")
	flags.StringVar(&ruleFile, "rule-file", "", "optional JSON rule config for expr-based rule tagging")
	flags.StringVar(&clapModelDir, "clap-model-dir", "", "path to directory containing CLAP ONNX models and tokenizer.json (enables CLAP tagging)")
	flags.StringVar(&clapLib, "clap-lib", "", "path to the ONNX Runtime shared library (default: platform search path)")
	flags.StringSliceVar(&clapLabels, "clap-label", nil, "text labels to classify audio against with CLAP (default: built-in list)")
	flags.Float32Var(&clapMinScore, "clap-min-score", 0, "minimum CLAP similarity score required before a label is emitted")
	flags.IntVar(&clapMaxPredicted, "clap-max-predicted", 0, "maximum number of CLAP labels to emit per sample")

	return cmd
}

func runAutoTagging(ctx context.Context, cmd *cobra.Command, repo *db.Repository, cfg *Config, targetPath, ruleFile, clapModelDir, clapLib string, clapLabels []string, clapMinScore float32, clapMaxPredicted int) error {
	if clapModelDir == "" {
		clapModelDir = os.Getenv("ZOUNDS_CLAP_MODEL_DIR")
	}

	samples, err := pipeline.SelectSamplesByPath(ctx, repo, targetPath)
	if err != nil {
		return err
	}

	summary, err := pipeline.AutoTag(ctx, repo, samples, pipeline.AutoTagOptions{
		DryRun:          cfg.DryRun,
		SkipUndecodable: strings.TrimSpace(targetPath) == "",
		Concurrency:     cfg.Concurrency,
		Logger:          newVerboseLogger(cfg, cmd.ErrOrStderr()),
		OnSkip: func(sample core.Sample, err error) {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: skipping undecodable sample %s: %v\n", sample.Path, err)
		},
		RuleFile:         ruleFile,
		CLAPModelDir:     clapModelDir,
		CLAPLib:          clapLib,
		CLAPLabels:       clapLabels,
		CLAPMinScore:     clapMinScore,
		CLAPMaxPredicted: clapMaxPredicted,
	})
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "processed %d samples and applied %d tags\n", summary.Processed, summary.Applied)
	return err
}

func runListTags(ctx context.Context, cmd *cobra.Command, repo *db.Repository) error {
	usages, err := repo.ListTagUsage(ctx)
	if err != nil {
		return err
	}

	for _, usage := range usages {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%d\t%s\n", usage.Tag.NormalizedName, usage.SampleCount, usage.Tag.Source); err != nil {
			return err
		}
	}

	return nil
}

func runManualTagUpdate(ctx context.Context, cmd *cobra.Command, repo *db.Repository, cfg *Config, samplePath string, values []string, attach bool, logger *log.Logger) error {
	if strings.TrimSpace(samplePath) == "" {
		return fmt.Errorf("--path is required for manual tag changes")
	}

	sample, err := repo.FindSampleByPath(ctx, samplePath)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("sample %q is not indexed", samplePath)
		}
		return err
	}

	updated := 0
	for _, raw := range values {
		normalized := core.NormalizeTagName(raw)
		if normalized == "" {
			continue
		}
		if attach {
			verbosef(logger, "adding tag %s to %s", normalized, sample.Path)
		} else {
			verbosef(logger, "removing tag %s from %s", normalized, sample.Path)
		}

		if cfg.DryRun {
			updated++
			continue
		}

		tagID, err := repo.EnsureTag(ctx, core.Tag{
			Name:           normalized,
			NormalizedName: normalized,
			Source:         "manual",
			Confidence:     1.0,
		})
		if err != nil {
			return err
		}

		if attach {
			if err := repo.AttachTagToSample(ctx, sample.ID, tagID, "manual", 1.0); err != nil {
				return err
			}
		} else {
			if err := repo.RemoveTagFromSample(ctx, sample.ID, tagID); err != nil {
				return err
			}
		}
		updated++
	}

	action := "added"
	if !attach {
		action = "removed"
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s %d tags for %s\n", action, updated, sample.Path)
	return err
}
