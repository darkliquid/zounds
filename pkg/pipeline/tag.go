package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/darkliquid/zounds/pkg/analysis"
	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/db"
	"github.com/darkliquid/zounds/pkg/tags"
)

// AutoTagOptions controls a batch auto-tagging run.
type AutoTagOptions struct {
	// DryRun generates tags but skips persistence.
	DryRun bool
	// SkipUndecodable logs a warning and continues when a sample cannot be
	// decoded instead of aborting the batch.
	SkipUndecodable bool
	// Logger receives verbose per-sample progress lines when non-nil.
	Logger *log.Logger
	// OnSkip receives samples that were skipped due to decode errors when
	// SkipUndecodable is set.
	OnSkip func(sample core.Sample, err error)

	// RuleFile is an optional path to a JSON rule config for the expr-based
	// rule tagger. Empty means no rule tagging.
	RuleFile string
	// CLAPModelDir enables the local CLAP zero-shot tagger when set.
	CLAPModelDir string
	CLAPLib      string
	CLAPLabels   []string
	// CLAPMinScore is the minimum similarity score before a label is emitted.
	CLAPMinScore float32
	// CLAPMaxPredicted caps the number of CLAP labels per sample.
	CLAPMaxPredicted int
}

// AutoTagSummary reports the outcome of an auto-tagging run.
type AutoTagSummary struct {
	Processed int
	Applied   int
	Skipped   int
}

// tagAnalyzers is the focused analyzer set used by auto-tagging. Unlike
// AnalyzeSample the taggers do not need the full suite.
func tagAnalyzers(registry *zaudio.Registry) ([]core.Analyzer, error) {
	factory := []func() (core.Analyzer, error){
		func() (core.Analyzer, error) { return analysis.NewMetadataAnalyzer(registry) },
		func() (core.Analyzer, error) { return analysis.NewSpectralAnalyzer(registry) },
		func() (core.Analyzer, error) { return analysis.NewDynamicsAnalyzer(registry) },
		func() (core.Analyzer, error) { return analysis.NewPitchAnalyzer(registry) },
		func() (core.Analyzer, error) { return analysis.NewKeyAnalyzer(registry) },
		func() (core.Analyzer, error) { return analysis.NewHarmonicsAnalyzer(registry) },
	}
	analyzers := make([]core.Analyzer, 0, len(factory))
	for _, create := range factory {
		analyzer, err := create()
		if err != nil {
			return nil, err
		}
		analyzers = append(analyzers, analyzer)
	}
	return analyzers, nil
}

// AutoTag generates and persists tags for samples using the metadata,
// optional rule, and optional CLAP taggers over a shared per-sample decode.
func AutoTag(ctx context.Context, repo *db.Repository, samples []core.Sample, opts AutoTagOptions) (AutoTagSummary, error) {
	if repo == nil {
		return AutoTagSummary{}, errors.New("pipeline: nil repository")
	}

	registry, err := NewCodecRegistry()
	if err != nil {
		return AutoTagSummary{}, fmt.Errorf("create codec registry: %w", err)
	}
	analyzers, err := tagAnalyzers(registry)
	if err != nil {
		return AutoTagSummary{}, err
	}

	taggers := []core.Tagger{tags.NewMetadataTagger()}
	if strings.TrimSpace(opts.RuleFile) != "" {
		ruleTagger, err := tags.NewRuleTaggerFromFile(opts.RuleFile)
		if err != nil {
			return AutoTagSummary{}, err
		}
		taggers = append(taggers, ruleTagger)
	}
	if strings.TrimSpace(opts.CLAPModelDir) != "" {
		clapTagger, err := tags.NewLocalCLAPTagger(opts.CLAPModelDir, opts.CLAPLib, opts.CLAPLabels, opts.CLAPMinScore, opts.CLAPMaxPredicted)
		if err != nil {
			return AutoTagSummary{}, fmt.Errorf("initialise CLAP tagger: %w", err)
		}
		defer func() { _ = clapTagger.Close() }()
		taggers = append(taggers, clapTagger)
	}

	var summary AutoTagSummary
	for _, sample := range samples {
		verbosef(opts.Logger, "tagging sample %s", sample.Path)

		decoded, err := zaudio.DecodeFile(ctx, registry, sample.Path)
		if err != nil {
			if opts.SkipUndecodable && zaudio.IsDecodeError(err) {
				summary.Skipped++
				if opts.OnSkip != nil {
					opts.OnSkip(sample, err)
				}
				continue
			}
			return summary, err
		}
		sampleCtx := analysis.ContextWithDecodedSample(ctx, sample.Path, decoded)

		results := make([]core.AnalysisResult, 0, len(analyzers))
		for _, analyzer := range analyzers {
			result, analyzeErr := analyzer.Analyze(ctx, sample)
			if analyzeErr != nil {
				return summary, analyzeErr
			}
			results = append(results, result)
		}
		combined := CombineAnalysisResults(sample.ID, results...)

		generated := make([]core.Tag, 0, 16)
		for _, tagger := range taggers {
			emitted, err := tagger.Tags(sampleCtx, sample, combined)
			if err != nil {
				return summary, err
			}
			generated = append(generated, emitted...)
		}
		generated = DedupeGeneratedTags(generated)
		verbosef(opts.Logger, "generated %d tags for %s: %s", len(generated), sample.Path, strings.Join(TagNames(generated), ", "))
		summary.Processed++

		if opts.DryRun {
			summary.Applied += len(generated)
			verbosef(opts.Logger, "skipping tag persistence for %s (dry run)", sample.Path)
			continue
		}

		for _, tag := range generated {
			verbosef(opts.Logger, "attaching tag %s to %s", NormalizedTagName(tag), sample.Path)
			tagID, err := repo.EnsureTag(ctx, tag)
			if err != nil {
				return summary, err
			}
			if err := repo.AttachTagToSample(ctx, sample.ID, tagID, tag.Source, tag.Confidence); err != nil {
				return summary, err
			}
			summary.Applied++
		}
	}

	return summary, nil
}

// NormalizedTagName returns the tag's normalized name, normalizing the raw
// name when unset.
func NormalizedTagName(tag core.Tag) string {
	if tag.NormalizedName != "" {
		return tag.NormalizedName
	}
	return core.NormalizeTagName(tag.Name)
}
