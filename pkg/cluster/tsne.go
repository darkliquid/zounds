package cluster

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/darkliquid/zounds/pkg/core"
)

type TSNEOptions struct {
	Perplexity        float64
	Iterations        int
	LearningRate      float64
	EarlyExaggeration float64
	Seed              int64
}

// ProjectTSNE2D implements exact t-SNE following van der Maaten and Hinton
// (2008), "Visualizing Data using t-SNE", for moderate-sized sample libraries.
func ProjectTSNE2D(vectors []core.FeatureVector, options TSNEOptions) ([]ProjectionPoint, error) {
	if len(vectors) == 0 {
		return nil, nil
	}
	if err := validateProjectionVectors(vectors); err != nil {
		return nil, err
	}

	if options.Perplexity <= 0 {
		options.Perplexity = 20
	}
	if options.Iterations <= 0 {
		options.Iterations = 500
	}
	if options.LearningRate <= 0 {
		options.LearningRate = 200
	}
	if options.EarlyExaggeration <= 0 {
		options.EarlyExaggeration = 4
	}
	if options.Seed == 0 {
		options.Seed = 1337
	}

	centered := centerVectors(vectors)
	distances := pairwiseSquaredDistances(centered)
	p := symmetricAffinities(distances, options.Perplexity)

	n := len(vectors)
	y := make([][2]float64, n)
	gains := make([][2]float64, n)
	velocity := make([][2]float64, n)
	rng := rand.New(rand.NewSource(options.Seed))
	for i := range y {
		y[i][0] = (rng.Float64() - 0.5) * 1e-3
		y[i][1] = (rng.Float64() - 0.5) * 1e-3
		gains[i][0], gains[i][1] = 1, 1
	}

	for iter := 0; iter < options.Iterations; iter++ {
		q, qsum := lowDimensionalAffinities(y)
		grads := make([][2]float64, n)
		exaggeration := 1.0
		if iter < options.Iterations/4 {
			exaggeration = options.EarlyExaggeration
		}

		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				if i == j {
					continue
				}
				pij := p[i][j] * exaggeration
				qij := q[i][j] / qsum
				diffX := y[i][0] - y[j][0]
				diffY := y[i][1] - y[j][1]
				multiplier := 4 * (pij - qij) * q[i][j]
				grads[i][0] += multiplier * diffX
				grads[i][1] += multiplier * diffY
			}
		}

		momentum := 0.5
		if iter >= options.Iterations/2 {
			momentum = 0.8
		}
		for i := range y {
			for dim := 0; dim < 2; dim++ {
				grad := grads[i][dim]
				previousVelocity := velocity[i][dim]
				signChanged := (grad > 0) != (previousVelocity > 0)
				if signChanged {
					gains[i][dim] += 0.2
				} else {
					gains[i][dim] *= 0.8
				}
				if gains[i][dim] < 0.01 {
					gains[i][dim] = 0.01
				}
				velocity[i][dim] = momentum*previousVelocity - options.LearningRate*gains[i][dim]*grad
				y[i][dim] += velocity[i][dim]
			}
		}

		zeroMean2D(y)
	}

	points := make([]ProjectionPoint, len(vectors))
	for i, vector := range vectors {
		points[i] = ProjectionPoint{
			SampleID: vector.SampleID,
			X:        y[i][0],
			Y:        y[i][1],
		}
	}
	return points, nil
}

func validateProjectionVectors(vectors []core.FeatureVector) error {
	dimensions := len(vectors[0].Values)
	if dimensions == 0 {
		return fmt.Errorf("project 2d: vectors must not be empty")
	}
	for _, vector := range vectors[1:] {
		if len(vector.Values) != dimensions {
			return fmt.Errorf("project 2d: inconsistent vector dimensions")
		}
	}
	return nil
}

func centerVectors(vectors []core.FeatureVector) [][]float64 {
	dimensions := len(vectors[0].Values)
	means := make([]float64, dimensions)
	for _, vector := range vectors {
		for dim, value := range vector.Values {
			means[dim] += value
		}
	}
	for dim := range means {
		means[dim] /= float64(len(vectors))
	}

	centered := make([][]float64, len(vectors))
	for i, vector := range vectors {
		row := make([]float64, dimensions)
		for dim, value := range vector.Values {
			row[dim] = value - means[dim]
		}
		centered[i] = row
	}
	return centered
}

func pairwiseSquaredDistances(vectors [][]float64) [][]float64 {
	n := len(vectors)
	distances := make([][]float64, n)
	for i := range distances {
		distances[i] = make([]float64, n)
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			var sum float64
			for dim := range vectors[i] {
				diff := vectors[i][dim] - vectors[j][dim]
				sum += diff * diff
			}
			distances[i][j] = sum
			distances[j][i] = sum
		}
	}
	return distances
}

func symmetricAffinities(distances [][]float64, perplexity float64) [][]float64 {
	n := len(distances)
	conditional := make([][]float64, n)
	for i := range conditional {
		conditional[i] = perplexityRow(distances[i], i, perplexity)
	}

	p := make([][]float64, n)
	for i := range p {
		p[i] = make([]float64, n)
	}
	denom := 2 * float64(n)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			value := (conditional[i][j] + conditional[j][i]) / denom
			p[i][j] = value
			p[j][i] = value
		}
	}
	return p
}

func perplexityRow(distances []float64, self int, perplexity float64) []float64 {
	row := make([]float64, len(distances))
	targetEntropy := math.Log(perplexity)
	beta := 1.0
	minBeta := math.Inf(-1)
	maxBeta := math.Inf(1)

	for iter := 0; iter < 50; iter++ {
		var (
			sum     float64
			entropy float64
			raw     = make([]float64, len(distances))
		)
		for j, distance := range distances {
			if j == self {
				continue
			}
			value := math.Exp(-distance * beta)
			raw[j] = value
			sum += value
		}
		if sum == 0 {
			break
		}
		for j, value := range raw {
			if j == self || value == 0 {
				continue
			}
			prob := value / sum
			row[j] = prob
			entropy -= prob * math.Log(prob)
		}
		diff := entropy - targetEntropy
		if math.Abs(diff) < 1e-5 {
			return row
		}
		if diff > 0 {
			minBeta = beta
			if math.IsInf(maxBeta, 1) {
				beta *= 2
			} else {
				beta = (beta + maxBeta) / 2
			}
		} else {
			maxBeta = beta
			if math.IsInf(minBeta, -1) {
				beta /= 2
			} else {
				beta = (beta + minBeta) / 2
			}
		}
	}

	var sum float64
	for j, distance := range distances {
		if j == self {
			continue
		}
		row[j] = math.Exp(-distance * beta)
		sum += row[j]
	}
	if sum == 0 {
		return row
	}
	for j := range row {
		if j == self {
			continue
		}
		row[j] /= sum
	}
	return row
}

func lowDimensionalAffinities(points [][2]float64) ([][]float64, float64) {
	n := len(points)
	q := make([][]float64, n)
	for i := range q {
		q[i] = make([]float64, n)
	}
	var sum float64
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			diffX := points[i][0] - points[j][0]
			diffY := points[i][1] - points[j][1]
			value := 1 / (1 + diffX*diffX + diffY*diffY)
			q[i][j] = value
			q[j][i] = value
			sum += 2 * value
		}
	}
	return q, sum
}

func zeroMean2D(points [][2]float64) {
	var meanX, meanY float64
	for _, point := range points {
		meanX += point[0]
		meanY += point[1]
	}
	meanX /= float64(len(points))
	meanY /= float64(len(points))
	for i := range points {
		points[i][0] -= meanX
		points[i][1] -= meanY
	}
}
