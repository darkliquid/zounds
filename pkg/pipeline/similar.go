package pipeline

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/darkliquid/zounds/pkg/analysis"
	"github.com/darkliquid/zounds/pkg/cluster"
	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/db"
)

// SimilarMatch is a candidate sample matched against a reference sample.
type SimilarMatch struct {
	Sample      core.Sample
	Reference   core.Sample
	Similarity  float64
	Distance    float64
	ReferenceID int64
}

// SimilarOptions controls a similarity search.
type SimilarOptions struct {
	// Namespace selects the feature-vector namespace to search over (default "acoustic").
	Namespace string
	// Metric selects the metric space: "acoustic", "clap", "timbre", "harmonic",
	// "rhythm", "perceptual", "hybrid".
	Metric string
	// Threshold is the minimum cosine similarity required for a match.
	Threshold float64
	// Order sorts matches by similarity: "asc" or "desc" (default "desc").
	Order string
	// Limit caps the number of matches returned (0 for unlimited).
	Limit int
}

type similarReference struct {
	Sample core.Sample
	Vector core.FeatureVector
}

var errReferenceNotFound = fmt.Errorf("reference not found")

// FindSimilar runs a multi-reference cosine similarity search over stored
// feature vectors.
func FindSimilar(ctx context.Context, repo *db.Repository, references []string, opts SimilarOptions) ([]SimilarMatch, error) {
	order := opts.Order
	if order == "" {
		order = "desc"
	}
	if opts.Limit < 0 {
		return nil, fmt.Errorf("limit must be greater than or equal to 0")
	}
	if opts.Threshold < -1 || opts.Threshold > 1 {
		return nil, fmt.Errorf("threshold must be between -1 and 1")
	}
	if order != "asc" && order != "desc" {
		return nil, fmt.Errorf("unsupported order %q", order)
	}

	samples, err := repo.ListSamples(ctx)
	if err != nil {
		return nil, err
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("no indexed samples found")
	}

	vectors, err := LoadMetricVectors(ctx, repo, opts.Metric, opts.Namespace)
	if err != nil {
		return nil, err
	}

	refs, err := resolveSimilarReferences(references, samples, vectors)
	if err != nil {
		return nil, err
	}

	matches, err := findSimilarMatches(refs, samples, vectors, opts.Threshold, order)
	if err != nil {
		return nil, err
	}
	if opts.Limit > 0 && len(matches) > opts.Limit {
		matches = matches[:opts.Limit]
	}

	return matches, nil
}

// LoadMetricVectors loads, resolves, and standardizes feature vectors according to
// the requested metric ("acoustic", "clap", "timbre", "harmonic", "rhythm",
// "perceptual", "hybrid") or custom namespace.
func LoadMetricVectors(ctx context.Context, repo *db.Repository, metric string, customNamespace string) ([]core.FeatureVector, error) {
	if metric == "" && customNamespace != "" {
		metric = customNamespace
	}
	if metric == "" {
		ns := customNamespace
		if ns == "" {
			ns = "analysis"
		}
		vectors, err := repo.ListFeatureVectors(ctx, ns)
		if err != nil {
			return nil, err
		}
		if len(vectors) == 0 {
			return nil, fmt.Errorf("no %s feature vectors found; run zounds analyze first", ns)
		}
		return vectors, nil
	}

	metric = strings.ToLower(metric)

	switch metric {
	case "clap":
		vectors, err := repo.ListFeatureVectors(ctx, "clap")
		if err != nil {
			return nil, err
		}
		if len(vectors) == 0 {
			return nil, fmt.Errorf("no clap feature vectors found; run zounds analyze --clap-model-dir first")
		}
		return vectors, nil

	case "acoustic", "timbre", "harmonic", "rhythm":
		vectors, err := repo.ListFeatureVectors(ctx, metric)
		if err == nil && len(vectors) > 0 {
			return cluster.StandardizeVectors(vectors, cluster.StandardizeIQR), nil
		}

		rawVectors, err := repo.ListFeatureVectors(ctx, "analysis")
		if err != nil {
			return nil, err
		}
		if len(rawVectors) == 0 {
			return nil, fmt.Errorf("no %s feature vectors found; run zounds analyze first", metric)
		}

		subspaceVectors := make([]core.FeatureVector, 0, len(rawVectors))
		for _, raw := range rawVectors {
			sub := projectSubspace(raw, metric)
			subspaceVectors = append(subspaceVectors, sub)
		}

		return cluster.StandardizeVectors(subspaceVectors, cluster.StandardizeIQR), nil

	case "perceptual":
		vectors, err := repo.ListFeatureVectors(ctx, "perceptual")
		if err == nil && len(vectors) > 0 {
			return vectors, nil
		}
		storedHashes, err := repo.ListPerceptualHashes(ctx, "spectral-8x8")
		if err != nil {
			return nil, err
		}
		if len(storedHashes) == 0 {
			return nil, fmt.Errorf("no perceptual feature vectors found; run zounds analyze first")
		}
		vectors = make([]core.FeatureVector, 0, len(storedHashes))
		for _, h := range storedHashes {
			if h.Hash == "" {
				continue
			}
			var val uint64
			if _, err := fmt.Sscanf(h.Hash, "%016x", &val); err == nil {
				bits64 := make([]float64, 64)
				for b := 0; b < 64; b++ {
					if (val & (uint64(1) << uint(b))) != 0 {
						bits64[b] = 1.0
					}
				}
				vectors = append(vectors, core.FeatureVector{
					SampleID:   h.SampleID,
					Namespace:  "perceptual",
					Dimensions: 64,
					Values:     bits64,
				})
			}
		}
		if len(vectors) == 0 {
			return nil, fmt.Errorf("no perceptual feature vectors found; run zounds analyze first")
		}
		return vectors, nil

	case "hybrid":
		// Combine semantic CLAP and standardized acoustic representations
		acousticVectors, err := LoadMetricVectors(ctx, repo, "acoustic", "")
		if err != nil {
			return nil, err
		}
		clapVectors, err := repo.ListFeatureVectors(ctx, "clap")
		if err != nil || len(clapVectors) == 0 {
			return acousticVectors, nil
		}

		clapBySample := make(map[int64][]float64, len(clapVectors))
		for _, cv := range clapVectors {
			clapBySample[cv.SampleID] = cv.Values
		}

		hybridVectors := make([]core.FeatureVector, 0, len(acousticVectors))
		scale := 1.0 / math.Sqrt(2.0)
		for _, av := range acousticVectors {
			normAcoustic := l2Normalize(av.Values)
			if clapVals, ok := clapBySample[av.SampleID]; ok {
				normClap := l2Normalize(clapVals)
				combined := make([]float64, len(normClap)+len(normAcoustic))
				for idx, val := range normClap {
					combined[idx] = val * scale
				}
				offset := len(normClap)
				for idx, val := range normAcoustic {
					combined[offset+idx] = val * scale
				}
				hybridVectors = append(hybridVectors, core.FeatureVector{
					SampleID:   av.SampleID,
					Namespace:  "hybrid",
					Dimensions: len(combined),
					Values:     combined,
				})
			} else {
				hybridVectors = append(hybridVectors, av)
			}
		}
		return hybridVectors, nil

	default:
		vectors, err := repo.ListFeatureVectors(ctx, metric)
		if err != nil {
			return nil, err
		}
		if len(vectors) == 0 {
			return nil, fmt.Errorf("no %s feature vectors found; run zounds analyze first", metric)
		}
		return vectors, nil
	}
}

var (
	subspaceIndicesOnce sync.Once
	timbreIndices       []int
	harmonicIndices     []int
	rhythmIndices       []int
)

func initSubspaceIndices() {
	allNames := analysis.FeatureNames()
	indexMap := make(map[string]int, len(allNames))
	for i, name := range allNames {
		indexMap[name] = i
	}

	resolve := func(names []string) []int {
		idxs := make([]int, 0, len(names))
		for _, name := range names {
			if idx, ok := indexMap[name]; ok {
				idxs = append(idxs, idx)
			}
		}
		return idxs
	}

	timbreIndices = resolve(analysis.TimbreFeatureNames())
	harmonicIndices = resolve(analysis.HarmonicFeatureNames())
	rhythmIndices = resolve(analysis.RhythmFeatureNames())
}

func projectSubspace(raw core.FeatureVector, subspace string) core.FeatureVector {
	subspaceIndicesOnce.Do(initSubspaceIndices)

	allNames := analysis.FeatureNames()
	if len(raw.Values) != len(allNames) {
		return raw
	}

	var targetIndices []int
	switch subspace {
	case "acoustic":
		// Indices 4..94: strictly excludes container metadata (sample_rate, channels, bit_depth, bitrate)
		targetIndices = make([]int, len(allNames)-4)
		for i := range targetIndices {
			targetIndices[i] = i + 4
		}
	case "timbre":
		targetIndices = timbreIndices
	case "harmonic":
		targetIndices = harmonicIndices
	case "rhythm":
		targetIndices = rhythmIndices
	default:
		return raw
	}

	vals := make([]float64, len(targetIndices))
	for i, idx := range targetIndices {
		vals[i] = raw.Values[idx]
	}

	return core.FeatureVector{
		ID:         raw.ID,
		SampleID:   raw.SampleID,
		Namespace:  subspace,
		Version:    raw.Version,
		Dimensions: len(vals),
		Values:     vals,
		CreatedAt:  raw.CreatedAt,
	}
}

func l2Normalize(vec []float64) []float64 {
	var sumSq float64
	for _, v := range vec {
		sumSq += v * v
	}
	norm := math.Sqrt(sumSq)
	res := make([]float64, len(vec))
	if norm > 1e-12 {
		for i, v := range vec {
			res[i] = v / norm
		}
	}
	return res
}

func resolveSimilarReferences(args []string, samples []core.Sample, vectors []core.FeatureVector) ([]similarReference, error) {
	sampleByID := make(map[int64]core.Sample, len(samples))
	sampleByPath := make(map[string]core.Sample, len(samples))
	samplesByRelativePath := make(map[string][]core.Sample)
	samplesByFileName := make(map[string][]core.Sample)
	for _, sample := range samples {
		sampleByID[sample.ID] = sample
		sampleByPath[sample.Path] = sample
		samplesByRelativePath[sample.RelativePath] = append(samplesByRelativePath[sample.RelativePath], sample)
		samplesByFileName[sample.FileName] = append(samplesByFileName[sample.FileName], sample)
	}

	vectorBySampleID := make(map[int64]core.FeatureVector, len(vectors))
	for _, vector := range vectors {
		vectorBySampleID[vector.SampleID] = vector
	}

	references := make([]similarReference, 0, len(args))
	seen := make(map[int64]struct{}, len(args))
	for _, arg := range args {
		sample, err := resolveSimilarReference(arg, sampleByID, sampleByPath, samplesByRelativePath, samplesByFileName)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[sample.ID]; ok {
			continue
		}

		vector, ok := vectorBySampleID[sample.ID]
		if !ok {
			return nil, fmt.Errorf("sample %q has no analysis feature vector; run zounds analyze first", sample.Path)
		}

		references = append(references, similarReference{
			Sample: sample,
			Vector: vector,
		})
		seen[sample.ID] = struct{}{}
	}

	return references, nil
}

func resolveSimilarReference(
	arg string,
	sampleByID map[int64]core.Sample,
	sampleByPath map[string]core.Sample,
	samplesByRelativePath map[string][]core.Sample,
	samplesByFileName map[string][]core.Sample,
) (core.Sample, error) {
	if id, err := strconv.ParseInt(arg, 10, 64); err == nil {
		sample, ok := sampleByID[id]
		if !ok {
			return core.Sample{}, fmt.Errorf("sample id %d is not indexed", id)
		}
		return sample, nil
	}

	if sample, ok := sampleByPath[arg]; ok {
		return sample, nil
	}

	if sample, err := resolveUniqueSampleReference(arg, samplesByRelativePath, "relative path"); err == nil {
		return sample, nil
	} else if err != nil && !errors.Is(err, errReferenceNotFound) {
		return core.Sample{}, err
	}

	if sample, err := resolveUniqueSampleReference(arg, samplesByFileName, "file name"); err == nil {
		return sample, nil
	} else if err != nil && !errors.Is(err, errReferenceNotFound) {
		return core.Sample{}, err
	}

	return core.Sample{}, fmt.Errorf("no indexed sample matched %q", arg)
}

func resolveUniqueSampleReference(arg string, index map[string][]core.Sample, label string) (core.Sample, error) {
	matches := index[arg]
	switch len(matches) {
	case 0:
		return core.Sample{}, errReferenceNotFound
	case 1:
		return matches[0], nil
	default:
		return core.Sample{}, fmt.Errorf("reference %q matched multiple samples by %s; use an id or full path", arg, label)
	}
}

func findSimilarMatches(
	references []similarReference,
	samples []core.Sample,
	vectors []core.FeatureVector,
	threshold float64,
	order string,
) ([]SimilarMatch, error) {
	sampleByID := make(map[int64]core.Sample, len(samples))
	for _, sample := range samples {
		sampleByID[sample.ID] = sample
	}

	referenceIDs := make(map[int64]struct{}, len(references))
	for _, reference := range references {
		referenceIDs[reference.Sample.ID] = struct{}{}
	}

	matches := make([]SimilarMatch, 0, len(vectors))
	for _, candidate := range vectors {
		if _, ok := referenceIDs[candidate.SampleID]; ok {
			continue
		}

		sample, ok := sampleByID[candidate.SampleID]
		if !ok {
			return nil, fmt.Errorf("sample id %d referenced by analysis vector is not indexed", candidate.SampleID)
		}

		bestSimilarity := math.Inf(-1)
		var bestReference core.Sample
		for _, reference := range references {
			similarity, err := cluster.CosineSimilarity(reference.Vector.Values, candidate.Values)
			if err != nil {
				if strings.Contains(err.Error(), "zero-length vector norm") {
					similarity = 0
				} else {
					return nil, fmt.Errorf("compare %q to %q: %w", reference.Sample.Path, sample.Path, err)
				}
			}
			if similarity > bestSimilarity {
				bestSimilarity = similarity
				bestReference = reference.Sample
			}
		}

		if bestSimilarity < threshold {
			continue
		}

		matches = append(matches, SimilarMatch{
			Sample:      sample,
			Reference:   bestReference,
			ReferenceID: bestReference.ID,
			Similarity:  bestSimilarity,
			Distance:    1 - bestSimilarity,
		})
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Similarity == matches[j].Similarity {
			if matches[i].Sample.Path == matches[j].Sample.Path {
				return matches[i].Reference.Path < matches[j].Reference.Path
			}
			return matches[i].Sample.Path < matches[j].Sample.Path
		}
		if order == "asc" {
			return matches[i].Similarity < matches[j].Similarity
		}
		return matches[i].Similarity > matches[j].Similarity
	})

	return matches, nil
}
