package commands

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"sync"

	"github.com/spf13/cobra"

	"github.com/darkliquid/zounds/pkg/analysis"
	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/audio/codecs"
	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/db"
)

func newAnalyzeCommand(cfg *Config) *cobra.Command {
	var (
		analyzeAll bool
		id         int64
	)

	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Run analyzers against indexed samples",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !analyzeAll && id == 0 {
				return fmt.Errorf("use --all or --id")
			}

			ctx := cmd.Context()
			repo, closer, err := openRepository(ctx, cfg)
			if err != nil {
				return err
			}
			defer func() { _ = closer.Close() }()

			samples, err := selectSamplesForAnalysis(ctx, repo, analyzeAll, id)
			if err != nil {
				return err
			}

			logger := newVerboseLogger(cfg, cmd.ErrOrStderr())
			builder := analysis.NewFeatureVectorBuilder(nil)
			processed := 0
			for _, sample := range samples {
				verbosef(logger, "analyzing sample %s", sample.Path)
				results, vector, err := analyzeSample(ctx, sample, builder)
				if err != nil {
					return err
				}
				_ = results

				if !cfg.DryRun {
					verbosef(logger, "persisting feature vector for %s", sample.Path)
					if _, err := repo.ReplaceFeatureVector(ctx, vector); err != nil {
						return err
					}
				} else {
					verbosef(logger, "skipping feature vector persistence for %s (dry run)", sample.Path)
				}
				processed++
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "analyzed %d samples\n", processed)
			return err
		},
	}

	flags := cmd.Flags()
	flags.BoolVar(&analyzeAll, "all", false, "analyze all indexed samples")
	flags.Int64Var(&id, "id", 0, "analyze a specific sample by id")

	return cmd
}

func newInfoCommand(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info <file>",
		Short: "Show detailed information about a sample",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			info, err := analyzeFileInfo(cmd.Context(), path)
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

func selectSamplesForAnalysis(ctx context.Context, repo *db.Repository, all bool, id int64) ([]core.Sample, error) {
	if all {
		return repo.ListSamples(ctx)
	}

	sample, err := repo.FindSampleByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("sample id %d is not indexed", id)
		}
		return nil, err
	}

	return []core.Sample{sample}, nil
}

func analyzeSample(ctx context.Context, sample core.Sample, builder *analysis.FeatureVectorBuilder) ([]core.AnalysisResult, core.FeatureVector, error) {
	registry, err := codecs.NewRegistry()
	if err != nil {
		return nil, core.FeatureVector{}, fmt.Errorf("create codec registry: %w", err)
	}
	decoded, err := zaudio.DecodeFile(ctx, registry, sample.Path)
	if err != nil {
		return nil, core.FeatureVector{}, fmt.Errorf("decode sample %q: %w", sample.Path, err)
	}
	ctx = analysis.ContextWithDecodedSample(ctx, sample.Path, decoded)

	analyzers := []core.Analyzer{}
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

	for _, create := range factory {
		analyzer, err := create()
		if err != nil {
			return nil, core.FeatureVector{}, err
		}
		analyzers = append(analyzers, analyzer)
	}

	results := make([]core.AnalysisResult, len(analyzers))
	errs := make([]error, len(analyzers))
	workers := runtime.GOMAXPROCS(0)
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
					errs[index] = fmt.Errorf("%s analysis panicked: %v\n%s", analyzer.Name(), recovered, debug.Stack())
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
			return nil, core.FeatureVector{}, err
		}
	}

	vector, err := builder.Build(sample.ID, results...)
	if err != nil {
		return nil, core.FeatureVector{}, err
	}

	return results, vector, nil
}

func analyzeFileInfo(ctx context.Context, path string) (map[string]any, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	sample := core.Sample{
		Path:      path,
		Extension: extensionFromPath(path),
		Format:    core.DetectFormatFromExtension(path),
		SizeBytes: info.Size(),
	}

	builder := analysis.NewFeatureVectorBuilder(nil)
	results, vector, err := analyzeSample(ctx, sample, builder)
	if err != nil {
		return nil, err
	}

	output := map[string]any{
		"path":           path,
		"format":         sample.Format,
		"feature_names":  analysis.FeatureNames(),
		"feature_vector": vector.Values,
		"analyzers":      map[string]map[string]any{},
	}

	analyzers := output["analyzers"].(map[string]map[string]any)
	for _, result := range results {
		analyzers[result.Analyzer] = map[string]any{
			"metrics":    result.Metrics,
			"attributes": result.Attributes,
		}
	}

	return output, nil
}

func extensionFromPath(path string) string {
	format := core.DetectFormatFromExtension(path)
	switch format {
	case core.FormatUnknown:
		return ""
	default:
		return string(format)
	}
}
