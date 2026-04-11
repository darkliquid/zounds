package cluster

import (
	"fmt"

	"gonum.org/v1/gonum/mat"

	"github.com/darkliquid/zounds/pkg/core"
)

type ProjectionPoint struct {
	SampleID int64
	X        float64
	Y        float64
}

func Project2D(vectors []core.FeatureVector) ([]ProjectionPoint, error) {
	if len(vectors) == 0 {
		return nil, nil
	}

	dimensions := len(vectors[0].Values)
	if dimensions == 0 {
		return nil, fmt.Errorf("project 2d: vectors must not be empty")
	}
	for _, vector := range vectors[1:] {
		if len(vector.Values) != dimensions {
			return nil, fmt.Errorf("project 2d: inconsistent vector dimensions")
		}
	}

	data := mat.NewDense(len(vectors), dimensions, nil)
	means := make([]float64, dimensions)
	for _, vector := range vectors {
		for dim, value := range vector.Values {
			means[dim] += value
		}
	}
	for dim := range means {
		means[dim] /= float64(len(vectors))
	}

	for row, vector := range vectors {
		for dim, value := range vector.Values {
			data.Set(row, dim, value-means[dim])
		}
	}

	var svd mat.SVD
	ok := svd.Factorize(data, mat.SVDThin)
	if !ok {
		return nil, fmt.Errorf("project 2d: svd factorization failed")
	}

	var v mat.Dense
	svd.VTo(&v)
	_, cols := v.Dims()
	if cols == 0 {
		return nil, fmt.Errorf("project 2d: no principal components available")
	}

	components := 2
	if cols < components {
		components = cols
	}
	projection := mat.NewDense(len(vectors), components, nil)
	projection.Mul(data, v.Slice(0, dimensions, 0, components))

	points := make([]ProjectionPoint, len(vectors))
	for i, vector := range vectors {
		point := ProjectionPoint{SampleID: vector.SampleID}
		point.X = projection.At(i, 0)
		if components > 1 {
			point.Y = projection.At(i, 1)
		}
		points[i] = point
	}

	return points, nil
}
