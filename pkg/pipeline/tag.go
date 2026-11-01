package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
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

	// LocalModelKNN enables the weighted k-NN similarity tagger.
	LocalModelKNN          bool
	LocalModelK            int
	LocalModelMinScore     float64
	LocalModelMaxPredicted int
	LocalModelNamespace    string

	// LLM options for OpenAI-compatible tag generation.
	LLMEndpoint     string
	LLMAPIKey       string
	LLMModel        string
	LLMSystemPrompt string
	LLMMaxPredicted int
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

	if opts.LocalModelKNN {
		ns := opts.LocalModelNamespace
		if ns == "" {
			ns = "analysis"
		}
		examples, err := LoadKNNTrainingExamples(ctx, repo, ns)
		if err != nil {
			return AutoTagSummary{}, fmt.Errorf("load k-NN training examples: %w", err)
		}
		if len(examples) > 0 {
			taggers = append(taggers, tags.NewLocalModelTagger(
				examples,
				opts.LocalModelK,
				opts.LocalModelMinScore,
				opts.LocalModelMaxPredicted,
			))
		} else {
			verbosef(opts.Logger, "warning: k-NN tag propagation requested but no tagged training examples found in namespace %q", ns)
		}
	}

	if strings.TrimSpace(opts.LLMEndpoint) != "" && strings.TrimSpace(opts.LLMModel) != "" {
		apiKey := opts.LLMAPIKey
		if apiKey == "" {
			apiKey = os.Getenv("OPENAI_API_KEY")
			if apiKey == "" {
				apiKey = os.Getenv("LLM_API_KEY")
			}
		}
		llmTagger := tags.NewLLMTagger(opts.LLMEndpoint, apiKey, opts.LLMModel)
		if opts.LLMSystemPrompt != "" {
			llmTagger.SystemPrompt = opts.LLMSystemPrompt
		}
		if opts.LLMMaxPredicted > 0 {
			llmTagger.MaxPredicted = opts.LLMMaxPredicted
		}
		taggers = append(taggers, llmTagger)
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
			if fvs, err := repo.ListFeatureVectorsForSample(ctx, sample.ID); err == nil {
				combined.FeatureVectors = fvs
			}
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
			combined.FeatureVectors = append([]core.FeatureVector{output.Vector}, output.FeatureVectors...)
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

// LoadKNNTrainingExamples queries the repository for all tagged samples that
// have an existing feature vector in namespace, grouping their tags into
// tags.TrainingExample values for the k-NN local model tagger.
func LoadKNNTrainingExamples(ctx context.Context, repo *db.Repository, namespace string) ([]tags.TrainingExample, error) {
	if strings.TrimSpace(namespace) == "" {
		namespace = "analysis"
	}

	rows, err := repo.DB().QueryContext(ctx, `
		SELECT fv.sample_id, fv.namespace, fv.version, fv.dimensions, fv.values_json,
		       t.id, t.name, t.normalized_name, st.source, st.confidence
		FROM feature_vectors fv
		JOIN sample_tags st ON st.sample_id = fv.sample_id
		JOIN tags t ON t.id = st.tag_id
		WHERE fv.namespace = ?
		ORDER BY fv.sample_id;
	`, namespace)
	if err != nil {
		return nil, fmt.Errorf("query knn training examples: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var (
		examples      []tags.TrainingExample
		currentID     int64 = -1
		currentVector core.FeatureVector
		currentTags   []core.Tag
	)

	flushCurrent := func() {
		if currentID >= 0 && len(currentTags) > 0 {
			examples = append(examples, tags.TrainingExample{
				Vector: currentVector,
				Tags:   currentTags,
			})
		}
	}

	for rows.Next() {
		var (
			sampleID   int64
			ns         string
			version    string
			dimensions int
			valuesJSON string
			tagID      int64
			tagName    string
			tagNorm    string
			source     string
			confidence float64
		)
		if err := rows.Scan(&sampleID, &ns, &version, &dimensions, &valuesJSON, &tagID, &tagName, &tagNorm, &source, &confidence); err != nil {
			return nil, fmt.Errorf("scan knn training row: %w", err)
		}

		if sampleID != currentID {
			flushCurrent()
			currentID = sampleID
			currentTags = nil
			var values []float64
			if err := json.Unmarshal([]byte(valuesJSON), &values); err != nil {
				return nil, fmt.Errorf("unmarshal training vector for sample %d: %w", sampleID, err)
			}
			currentVector = core.FeatureVector{
				SampleID:   sampleID,
				Namespace:  ns,
				Version:    version,
				Dimensions: dimensions,
				Values:     values,
			}
		}

		currentTags = append(currentTags, core.Tag{
			ID:             tagID,
			Name:           tagName,
			NormalizedName: tagNorm,
			Source:         source,
			Confidence:     confidence,
		})
	}

	flushCurrent()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate knn training rows: %w", err)
	}

	return examples, nil
}
