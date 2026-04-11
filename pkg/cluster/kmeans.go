package cluster

import (
	"context"
	"fmt"
	"time"

	"github.com/darkliquid/zounds/pkg/core"
)

type Membership struct {
	SampleID     int64
	ClusterIndex int
	Distance     float64
}

type KMeansOptions struct {
	K             int
	MaxIterations int
}

type KMeansResult struct {
	Clusters    []core.Cluster
	Memberships []Membership
	Centroids   [][]float64
	Iterations  int
}

type KMeans struct {
	options KMeansOptions
}

func NewKMeans(options KMeansOptions) *KMeans {
	if options.MaxIterations <= 0 {
		options.MaxIterations = 50
	}
	return &KMeans{options: options}
}

func (k *KMeans) Name() string {
	return "kmeans"
}

func (k *KMeans) Cluster(_ context.Context, vectors []core.FeatureVector) ([]core.Cluster, error) {
	result, err := k.Fit(vectors)
	if err != nil {
		return nil, err
	}
	return result.Clusters, nil
}

func (k *KMeans) Fit(vectors []core.FeatureVector) (KMeansResult, error) {
	if k == nil {
		return KMeansResult{}, fmt.Errorf("kmeans: nil clusterer")
	}
	if k.options.K <= 0 {
		return KMeansResult{}, fmt.Errorf("kmeans: K must be greater than zero")
	}
	if len(vectors) < k.options.K {
		return KMeansResult{}, fmt.Errorf("kmeans: need at least %d vectors, got %d", k.options.K, len(vectors))
	}

	centroids, err := initializeCentroids(vectors, k.options.K)
	if err != nil {
		return KMeansResult{}, err
	}

	assignments := make([]int, len(vectors))
	for i := range assignments {
		assignments[i] = -1
	}

	iterations := 0
	for ; iterations < k.options.MaxIterations; iterations++ {
		changed := false
		for i, vector := range vectors {
			nearest, _, err := nearestCentroid(vector.Values, centroids)
			if err != nil {
				return KMeansResult{}, err
			}
			if assignments[i] != nearest {
				assignments[i] = nearest
				changed = true
			}
		}

		if !changed && iterations > 0 {
			break
		}

		updated := recomputeCentroids(vectors, assignments, centroids)
		copyCentroids(centroids, updated)
	}

	memberships := make([]Membership, 0, len(vectors))
	clusterSizes := make([]int, len(centroids))
	for i, vector := range vectors {
		clusterIndex := assignments[i]
		_, distance, err := nearestCentroid(vector.Values, centroids)
		if err != nil {
			return KMeansResult{}, err
		}
		memberships = append(memberships, Membership{
			SampleID:     vector.SampleID,
			ClusterIndex: clusterIndex,
			Distance:     distance,
		})
		clusterSizes[clusterIndex]++
	}

	clusters := make([]core.Cluster, len(centroids))
	for i := range clusters {
		clusters[i] = core.Cluster{
			Method: "kmeans",
			Label:  fmt.Sprintf("cluster-%d", i+1),
			Size:   clusterSizes[i],
			Parameters: map[string]float64{
				"k":          float64(k.options.K),
				"iterations": float64(iterations),
			},
			CreatedAt: time.Now().UTC(),
		}
	}

	return KMeansResult{
		Clusters:    clusters,
		Memberships: memberships,
		Centroids:   cloneCentroids(centroids),
		Iterations:  iterations,
	}, nil
}

func initializeCentroids(vectors []core.FeatureVector, k int) ([][]float64, error) {
	centroids := make([][]float64, 0, k)
	centroids = append(centroids, append([]float64(nil), vectors[0].Values...))

	for len(centroids) < k {
		bestIndex := -1
		bestDistance := -1.0
		for _, vector := range vectors {
			_, distance, err := nearestCentroid(vector.Values, centroids)
			if err != nil {
				return nil, err
			}
			if distance > bestDistance {
				bestDistance = distance
				bestIndex = indexOfVector(vectors, vector.SampleID)
			}
		}
		centroids = append(centroids, append([]float64(nil), vectors[bestIndex].Values...))
	}

	return centroids, nil
}

func nearestCentroid(values []float64, centroids [][]float64) (int, float64, error) {
	bestIndex := 0
	bestDistance := 0.0
	for i, centroid := range centroids {
		distance, err := EuclideanDistance(values, centroid)
		if err != nil {
			return 0, 0, err
		}
		if i == 0 || distance < bestDistance {
			bestIndex = i
			bestDistance = distance
		}
	}
	return bestIndex, bestDistance, nil
}

func recomputeCentroids(vectors []core.FeatureVector, assignments []int, current [][]float64) [][]float64 {
	next := make([][]float64, len(current))
	counts := make([]int, len(current))
	for i, centroid := range current {
		next[i] = make([]float64, len(centroid))
	}

	for i, vector := range vectors {
		clusterIndex := assignments[i]
		counts[clusterIndex]++
		for dim, value := range vector.Values {
			next[clusterIndex][dim] += value
		}
	}

	for i := range next {
		if counts[i] == 0 {
			copy(next[i], current[i])
			continue
		}
		for dim := range next[i] {
			next[i][dim] /= float64(counts[i])
		}
	}

	return next
}

func copyCentroids(dst, src [][]float64) {
	for i := range dst {
		copy(dst[i], src[i])
	}
}

func cloneCentroids(centroids [][]float64) [][]float64 {
	out := make([][]float64, len(centroids))
	for i, centroid := range centroids {
		out[i] = append([]float64(nil), centroid...)
	}
	return out
}

func indexOfVector(vectors []core.FeatureVector, sampleID int64) int {
	for i, vector := range vectors {
		if vector.SampleID == sampleID {
			return i
		}
	}
	return -1
}
