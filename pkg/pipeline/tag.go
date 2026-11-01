package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

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
	// Concurrency sets how many samples are tagged in parallel (<= 0 means
	// 1, sequential processing). When CLAP tagging is enabled the run falls
	// back to sequential processing because the CLAP tagger is not
	// concurrency-safe.
	Concurrency int

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

// AutoTag generates and persists tags for samples using the metadata,
// optional rule, and optional CLAP taggers. When a current SampleAnalysis is
// already stored in the repository, it consumes the stored metrics and
// attributes without re-decoding the audio file from disk.
func AutoTag(ctx context.Context, repo *db.Repository, samples []core.Sample, opts AutoTagOptions) (AutoTagSummary, error) {
	if repo == nil {
		return AutoTagSummary{}, errors.New("pipeline: nil repository")
	}

	taggers := []core.Tagger{tags.NewMetadataTagger()}
	if strings.TrimSpace(opts.RuleFile) != "" {
		ruleTagger, err := tags.NewRuleTaggerFromFile(opts.RuleFile)
		if err != nil {
			return AutoTagSummary{}, err
		}
		taggers = append(taggers, ruleTagger)
	}
	clapEnabled := strings.TrimSpace(opts.CLAPModelDir) != ""
	if clapEnabled {
		clapTagger, err := tags.NewLocalCLAPTagger(opts.CLAPModelDir, opts.CLAPLib, opts.CLAPLabels, opts.CLAPMinScore, opts.CLAPMaxPredicted)
		if err != nil {
			return AutoTagSummary{}, fmt.Errorf("initialise CLAP tagger: %w", err)
		}
		defer func() { _ = clapTagger.Close() }()
		taggers = append(taggers, clapTagger)
	}

	concurrency := opts.Concurrency
	if clapEnabled {
		concurrency = 1
	}
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > len(samples) {
		concurrency = len(samples)
	}

	var (
		summary  AutoTagSummary
		summaryMu sync.Mutex
		firstErr error
		errMu    sync.Mutex
	)

	setErr := func(err error) {
		errMu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		errMu.Unlock()
	}

	tagSample := func(sample core.Sample) {
		verbosef(opts.Logger, "tagging sample %s", sample.Path)

		var (
			combined  core.AnalysisResult
			sampleCtx = ctx
		)

		stored, err := repo.GetSampleAnalysis(ctx, sample.ID)
		analysisCurrent := err == nil && (!sample.ModifiedAt.IsZero() && stored.AnalyzedModifiedAt.Equal(sample.ModifiedAt) || sample.ModifiedAt.IsZero())

		if analysisCurrent {
			combined = AnalysisResultFromSampleAnalysis(stored)
		} else {
			output, analyzeErr := AnalyzeSample(ctx, sample)
			if analyzeErr != nil {
				if opts.SkipUndecodable && zaudio.IsDecodeError(analyzeErr) {
					summaryMu.Lock()
					summary.Skipped++
					summaryMu.Unlock()
					if opts.OnSkip != nil {
						opts.OnSkip(sample, analyzeErr)
					}
					return
				}
				setErr(fmt.Errorf("analyze sample %q: %w", sample.Path, analyzeErr))
				return
			}

			if !opts.DryRun {
				if err := PersistAnalysisOutput(ctx, repo, sample, output); err != nil {
					setErr(err)
					return
				}
			}
			combined = CombineAnalysisResults(sample.ID, output.Results...)
		}

		generated := make([]core.Tag, 0, 16)
		for _, tagger := range taggers {
			emitted, err := tagger.Tags(sampleCtx, sample, combined)
			if err != nil {
				setErr(err)
				return
			}
			generated = append(generated, emitted...)
		}
		generated = DedupeGeneratedTags(generated)
		verbosef(opts.Logger, "generated %d tags for %s: %s", len(generated), sample.Path, strings.Join(TagNames(generated), ", "))

		if opts.DryRun {
			summaryMu.Lock()
			summary.Processed++
			summary.Applied += len(generated)
			summaryMu.Unlock()
			verbosef(opts.Logger, "skipping tag persistence for %s (dry run)", sample.Path)
			return
		}

		applied := 0
		for _, tag := range generated {
			verbosef(opts.Logger, "attaching tag %s to %s", NormalizedTagName(tag), sample.Path)
			tagID, err := repo.EnsureTag(ctx, tag)
			if err != nil {
				setErr(err)
				return
			}
			if err := repo.AttachTagToSample(ctx, sample.ID, tagID, tag.Source, tag.Confidence); err != nil {
				setErr(err)
				return
			}
			applied++
		}

		summaryMu.Lock()
		summary.Processed++
		summary.Applied += applied
		summaryMu.Unlock()
	}

	jobs := make(chan core.Sample)
	var wg sync.WaitGroup
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for sample := range jobs {
				if func() bool {
					errMu.Lock()
					stopped := firstErr != nil
					errMu.Unlock()
					return stopped
				}() {
					continue
				}
				tagSample(sample)
			}
		}()
	}

	for _, sample := range samples {
		jobs <- sample
	}
	close(jobs)
	wg.Wait()

	return summary, firstErr
}

// NormalizedTagName returns the tag's normalized name, normalizing the raw
// name when unset.
func NormalizedTagName(tag core.Tag) string {
	if tag.NormalizedName != "" {
		return tag.NormalizedName
	}
	return core.NormalizeTagName(tag.Name)
}
