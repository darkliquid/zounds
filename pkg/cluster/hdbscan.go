package cluster

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/darkliquid/zounds/pkg/core"
)

type HDBSCANOptions struct {
	MinClusterSize int
	MinSamples     int
}

type HDBSCANResult struct {
	Clusters    []core.Cluster
	Memberships []Membership
	Noise       []core.FeatureVector
}

type HDBSCAN struct {
	options HDBSCANOptions
}

func NewHDBSCAN(options HDBSCANOptions) *HDBSCAN {
	if options.MinClusterSize <= 0 {
		options.MinClusterSize = 5
	}
	if options.MinSamples <= 0 {
		options.MinSamples = options.MinClusterSize
	}
	return &HDBSCAN{options: options}
}

func (h *HDBSCAN) Name() string {
	return "hdbscan"
}

func (h *HDBSCAN) Cluster(_ context.Context, vectors []core.FeatureVector) ([]core.Cluster, error) {
	result, err := h.Fit(vectors)
	if err != nil {
		return nil, err
	}
	return result.Clusters, nil
}

type dendroNode struct {
	left     int
	right    int
	distance float64
	size     int
	members  []int
}

type condensedNode struct {
	id          int
	children    []int
	birthLambda float64
	deathLambda float64
	leafMembers []int
	dropLambdas map[int]float64
	stability   float64
	selected    bool
}

func (h *HDBSCAN) Fit(vectors []core.FeatureVector) (HDBSCANResult, error) {
	if h == nil {
		return HDBSCANResult{}, fmt.Errorf("hdbscan: nil clusterer")
	}
	if len(vectors) == 0 {
		return HDBSCANResult{}, nil
	}
	if err := validateProjectionVectors(vectors); err != nil {
		return HDBSCANResult{}, err
	}

	n := len(vectors)
	minClusterSize := h.options.MinClusterSize
	if minClusterSize < 2 {
		minClusterSize = 2
	}
	if n < minClusterSize {
		return HDBSCANResult{
			Noise: append([]core.FeatureVector(nil), vectors...),
		}, nil
	}
	minSamples := h.options.MinSamples
	if minSamples < 1 {
		minSamples = 1
	}
	if minSamples > n {
		minSamples = n
	}

	// 1. Pairwise Euclidean distances
	distMatrix := make([][]float64, n)
	for i := range distMatrix {
		distMatrix[i] = make([]float64, n)
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			d, err := EuclideanDistance(vectors[i].Values, vectors[j].Values)
			if err != nil {
				return HDBSCANResult{}, err
			}
			distMatrix[i][j] = d
			distMatrix[j][i] = d
		}
	}

	// 2. Core distances: distance to minSamples-th neighbor (1-indexed, index k-1 in sorted row)
	coreDistances := make([]float64, n)
	for i := 0; i < n; i++ {
		row := append([]float64(nil), distMatrix[i]...)
		sort.Float64s(row)
		k := minSamples
		if k > n {
			k = n
		}
		coreDistances[i] = row[k-1]
	}

	// 3. Mutual reachability distance
	mreach := func(i, j int) float64 {
		d := distMatrix[i][j]
		if coreDistances[i] > d {
			d = coreDistances[i]
		}
		if coreDistances[j] > d {
			d = coreDistances[j]
		}
		return d
	}

	// 4. Minimum Spanning Tree via Prim's algorithm
	type mstEdge struct {
		u, v   int
		weight float64
	}
	inTree := make([]bool, n)
	inTree[0] = true
	minDist := make([]float64, n)
	minParent := make([]int, n)
	for i := 1; i < n; i++ {
		minDist[i] = mreach(0, i)
		minParent[i] = 0
	}

	edges := make([]mstEdge, 0, n-1)
	for step := 0; step < n-1; step++ {
		bestNode := -1
		bestDist := math.Inf(1)
		for i := 1; i < n; i++ {
			if !inTree[i] && minDist[i] < bestDist {
				bestDist = minDist[i]
				bestNode = i
			}
		}
		if bestNode < 0 {
			break
		}
		inTree[bestNode] = true
		edges = append(edges, mstEdge{
			u:      minParent[bestNode],
			v:      bestNode,
			weight: bestDist,
		})
		for i := 1; i < n; i++ {
			if !inTree[i] {
				w := mreach(bestNode, i)
				if w < minDist[i] {
					minDist[i] = w
					minParent[i] = bestNode
				}
			}
		}
	}

	// 5. Sort MST edges by weight ascending
	sort.Slice(edges, func(i, j int) bool {
		return edges[i].weight < edges[j].weight
	})

	// 6. Build single-linkage dendrogram
	dendro := make([]dendroNode, 2*n-1)
	for i := 0; i < n; i++ {
		dendro[i] = dendroNode{
			left:    -1,
			right:   -1,
			size:    1,
			members: []int{i},
		}
	}
	ufParent := make([]int, 2*n-1)
	for i := range ufParent {
		ufParent[i] = i
	}
	var find func(i int) int
	find = func(i int) int {
		if ufParent[i] != i {
			ufParent[i] = find(ufParent[i])
		}
		return ufParent[i]
	}

	nextNodeID := n
	for _, edge := range edges {
		rootU := find(edge.u)
		rootV := find(edge.v)
		if rootU == rootV {
			continue
		}
		newNodeID := nextNodeID
		nextNodeID++

		nodeU := &dendro[rootU]
		nodeV := &dendro[rootV]

		mergedMembers := make([]int, len(nodeU.members)+len(nodeV.members))
		copy(mergedMembers, nodeU.members)
		copy(mergedMembers[len(nodeU.members):], nodeV.members)

		dendro[newNodeID] = dendroNode{
			left:     rootU,
			right:    rootV,
			distance: edge.weight,
			size:     nodeU.size + nodeV.size,
			members:  mergedMembers,
		}
		ufParent[rootU] = newNodeID
		ufParent[rootV] = newNodeID
	}
	rootDendroID := nextNodeID - 1

	// 7. Condense tree
	var condensed []*condensedNode
	rootCond := &condensedNode{
		id:          0,
		dropLambdas: make(map[int]float64),
	}
	condensed = append(condensed, rootCond)

	var condense func(dendroID int, clusterID int)
	condense = func(dendroID int, clusterID int) {
		if dendroID < n {
			condensed[clusterID].leafMembers = append(condensed[clusterID].leafMembers, dendroID)
			return
		}

		node := &dendro[dendroID]
		d := node.distance
		lambda := 1e12
		if d > 1e-12 {
			lambda = 1.0 / d
		}

		leftSize := dendro[node.left].size
		rightSize := dendro[node.right].size

		if leftSize >= minClusterSize && rightSize >= minClusterSize {
			condensed[clusterID].deathLambda = lambda

			child1ID := len(condensed)
			child1 := &condensedNode{
				id:          child1ID,
				birthLambda: lambda,
				dropLambdas: make(map[int]float64),
			}
			condensed = append(condensed, child1)
			condensed[clusterID].children = append(condensed[clusterID].children, child1ID)

			child2ID := len(condensed)
			child2 := &condensedNode{
				id:          child2ID,
				birthLambda: lambda,
				dropLambdas: make(map[int]float64),
			}
			condensed = append(condensed, child2)
			condensed[clusterID].children = append(condensed[clusterID].children, child2ID)

			condense(node.left, child1ID)
			condense(node.right, child2ID)
		} else if leftSize >= minClusterSize && rightSize < minClusterSize {
			for _, p := range dendro[node.right].members {
				condensed[clusterID].dropLambdas[p] = lambda
			}
			condense(node.left, clusterID)
		} else if rightSize >= minClusterSize && leftSize < minClusterSize {
			for _, p := range dendro[node.left].members {
				condensed[clusterID].dropLambdas[p] = lambda
			}
			condense(node.right, clusterID)
		} else {
			for _, p := range dendro[node.left].members {
				condensed[clusterID].dropLambdas[p] = lambda
			}
			for _, p := range dendro[node.right].members {
				condensed[clusterID].dropLambdas[p] = lambda
			}
		}
	}

	condense(rootDendroID, 0)

	// 8. Compute cluster stabilities
	for _, c := range condensed {
		for _, dropLam := range c.dropLambdas {
			if dropLam > c.birthLambda {
				c.stability += dropLam - c.birthLambda
			}
		}
		deathLam := c.deathLambda
		if deathLam <= c.birthLambda {
			deathLam = c.birthLambda + 1.0
		}
		for range c.leafMembers {
			c.stability += deathLam - c.birthLambda
		}
	}

	// 9. Bottom-up selection of stable clusters
	var unselectDescendants func(c *condensedNode)
	unselectDescendants = func(c *condensedNode) {
		for _, childID := range c.children {
			child := condensed[childID]
			child.selected = false
			unselectDescendants(child)
		}
	}

	for i := len(condensed) - 1; i >= 0; i-- {
		c := condensed[i]
		if len(c.children) == 0 {
			c.selected = true
		} else {
			childSum := 0.0
			for _, childID := range c.children {
				childSum += condensed[childID].stability
			}
			if c.stability > childSum {
				c.selected = true
				unselectDescendants(c)
			} else {
				c.selected = false
				c.stability = childSum
			}
		}
	}

	// If root was selected, but has children, keep it as single cluster if root stability won;
	// if root was selected with no children, it's 1 cluster.
	var selectedClusters []*condensedNode
	for _, c := range condensed {
		if c.selected {
			selectedClusters = append(selectedClusters, c)
		}
	}

	// Collect points for each selected cluster
	var collectPoints func(c *condensedNode) []int
	collectPoints = func(c *condensedNode) []int {
		points := append([]int(nil), c.leafMembers...)
		for p := range c.dropLambdas {
			points = append(points, p)
		}
		for _, childID := range c.children {
			points = append(points, collectPoints(condensed[childID])...)
		}
		return points
	}

	clusterMembers := make([][]int, len(selectedClusters))
	assigned := make([]int, n)
	for i := range assigned {
		assigned[i] = -1
	}

	validClusterIdx := 0
	for _, sc := range selectedClusters {
		pts := collectPoints(sc)
		// De-duplicate points if any
		seen := make(map[int]bool)
		uniquePts := make([]int, 0, len(pts))
		for _, p := range pts {
			if !seen[p] {
				seen[p] = true
				uniquePts = append(uniquePts, p)
			}
		}
		if len(uniquePts) >= minClusterSize {
			for _, p := range uniquePts {
				assigned[p] = validClusterIdx
			}
			clusterMembers[validClusterIdx] = uniquePts
			validClusterIdx++
		}
	}
	clusterMembers = clusterMembers[:validClusterIdx]

	// 10. Form result
	clusters := make([]core.Cluster, validClusterIdx)
	for i := 0; i < validClusterIdx; i++ {
		clusters[i] = core.Cluster{
			Method: "hdbscan",
			Label:  fmt.Sprintf("cluster-%d", i+1),
			Size:   len(clusterMembers[i]),
			Parameters: map[string]float64{
				"min_cluster_size": float64(minClusterSize),
				"min_samples":      float64(minSamples),
			},
			CreatedAt: time.Now().UTC(),
		}
	}

	memberships := make([]Membership, 0, n)
	noise := make([]core.FeatureVector, 0)
	for i, vector := range vectors {
		cIdx := assigned[i]
		if cIdx < 0 {
			noise = append(noise, vector)
			continue
		}
		// Compute distance to centroid
		dist := h.distanceToCentroid(vectors, i, clusterMembers[cIdx])
		memberships = append(memberships, Membership{
			SampleID:     vector.SampleID,
			ClusterIndex: cIdx,
			Distance:     dist,
		})
	}

	return HDBSCANResult{
		Clusters:    clusters,
		Memberships: memberships,
		Noise:       noise,
	}, nil
}

func (h *HDBSCAN) distanceToCentroid(vectors []core.FeatureVector, point int, members []int) float64 {
	if len(members) == 0 {
		return 0
	}
	dims := len(vectors[point].Values)
	centroid := make([]float64, dims)
	for _, m := range members {
		for d := 0; d < dims; d++ {
			centroid[d] += vectors[m].Values[d]
		}
	}
	inv := 1.0 / float64(len(members))
	for d := 0; d < dims; d++ {
		centroid[d] *= inv
	}
	dist, err := EuclideanDistance(vectors[point].Values, centroid)
	if err != nil {
		return 0
	}
	return dist
}
