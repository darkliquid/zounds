package pipeline

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/darkliquid/zounds/pkg/analysis"
	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/clap"
	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/db"
)

// AnalyzeOptions controls a batch analysis run.
type AnalyzeOptions struct {
	// DryRun runs analysis but skips persistence.
	DryRun bool
	// SkipUndecodable logs a warning and continues when a sample cannot be
	// decoded instead of aborting the batch.
	SkipUndecodable bool
	// Concurrency sets how many samples are analyzed in parallel
	// (<= 0 means 1, sequential processing).
	Concurrency int
	// Logger receives verbose per-sample progress lines when non-nil.
	Logger *log.Logger
	// OnSkip receives samples that were skipped due to decode errors when
	// SkipUndecodable is set.
	OnSkip func(sample core.Sample, err error)
	// SkipUnchanged skips samples whose stored analysis already matches the
	// current file modification time.
	SkipUnchanged bool
	// OnUnchanged receives samples skipped because their analysis is current
	// when SkipUnchanged is set.
	OnUnchanged func(sample core.Sample)
	// CLAPModelDir specifies the directory containing CLAP ONNX models and tokenizer.json.
	// When provided, 512-D CLAP embeddings are extracted and persisted under namespace "clap".
	CLAPModelDir string
	// CLAPLib specifies the path to the ONNX Runtime shared library (empty uses default).
	CLAPLib string
	// CLAPModel allows passing a pre-loaded CLAP model directly.
	CLAPModel *clap.Model
}

// AnalyzeSummary reports the outcome of a batch analysis run.
type AnalyzeSummary struct {
	Processed int
	Skipped   int
	Unchanged int
}

// AnalyzeSamples runs the analyzer suite over samples, persisting the
// feature vector and rich sample analysis per sample unless DryRun is set.
func AnalyzeSamples(ctx context.Context, repo *db.Repository, samples []core.Sample, opts AnalyzeOptions) (AnalyzeSummary, error) {
	if repo == nil {
		return AnalyzeSummary{}, errors.New("pipeline: nil repository")
	}

	var clapModel *clap.Model
	if opts.CLAPModel != nil {
		clapModel = opts.CLAPModel
	} else if strings.TrimSpace(opts.CLAPModelDir) != "" {
		m, err := loadCLAPModel(opts.CLAPModelDir, opts.CLAPLib)
		if err != nil {
			return AnalyzeSummary{}, fmt.Errorf("load CLAP model: %w", err)
		}
		clapModel = m
		defer func() { _ = clapModel.Close() }()
	}
	opts.CLAPModel = clapModel

	unchanged := 0
	if opts.SkipUnchanged {
		var filtered []core.Sample
		for _, sample := range samples {
			current, err := analysisIsCurrent(ctx, repo, sample)
			if err != nil {
				return AnalyzeSummary{}, err
			}
			if current {
				unchanged++
				verbosef(opts.Logger, "skipping unchanged sample %s", sample.Path)
				if opts.OnUnchanged != nil {
					opts.OnUnchanged(sample)
				}
				continue
			}
			filtered = append(filtered, sample)
		}
		samples = filtered
	}

	summary, err := analyzeSamplesWorkerPool(ctx, repo, samples, opts)
	summary.Unchanged = unchanged
	return summary, err
}

// analysisIsCurrent reports whether the stored analysis row matches the
// sample's current file modification time. Samples without a recorded
// modification time are never treated as unchanged.
func analysisIsCurrent(ctx context.Context, repo *db.Repository, sample core.Sample) (bool, error) {
	if sample.ModifiedAt.IsZero() {
		return false, nil
	}

	stored, err := repo.GetSampleAnalysis(ctx, sample.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	return stored.AnalyzedModifiedAt.Equal(sample.ModifiedAt), nil
}

func analyzeSamplesWorkerPool(ctx context.Context, repo *db.Repository, samples []core.Sample, opts AnalyzeOptions) (AnalyzeSummary, error) {
	concurrency := opts.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > len(samples) {
		concurrency = len(samples)
	}

	var (
		summary   AnalyzeSummary
		summaryMu sync.Mutex
		firstErr  error
		errMu     sync.Mutex
	)

	setErr := func(err error) {
		errMu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		errMu.Unlock()
	}

	jobs := make(chan core.Sample)
	var wg sync.WaitGroup
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for sample := range jobs {
				verbosef(opts.Logger, "analyzing sample %s", sample.Path)
				output, err := AnalyzeSampleWithCLAP(ctx, sample, opts.CLAPModel)
				if err != nil {
					if opts.SkipUndecodable && zaudio.IsDecodeError(err) {
						summaryMu.Lock()
						summary.Skipped++
						summaryMu.Unlock()
						if opts.OnSkip != nil {
							opts.OnSkip(sample, err)
						}
						continue
					}
					setErr(err)
					return
				}

				if opts.DryRun {
					verbosef(opts.Logger, "skipping persistence for %s (dry run)", sample.Path)
				} else {
					verbosef(opts.Logger, "persisting feature vector for %s", sample.Path)
					if err := PersistAnalysisOutput(ctx, repo, sample, output); err != nil {
						setErr(err)
						return
					}
				}

				summaryMu.Lock()
				summary.Processed++
				summaryMu.Unlock()
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

// PersistAnalysisOutput stores the primary feature vector, any extra feature
// vectors (such as 512-D CLAP embeddings), and the flattened rich analysis
// (metrics and attributes) for a sample.
func PersistAnalysisOutput(ctx context.Context, repo *db.Repository, sample core.Sample, output AnalysisOutput) error {
	if _, err := repo.ReplaceFeatureVector(ctx, output.Vector); err != nil {
		return err
	}
	for _, fv := range output.FeatureVectors {
		if _, err := repo.ReplaceFeatureVector(ctx, fv); err != nil {
			return err
		}
	}

	combined := CombineAnalysisResults(sample.ID, output.Results...)
	if err := repo.UpsertSampleAnalysis(ctx, SampleAnalysisFromResult(combined, sample.ModifiedAt)); err != nil {
		return err
	}

	for _, res := range output.Results {
		if res.Analyzer == "perceptual_hash" {
			hashVal := res.Attributes["perceptual_hash"]
			algo := res.Attributes["hash_algorithm"]
			bits := int(res.Metrics["hash_bits"])
			if hashVal != "" && algo != "" {
				if err := repo.UpsertPerceptualHash(ctx, core.PerceptualHash{
					SampleID:  sample.ID,
					Algorithm: algo,
					Hash:      hashVal,
					HashBits:  bits,
				}); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func loadCLAPModel(modelDir, libPath string) (*clap.Model, error) {
	cfg := clap.DefaultConfig()
	if libPath != "" {
		cfg.LibPath = libPath
	}
	return clap.NewModel(modelDir, cfg)
}

// AnalyzeFileInfo runs the full analyzer suite against an arbitrary file on
// disk and returns a JSON-friendly report.
func AnalyzeFileInfo(ctx context.Context, path string) (map[string]any, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	sample := core.Sample{
		Path:      path,
		Extension: ExtensionFromPath(path),
		Format:    core.DetectFormatFromExtension(path),
		SizeBytes: info.Size(),
	}

	output, err := AnalyzeSample(ctx, sample)
	if err != nil {
		return nil, err
	}

	report := map[string]any{
		"path":           path,
		"format":         sample.Format,
		"feature_names":  analysis.FeatureNames(),
		"feature_vector": output.Vector.Values,
		"analyzers":      map[string]map[string]any{},
	}

	analyzers := report["analyzers"].(map[string]map[string]any)
	for _, result := range output.Results {
		analyzers[result.Analyzer] = map[string]any{
			"metrics":    result.Metrics,
			"attributes": result.Attributes,
		}
	}

	return report, nil
}

// SelectSamplesForAnalysis resolves the analyze command target selection:
// all indexed samples or a single sample by id.
func SelectSamplesForAnalysis(ctx context.Context, repo *db.Repository, all bool, id int64) ([]core.Sample, error) {
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

// SelectSamplesByPath resolves a single indexed sample by path, or lists all
// indexed samples when targetPath is empty.
func SelectSamplesByPath(ctx context.Context, repo *db.Repository, targetPath string) ([]core.Sample, error) {
	if targetPath != "" {
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

// ExtensionFromPath returns the canonical extension for a path based on its
// detected audio format.
func ExtensionFromPath(path string) string {
	format := core.DetectFormatFromExtension(path)
	if format == core.FormatUnknown {
		return ""
	}
	return string(format)
}

func verbosef(logger *log.Logger, format string, args ...any) {
	if logger != nil {
		logger.Printf(format, args...)
	}
}
