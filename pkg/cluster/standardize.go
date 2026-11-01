package cluster

import (
	"math"
	"sort"

	"github.com/darkliquid/zounds/pkg/core"
)

// StandardizeMethod selects between zscore and iqr standardization.
type StandardizeMethod string

const (
	StandardizeZScore StandardizeMethod = "zscore"
	StandardizeIQR    StandardizeMethod = "iqr"
)

// StandardizeVectors standardizes a dataset of feature vectors along each dimension.
// If method is empty or "iqr", it uses median and Inter-Quartile Range (IQR);
// if "zscore", it uses mean and standard deviation.
// Dimensions with zero dispersion are centered to 0.0.
func StandardizeVectors(vectors []core.FeatureVector, method ...StandardizeMethod) []core.FeatureVector {
	if len(vectors) <= 1 {
		return vectors
	}

	mode := StandardizeIQR
	if len(method) > 0 && method[0] == StandardizeZScore {
		mode = StandardizeZScore
	}

	dims := vectors[0].Dimensions
	if dims == 0 && len(vectors[0].Values) > 0 {
		dims = len(vectors[0].Values)
	}

	for _, v := range vectors {
		if len(v.Values) != dims {
			res := make([]core.FeatureVector, len(vectors))
			copy(res, vectors)
			return res
		}
	}

	centers := make([]float64, dims)
	scales := make([]float64, dims)

	col := make([]float64, len(vectors))

	for j := 0; j < dims; j++ {
		for i := range vectors {
			col[i] = vectors[i].Values[j]
		}

		if mode == StandardizeIQR {
			sort.Float64s(col)
			q1 := quantile(col, 0.25)
			median := quantile(col, 0.50)
			q3 := quantile(col, 0.75)
			iqr := q3 - q1

			if iqr > 1e-9 {
				centers[j] = median
				scales[j] = iqr
			} else {
				mean, std := meanStd(col)
				if std > 1e-9 {
					centers[j] = mean
					scales[j] = std
				} else {
					centers[j] = mean
					scales[j] = 1.0
				}
			}
		} else {
			mean, std := meanStd(col)
			if std > 1e-9 {
				centers[j] = mean
				scales[j] = std
			} else {
				centers[j] = mean
				scales[j] = 1.0
			}
		}
	}

	res := make([]core.FeatureVector, len(vectors))
	for i, v := range vectors {
		stdVals := make([]float64, dims)
		for j := 0; j < dims; j++ {
			if scales[j] > 1e-9 {
				stdVals[j] = (v.Values[j] - centers[j]) / scales[j]
			}
		}
		res[i] = core.FeatureVector{
			ID:         v.ID,
			SampleID:   v.SampleID,
			Namespace:  v.Namespace,
			Version:    v.Version,
			Dimensions: dims,
			Values:     stdVals,
			CreatedAt:  v.CreatedAt,
		}
	}

	return res
}

func quantile(sorted []float64, q float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sorted[0]
	}
	idx := q * float64(n-1)
	low := int(math.Floor(idx))
	high := int(math.Ceil(idx))
	if low == high {
		return sorted[low]
	}
	weight := idx - float64(low)
	return (1.0-weight)*sorted[low] + weight*sorted[high]
}

func meanStd(vals []float64) (float64, float64) {
	n := float64(len(vals))
	if n == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	mean := sum / n

	var sumSq float64
	for _, v := range vals {
		diff := v - mean
		sumSq += diff * diff
	}
	std := math.Sqrt(sumSq / n)
	return mean, std
}
