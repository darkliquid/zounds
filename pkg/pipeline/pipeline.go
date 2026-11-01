// Package pipeline hosts the shared orchestration workflows (analysis,
// auto-tagging, similarity search, clustering) so the CLI, the embedded web
// application, and library callers run one implementation instead of each
// re-implementing batch loops, decode caching, and persistence rules.
package pipeline

import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"

	"github.com/darkliquid/zounds/pkg/analysis"
	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/audio/codecs"
	"github.com/darkliquid/zounds/pkg/core"
)

// AnalysisOutput is the result of the full analyzer suite for one sample.
type AnalysisOutput struct {
	Results []core.AnalysisResult
	Vector  core.FeatureVector
}

// NewCodecRegistry builds the standard decoder registry used by all
// workflows.
func NewCodecRegistry() (*zaudio.Registry, error) {
	return codecs.NewRegistry()
}

// DefaultAnalyzers constructs the analyzer suite run by zounds analyze.
// Each analyzer shares the decoded sample attached to the context by
// AnalyzeSample.
func DefaultAnalyzers(registry *zaudio.Registry) ([]core.Analyzer, error) {
	factory := []func() (core.Analyzer, error){
		func() (core.Analyzer, error) { return analysis.NewMetadataAnalyzer(registry) },
		func() (core.Analyzer, error) { return analysis.NewSpectralAnalyzer(registry) },
		func() (core.Analyzer, error) { return analysis.NewKeyAnalyzer(registry) },
		func() (core.Analyzer, error) { return analysis.NewBeatAnalyzer(registry) },
		func() (core.Analyzer, error) { return analysis.NewPitchAnalyzer(registry) },
		func() (core.Analyzer, error) { return analysis.NewLoudnessAnalyzer(registry) },
		func() (core.Analyzer, error) { return analysis.NewDynamicsAnalyzer(registry) },
		func() (core.Analyzer, error) { return analysis.NewHPSSAnalyzer(registry) },
		func() (core.Analyzer, error) { return analysis.NewQualityAnalyzer(registry) },
		func() (core.Analyzer, error) { return analysis.NewHarmonicsAnalyzer(registry) },
		func() (core.Analyzer, error) { return analysis.NewFormantAnalyzer(registry) },
		func() (core.Analyzer, error) { return analysis.NewSpliceAnalyzer(registry) },
		func() (core.Analyzer, error) { return analysis.NewMFCCAnalyzer(registry) },
		func() (core.Analyzer, error) { return analysis.NewLoopAnalyzer(registry) },
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

// AnalyzeSample decodes the sample once, runs the default analyzer suite
// concurrently over the shared decode, and builds the 95-D feature vector.
func AnalyzeSample(ctx context.Context, sample core.Sample) (AnalysisOutput, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	registry, err := NewCodecRegistry()
	if err != nil {
		return AnalysisOutput{}, fmt.Errorf("create codec registry: %w", err)
	}

	decoded, err := zaudio.DecodeFile(ctx, registry, sample.Path)
	if err != nil {
		return AnalysisOutput{}, fmt.Errorf("decode sample %q: %w", sample.Path, err)
	}
	ctx = analysis.ContextWithDecodedSample(ctx, sample.Path, decoded)

	analyzers, err := DefaultAnalyzers(registry)
	if err != nil {
		return AnalysisOutput{}, err
	}

	results, err := RunAnalyzers(ctx, sample, analyzers, runtime.GOMAXPROCS(0))
	if err != nil {
		return AnalysisOutput{}, err
	}

	builder := analysis.NewFeatureVectorBuilder(nil)
	vector, err := builder.Build(sample.ID, results...)
	if err != nil {
		return AnalysisOutput{}, err
	}

	return AnalysisOutput{Results: results, Vector: vector}, nil
}

// RunAnalyzers executes analyzers concurrently (bounded by workers) while
// preserving input order, converting panics into errors.
func RunAnalyzers(ctx context.Context, sample core.Sample, analyzers []core.Analyzer, workers int) ([]core.AnalysisResult, error) {
	results := make([]core.AnalysisResult, len(analyzers))
	errs := make([]error, len(analyzers))

	if workers < 1 {
		workers = 1
	}
	if workers > len(analyzers) {
		workers = len(analyzers)
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i, analyzer := range analyzers {
		wg.Add(1)
		sem <- struct{}{}
		go func(index int, analyzer core.Analyzer) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if recovered := recover(); recovered != nil {
					errs[index] = fmt.Errorf("%s analysis panicked: %v; stack=%s", analyzer.Name(), recovered, debug.Stack())
				}
			}()

			result, err := analyzer.Analyze(ctx, sample)
			if err != nil {
				errs[index] = fmt.Errorf("%s analysis failed: %w", analyzer.Name(), err)
				return
			}
			results[index] = result
		}(i, analyzer)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return results, nil
}

// CombineAnalysisResults merges analyzer results into one result with
// flattened metrics and merged attributes.
func CombineAnalysisResults(sampleID int64, results ...core.AnalysisResult) core.AnalysisResult {
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

// DedupeGeneratedTags removes duplicate tag names (by normalized name)
// while preserving first-seen order.
func DedupeGeneratedTags(tags []core.Tag) []core.Tag {
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

// TagNames returns the normalized names of the given tags.
func TagNames(tags []core.Tag) []string {
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		names = append(names, tag.NormalizedName)
	}
	return names
}
