package cluster

import (
	"context"
	"fmt"
	"time"

	"github.com/darkliquid/zounds/pkg/core"
)

type DBSCANOptions struct {
	Epsilon   float64
	MinPoints int
}

type DBSCANResult struct {
	Clusters    []core.Cluster
	Memberships []Membership
	Noise       []core.FeatureVector
}

type DBSCAN struct {
	options DBSCANOptions
}

func NewDBSCAN(options DBSCANOptions) *DBSCAN {
	if options.Epsilon <= 0 {
		options.Epsilon = 0.5
	}
	if options.MinPoints <= 0 {
		options.MinPoints = 2
	}
	return &DBSCAN{options: options}
}

func (d *DBSCAN) Name() string {
	return "dbscan"
}

func (d *DBSCAN) Cluster(_ context.Context, vectors []core.FeatureVector) ([]core.Cluster, error) {
	result, err := d.Fit(vectors)
	if err != nil {
		return nil, err
	}
	return result.Clusters, nil
}

func (d *DBSCAN) Fit(vectors []core.FeatureVector) (DBSCANResult, error) {
	if d == nil {
		return DBSCANResult{}, fmt.Errorf("dbscan: nil clusterer")
	}
	if len(vectors) == 0 {
		return DBSCANResult{}, nil
	}

	visited := make([]bool, len(vectors))
	assignments := make([]int, len(vectors))
	for i := range assignments {
		assignments[i] = -1
	}

	clusterIndex := 0
	for i := range vectors {
		if visited[i] {
			continue
		}
		visited[i] = true

		neighbors, err := d.regionQuery(vectors, i)
		if err != nil {
			return DBSCANResult{}, err
		}
		if len(neighbors) < d.options.MinPoints {
			continue
		}

		if err := d.expandCluster(vectors, i, neighbors, clusterIndex, visited, assignments); err != nil {
			return DBSCANResult{}, err
		}
		clusterIndex++
	}

	clusterSizes := make([]int, clusterIndex)
	memberships := make([]Membership, 0, len(vectors))
	noise := make([]core.FeatureVector, 0)
	for i, vector := range vectors {
		if assignments[i] < 0 {
			noise = append(noise, vector)
			continue
		}
		distance, err := d.distanceToCluster(vectors, i, assignments)
		if err != nil {
			return DBSCANResult{}, err
		}
		memberships = append(memberships, Membership{
			SampleID:     vector.SampleID,
			ClusterIndex: assignments[i],
			Distance:     distance,
		})
		clusterSizes[assignments[i]]++
	}

	clusters := make([]core.Cluster, clusterIndex)
	for i := 0; i < clusterIndex; i++ {
		clusters[i] = core.Cluster{
			Method: "dbscan",
			Label:  fmt.Sprintf("cluster-%d", i+1),
			Size:   clusterSizes[i],
			Parameters: map[string]float64{
				"epsilon":    d.options.Epsilon,
				"min_points": float64(d.options.MinPoints),
			},
			CreatedAt: time.Now().UTC(),
		}
	}

	return DBSCANResult{
		Clusters:    clusters,
		Memberships: memberships,
		Noise:       noise,
	}, nil
}

func (d *DBSCAN) expandCluster(vectors []core.FeatureVector, point int, neighbors []int, clusterIndex int, visited []bool, assignments []int) error {
	assignments[point] = clusterIndex
	queue := append([]int(nil), neighbors...)

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if !visited[current] {
			visited[current] = true
			currentNeighbors, err := d.regionQuery(vectors, current)
			if err != nil {
				return err
			}
			if len(currentNeighbors) >= d.options.MinPoints {
				queue = append(queue, currentNeighbors...)
			}
		}

		if assignments[current] == -1 {
			assignments[current] = clusterIndex
		}
	}

	return nil
}

func (d *DBSCAN) regionQuery(vectors []core.FeatureVector, point int) ([]int, error) {
	neighbors := make([]int, 0, len(vectors))
	for i := range vectors {
		distance, err := EuclideanDistance(vectors[point].Values, vectors[i].Values)
		if err != nil {
			return nil, err
		}
		if distance <= d.options.Epsilon {
			neighbors = append(neighbors, i)
		}
	}
	return neighbors, nil
}

func (d *DBSCAN) distanceToCluster(vectors []core.FeatureVector, point int, assignments []int) (float64, error) {
	clusterIndex := assignments[point]
	best := -1.0
	for i := range vectors {
		if i == point || assignments[i] != clusterIndex {
			continue
		}
		distance, err := EuclideanDistance(vectors[point].Values, vectors[i].Values)
		if err != nil {
			return 0, err
		}
		if best < 0 || distance < best {
			best = distance
		}
	}
	if best < 0 {
		return 0, nil
	}
	return best, nil
}
