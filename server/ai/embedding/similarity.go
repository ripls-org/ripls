package embedding

import "math"

// CosineSimilarity returns the cosine similarity of two vectors — their dot
// product divided by the product of their magnitudes. For the L2-normalized
// vectors this package's Generate produces, the magnitudes are 1, so this
// reduces to a plain dot product; the normalization is kept so the helper is
// also correct for un-normalized inputs.
//
// Returns 0 when the vectors have different lengths, either is empty, or either
// has zero magnitude — callers treat 0 as "not similar".
func CosineSimilarity(a, b []float32) float32 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	denom := float32(math.Sqrt(float64(normA)) * math.Sqrt(float64(normB)))
	return dot / denom
}
