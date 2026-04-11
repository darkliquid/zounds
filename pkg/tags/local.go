package tags

import (
	"context"
	"fmt"
	"math"
	"slices"

	"github.com/darkliquid/zounds/pkg/core"
)

const localTaggerVersion = "0.1.0"

type TrainingExample struct {
	Vector core.FeatureVector
	Tags   []core.Tag
}

type LocalModelTagger struct {
	k            int
	minScore     float64
	maxPredicted int
	examples     []TrainingExample
}

func NewLocalModelTagger(examples []TrainingExample, k int, minScore float64, maxPredicted int) LocalModelTagger {
	if k <= 0 {
		k = 3
	}
	if minScore <= 0 {
		minScore = 0.34
	}
	if maxPredicted <= 0 {
		maxPredicted = 5
	}

	cloned := make([]TrainingExample, 0, len(examples))
	for _, example := range examples {
		cloned = append(cloned, TrainingExample{
			Vector: example.Vector,
			Tags:   slices.Clone(example.Tags),
		})
	}

	return LocalModelTagger{
		k:            k,
		minScore:     minScore,
		maxPredicted: maxPredicted,
		examples:     cloned,
	}
}

func (LocalModelTagger) Name() string {
	return "local"
}

func (LocalModelTagger) Version() string {
	return localTaggerVersion
}

func (t LocalModelTagger) Tags(_ context.Context, _ core.Sample, result core.AnalysisResult) ([]core.Tag, error) {
	if len(t.examples) == 0 {
		return nil, fmt.Errorf("local tagger has no training examples")
	}
	if len(result.FeatureVectors) == 0 {
		return nil, fmt.Errorf("local tagger requires at least one feature vector")
	}

	query := result.FeatureVectors[0]
	type neighbor struct {
		tags   []core.Tag
		weight float64
	}

	neighbors := make([]neighbor, 0, len(t.examples))
	var totalWeight float64
	for _, example := range t.examples {
		distance, err := euclideanDistance(query.Values, example.Vector.Values)
		if err != nil {
			return nil, err
		}
		weight := 1 / (1 + distance)
		neighbors = append(neighbors, neighbor{tags: example.Tags, weight: weight})
		totalWeight += weight
	}

	slices.SortFunc(neighbors, func(left, right neighbor) int {
		switch {
		case left.weight > right.weight:
			return -1
		case left.weight < right.weight:
			return 1
		default:
			return 0
		}
	})

	if len(neighbors) > t.k {
		neighbors = neighbors[:t.k]
		totalWeight = 0
		for _, neighbor := range neighbors {
			totalWeight += neighbor.weight
		}
	}

	scores := map[string]float64{}
	for _, neighbor := range neighbors {
		for _, tag := range neighbor.tags {
			name := tag.NormalizedName
			if name == "" {
				name = core.NormalizeTagName(tag.Name)
			}
			if name == "" {
				continue
			}
			scores[name] += neighbor.weight
		}
	}

	type scoredTag struct {
		name  string
		score float64
	}

	ranked := make([]scoredTag, 0, len(scores))
	for name, score := range scores {
		normalizedScore := score
		if totalWeight > 0 {
			normalizedScore = score / totalWeight
		}
		if normalizedScore < t.minScore {
			continue
		}
		ranked = append(ranked, scoredTag{name: name, score: normalizedScore})
	}

	slices.SortFunc(ranked, func(left, right scoredTag) int {
		switch {
		case left.score > right.score:
			return -1
		case left.score < right.score:
			return 1
		case left.name < right.name:
			return -1
		case left.name > right.name:
			return 1
		default:
			return 0
		}
	})

	if len(ranked) > t.maxPredicted {
		ranked = ranked[:t.maxPredicted]
	}

	out := make([]core.Tag, 0, len(ranked))
	for _, candidate := range ranked {
		out = append(out, core.Tag{
			Name:           candidate.name,
			NormalizedName: candidate.name,
			Source:         "local",
			Confidence:     candidate.score,
		})
	}
	return out, nil
}

func euclideanDistance(left, right []float64) (float64, error) {
	if len(left) == 0 || len(right) == 0 {
		return 0, fmt.Errorf("feature vectors must not be empty")
	}
	if len(left) != len(right) {
		return 0, fmt.Errorf("feature vector dimension mismatch: %d != %d", len(left), len(right))
	}

	var sum float64
	for i := range left {
		delta := left[i] - right[i]
		sum += delta * delta
	}
	return math.Sqrt(sum), nil
}
