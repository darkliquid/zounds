package commands

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/darkliquid/zounds/pkg/analysis"
	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/db"
	"github.com/darkliquid/zounds/pkg/tags"
)

func newTagCommand(cfg *Config) *cobra.Command {
	var (
		auto         bool
		list         bool
		path         string
		addTags      []string
		removeTags   []string
		clapEndpoint string
		clapAPIKey   string
		clapLabels   []string
		ruleFile     string
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
			defer closer.Close()

			switch {
			case auto:
				return runAutoTagging(ctx, cmd, repo, cfg, path, clapEndpoint, clapAPIKey, clapLabels, ruleFile)
			case list:
				return runListTags(ctx, cmd, repo)
			case len(addTags) > 0:
				return runManualTagUpdate(ctx, cmd, repo, cfg, path, addTags, true)
			case len(removeTags) > 0:
				return runManualTagUpdate(ctx, cmd, repo, cfg, path, removeTags, false)
			default:
				return fmt.Errorf("no tag action selected")
			}
		},
	}

	flags := cmd.Flags()
	flags.BoolVar(&auto, "auto", false, "infer and attach path/metadata tags")
	flags.BoolVar(&list, "list", false, "list known tags and usage counts")
	flags.StringVar(&path, "path", "", "sample path to target for add/remove or auto-tagging one file")
	flags.StringSliceVar(&addTags, "add", nil, "manually add one or more tags")
	flags.StringSliceVar(&removeTags, "remove", nil, "manually remove one or more tags")
	flags.StringVar(&clapEndpoint, "clap-endpoint", "", "optional CLAP classifier endpoint for auto-tagging")
	flags.StringVar(&clapAPIKey, "clap-api-key", "", "optional API key for the CLAP classifier endpoint")
	flags.StringSliceVar(&clapLabels, "clap-label", nil, "candidate CLAP labels to classify against (repeatable)")
	flags.StringVar(&ruleFile, "rule-file", "", "optional JSON rule config for expr-based rule tagging")

	return cmd
}

func runAutoTagging(ctx context.Context, cmd *cobra.Command, repo *db.Repository, cfg *Config, targetPath, clapEndpoint, clapAPIKey string, clapLabels []string, ruleFile string) error {
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
	pathTagger := tags.NewPathTagger()
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
	clapTagger := tags.NewCLAPTagger(clapEndpoint, clapAPIKey, clapLabels)

	applied := 0
	for _, sample := range samples {
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
		pathTags, err := pathTagger.Tags(ctx, sample, combined)
		if err != nil {
			return err
		}
		metadataTags, err := metadataTagger.Tags(ctx, sample, combined)
		if err != nil {
			return err
		}
		ruleTags, err := ruleTagger.Tags(ctx, sample, combined)
		if err != nil {
			return err
		}
		generated = append(generated, pathTags...)
		generated = append(generated, metadataTags...)
		generated = append(generated, ruleTags...)
		if strings.TrimSpace(clapEndpoint) != "" {
			clapTags, err := clapTagger.Tags(ctx, sample, combined)
			if err != nil {
				return err
			}
			generated = append(generated, clapTags...)
		}
		generated = dedupeGeneratedTags(generated)

		if cfg.DryRun {
			applied += len(generated)
			continue
		}

		for _, tag := range generated {
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

func runManualTagUpdate(ctx context.Context, cmd *cobra.Command, repo *db.Repository, cfg *Config, samplePath string, values []string, attach bool) error {
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
