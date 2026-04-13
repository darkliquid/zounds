package commands

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/darkliquid/zounds/pkg/analysis"
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

			builder := analysis.NewFeatureVectorBuilder(nil)
			processed := 0
			for _, sample := range samples {
				results, vector, err := analyzeSample(ctx, sample, builder)
				if err != nil {
					return err
				}
				_ = results

				if !cfg.DryRun {
					if _, err := repo.ReplaceFeatureVector(ctx, vector); err != nil {
						return err
					}
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
	analyzers := []core.Analyzer{}
	factory := []func() (core.Analyzer, error){
		func() (core.Analyzer, error) { return analysis.NewMetadataAnalyzer(nil) },
		func() (core.Analyzer, error) { return analysis.NewSpectralAnalyzer(nil) },
		func() (core.Analyzer, error) { return analysis.NewKeyAnalyzer(nil) },
		func() (core.Analyzer, error) { return analysis.NewBeatAnalyzer(nil) },
		func() (core.Analyzer, error) { return analysis.NewPitchAnalyzer(nil) },
		func() (core.Analyzer, error) { return analysis.NewLoudnessAnalyzer(nil) },
		func() (core.Analyzer, error) { return analysis.NewDynamicsAnalyzer(nil) },
		func() (core.Analyzer, error) { return analysis.NewHPSSAnalyzer(nil) },
		func() (core.Analyzer, error) { return analysis.NewQualityAnalyzer(nil) },
		func() (core.Analyzer, error) { return analysis.NewHarmonicsAnalyzer(nil) },
		func() (core.Analyzer, error) { return analysis.NewFormantAnalyzer(nil) },
		func() (core.Analyzer, error) { return analysis.NewSpliceAnalyzer(nil) },
		func() (core.Analyzer, error) { return analysis.NewMFCCAnalyzer(nil) },
		func() (core.Analyzer, error) { return analysis.NewLoopAnalyzer(nil) },
	}

	for _, create := range factory {
		analyzer, err := create()
		if err != nil {
			return nil, core.FeatureVector{}, err
		}
		analyzers = append(analyzers, analyzer)
	}

	results := make([]core.AnalysisResult, 0, len(analyzers))
	for _, analyzer := range analyzers {
		result, err := analyzer.Analyze(ctx, sample)
		if err != nil {
			return nil, core.FeatureVector{}, fmt.Errorf("%s analysis failed: %w", analyzer.Name(), err)
		}
		results = append(results, result)
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
