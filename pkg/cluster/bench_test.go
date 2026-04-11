package cluster

import (
	"testing"

	"github.com/darkliquid/zounds/pkg/core"
)

func BenchmarkKMeansFit(b *testing.B) {
	model := NewKMeans(KMeansOptions{K: 8, MaxIterations: 20})
	vectors := benchmarkVectors(256, 16)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := model.Fit(vectors); err != nil {
			b.Fatalf("Fit returned error: %v", err)
		}
	}
}

func benchmarkVectors(count, dims int) []core.FeatureVector {
	out := make([]core.FeatureVector, 0, count)
	for i := 0; i < count; i++ {
		values := make([]float64, dims)
		for j := 0; j < dims; j++ {
			values[j] = float64((i%8)*10+j) / float64(dims)
		}
		out = append(out, core.FeatureVector{
			SampleID: int64(i + 1),
			Values:   values,
		})
	}
	return out
}
