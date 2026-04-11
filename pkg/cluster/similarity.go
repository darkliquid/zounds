package cluster

import (
	"fmt"
	"math"
	"sort"

	"github.com/darkliquid/zounds/pkg/core"
)

type SimilarityMetric string

const (
	MetricCosine    SimilarityMetric = "cosine"
	MetricEuclidean SimilarityMetric = "euclidean"
)

type PairSimilarity struct {
	LeftSampleID  int64
	RightSampleID int64
	Similarity    float64
	Distance      float64
}

type Neighbor struct {
	Vector     core.FeatureVector
	Similarity float64
	Distance   float64
}

func CosineSimilarity(left, right []float64) (float64, error) {
	if len(left) == 0 || len(right) == 0 {
		return 0, fmt.Errorf("cosine similarity: vectors must not be empty")
	}
	if len(left) != len(right) {
		return 0, fmt.Errorf("cosine similarity: dimension mismatch %d != %d", len(left), len(right))
	}

	var dot, leftNorm, rightNorm float64
	for i := range left {
		dot += left[i] * right[i]
		leftNorm += left[i] * left[i]
		rightNorm += right[i] * right[i]
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0, fmt.Errorf("cosine similarity: zero-length vector norm")
	}

	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm)), nil
}

func EuclideanDistance(left, right []float64) (float64, error) {
	if len(left) == 0 || len(right) == 0 {
		return 0, fmt.Errorf("euclidean distance: vectors must not be empty")
	}
	if len(left) != len(right) {
		return 0, fmt.Errorf("euclidean distance: dimension mismatch %d != %d", len(left), len(right))
	}

	var sum float64
	for i := range left {
		diff := left[i] - right[i]
		sum += diff * diff
	}
	return math.Sqrt(sum), nil
}

func Pairwise(vectors []core.FeatureVector, metric SimilarityMetric) ([]PairSimilarity, error) {
	pairs := make([]PairSimilarity, 0, maxPairCount(len(vectors)))
	for i := 0; i < len(vectors); i++ {
		for j := i + 1; j < len(vectors); j++ {
			similarity, distance, err := compareVectors(vectors[i], vectors[j], metric)
			if err != nil {
				return nil, err
			}
			pairs = append(pairs, PairSimilarity{
				LeftSampleID:  vectors[i].SampleID,
				RightSampleID: vectors[j].SampleID,
				Similarity:    similarity,
				Distance:      distance,
			})
		}
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Similarity == pairs[j].Similarity {
			if pairs[i].LeftSampleID == pairs[j].LeftSampleID {
				return pairs[i].RightSampleID < pairs[j].RightSampleID
			}
			return pairs[i].LeftSampleID < pairs[j].LeftSampleID
		}
		return pairs[i].Similarity > pairs[j].Similarity
	})

	return pairs, nil
}

func NearestNeighbors(target core.FeatureVector, candidates []core.FeatureVector, limit int, metric SimilarityMetric) ([]Neighbor, error) {
	if limit <= 0 || len(candidates) == 0 {
		return nil, nil
	}

	neighbors := make([]Neighbor, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.SampleID == target.SampleID {
			continue
		}

		similarity, distance, err := compareVectors(target, candidate, metric)
		if err != nil {
			return nil, err
		}
		neighbors = append(neighbors, Neighbor{
			Vector:     candidate,
			Similarity: similarity,
			Distance:   distance,
		})
	}

	sort.Slice(neighbors, func(i, j int) bool {
		if neighbors[i].Similarity == neighbors[j].Similarity {
			return neighbors[i].Vector.SampleID < neighbors[j].Vector.SampleID
		}
		return neighbors[i].Similarity > neighbors[j].Similarity
	})

	if limit > len(neighbors) {
		limit = len(neighbors)
	}
	return neighbors[:limit], nil
}

func compareVectors(left, right core.FeatureVector, metric SimilarityMetric) (float64, float64, error) {
	switch metric {
	case "", MetricCosine:
		similarity, err := CosineSimilarity(left.Values, right.Values)
		if err != nil {
			return 0, 0, err
		}
		return similarity, 1 - similarity, nil
	case MetricEuclidean:
		distance, err := EuclideanDistance(left.Values, right.Values)
		if err != nil {
			return 0, 0, err
		}
		return 1 / (1 + distance), distance, nil
	default:
		return 0, 0, fmt.Errorf("compare vectors: unsupported metric %q", metric)
	}
}

func maxPairCount(n int) int {
	if n < 2 {
		return 0
	}
	return (n * (n - 1)) / 2
}
