package commands

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/darkliquid/zounds/pkg/analysis"
	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/db"
	"github.com/darkliquid/zounds/pkg/tags"
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
	logger := newVerboseLogger(cfg, cmd.ErrOrStderr())

	samples, err := selectSamplesForTagging(ctx, repo, targetPath)
	if err != nil {
		return err
	}

	analyzer, err := analysis.NewMetadataAnalyzer(nil)
	if err != nil {
		return err
	}
	spectralAnalyzer, err := analysis.NewSpectralAnalyzer(nil)
	if err != nil {
		return err
	}
	dynamicsAnalyzer, err := analysis.NewDynamicsAnalyzer(nil)
	if err != nil {
		return err
	}
	pitchAnalyzer, err := analysis.NewPitchAnalyzer(nil)
	if err != nil {
		return err
	}
	keyAnalyzer, err := analysis.NewKeyAnalyzer(nil)
	if err != nil {
		return err
	}
	harmonicsAnalyzer, err := analysis.NewHarmonicsAnalyzer(nil)
	if err != nil {
		return err
	}
	metadataTagger := tags.NewMetadataTagger()
	var ruleTagger tags.RuleTagger
	if strings.TrimSpace(ruleFile) != "" {
		ruleTagger, err = tags.NewRuleTaggerFromFile(ruleFile)
	} else {
		ruleTagger, err = tags.NewRuleTagger()
	}
	if err != nil {
		return err
	}

	var clapTagger *tags.LocalCLAPTagger
	if strings.TrimSpace(clapModelDir) != "" {
		clapTagger, err = tags.NewLocalCLAPTagger(clapModelDir, clapLib, clapLabels, clapMinScore, clapMaxPredicted)
		if err != nil {
			return fmt.Errorf("initialise CLAP tagger: %w", err)
		}
		defer func() { _ = clapTagger.Close() }()
	}

	applied := 0
	for _, sample := range samples {
		verbosef(logger, "tagging sample %s", sample.Path)
		results := make([]core.AnalysisResult, 0, 6)
		result, err := analyzer.Analyze(ctx, sample)
		if err != nil {
			return err
		}
		results = append(results, result)
		spectralResult, err := spectralAnalyzer.Analyze(ctx, sample)
		if err != nil {
			return err
		}
		results = append(results, spectralResult)
		dynamicsResult, err := dynamicsAnalyzer.Analyze(ctx, sample)
		if err != nil {
			return err
		}
		results = append(results, dynamicsResult)
		pitchResult, err := pitchAnalyzer.Analyze(ctx, sample)
		if err != nil {
			return err
		}
		results = append(results, pitchResult)
		keyResult, err := keyAnalyzer.Analyze(ctx, sample)
		if err != nil {
			return err
		}
		results = append(results, keyResult)
		harmonicsResult, err := harmonicsAnalyzer.Analyze(ctx, sample)
		if err != nil {
			return err
		}
		results = append(results, harmonicsResult)

		combined := combineAnalysisResults(sample.ID, results...)

		var generated []core.Tag
		metadataTags, err := metadataTagger.Tags(ctx, sample, combined)
		if err != nil {
			return err
		}
		ruleTags, err := ruleTagger.Tags(ctx, sample, combined)
		if err != nil {
			return err
		}
		generated = append(generated, metadataTags...)
		generated = append(generated, ruleTags...)
		if clapTagger != nil {
			clapTags, err := clapTagger.Tags(ctx, sample, combined)
			if err != nil {
				return err
			}
			generated = append(generated, clapTags...)
		}
		generated = dedupeGeneratedTags(generated)
		verbosef(logger, "generated %d tags for %s: %s", len(generated), sample.Path, strings.Join(tagNames(generated), ", "))

		if cfg.DryRun {
			applied += len(generated)
			verbosef(logger, "skipping tag persistence for %s (dry run)", sample.Path)
			continue
		}

		for _, tag := range generated {
			verbosef(logger, "attaching tag %s to %s", normalizedTagName(tag), sample.Path)
			tagID, err := repo.EnsureTag(ctx, tag)
			if err != nil {
				return err
			}
			if err := repo.AttachTagToSample(ctx, sample.ID, tagID); err != nil {
				return err
			}
			applied++
		}
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "processed %d samples and applied %d tags\n", len(samples), applied)
	return err
}

func combineAnalysisResults(sampleID int64, results ...core.AnalysisResult) core.AnalysisResult {
	combined := core.AnalysisResult{
		SampleID:    sampleID,
		Analyzer:    "combined",
		Version:     "0.1.0",
		Metrics:     analysis.FlattenMetrics(results...),
		Attributes:  map[string]string{},
		CompletedAt: results[0].CompletedAt,
	}

	for _, result := range results {
		for key, value := range result.Attributes {
			combined.Attributes[key] = value
		}
	}

	return combined
}

func dedupeGeneratedTags(tags []core.Tag) []core.Tag {
	seen := make(map[string]struct{}, len(tags))
	out := make([]core.Tag, 0, len(tags))
	for _, tag := range tags {
		if tag.NormalizedName == "" {
			tag.NormalizedName = core.NormalizeTagName(tag.Name)
		}
		if tag.NormalizedName == "" {
			continue
		}
		if _, ok := seen[tag.NormalizedName]; ok {
			continue
		}
		seen[tag.NormalizedName] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func tagNames(tags []core.Tag) []string {
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		names = append(names, normalizedTagName(tag))
	}
	if len(names) == 0 {
		return []string{"<none>"}
	}
	slices.Sort(names)
	return names
}

func normalizedTagName(tag core.Tag) string {
	if tag.NormalizedName != "" {
		return tag.NormalizedName
	}
	return core.NormalizeTagName(tag.Name)
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
			if err := repo.AttachTagToSample(ctx, sample.ID, tagID); err != nil {
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

func selectSamplesForTagging(ctx context.Context, repo *db.Repository, targetPath string) ([]core.Sample, error) {
	if strings.TrimSpace(targetPath) != "" {
		sample, err := repo.FindSampleByPath(ctx, targetPath)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("sample %q is not indexed", targetPath)
			}
			return nil, err
		}
		return []core.Sample{sample}, nil
	}

	return repo.ListSamples(ctx)
}
