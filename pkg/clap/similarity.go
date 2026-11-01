package clap

import "math"

// L2Normalize normalises v in-place to unit length. No-ops on zero vectors.
func L2Normalize(v []float32) {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	norm := math.Sqrt(sum)
	if norm < 1e-12 {
		return
	}
	inv := float32(1.0 / norm)
	for i := range v {
		v[i] *= inv
	}
}

// l2Normalize is an alias kept for backward package compatibility.
func l2Normalize(v []float32) {
	L2Normalize(v)
}

// CosineSimilarity returns the cosine similarity of two vectors.
// For best results normalise both vectors with L2Normalize first, in which
// case this is just the dot product.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom < 1e-12 {
		return 0
	}
	return float32(dot / denom)
}

// cosineSimilarity is an alias kept for backward package compatibility.
func cosineSimilarity(a, b []float32) float32 {
	return CosineSimilarity(a, b)
}
