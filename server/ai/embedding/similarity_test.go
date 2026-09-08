package embedding

import (
	"math"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	cases := []struct {
		name string
		a, b []float32
		want float32
	}{
		{"identical", []float32{1, 0, 0}, []float32{1, 0, 0}, 1},
		{"orthogonal", []float32{1, 0, 0}, []float32{0, 1, 0}, 0},
		{"opposite", []float32{1, 0}, []float32{-1, 0}, -1},
		{"unnormalized identical direction", []float32{2, 0}, []float32{5, 0}, 1},
		{"length mismatch", []float32{1, 0, 0}, []float32{1, 0}, 0},
		{"empty", []float32{}, []float32{}, 0},
		{"zero magnitude a", []float32{0, 0}, []float32{1, 1}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := CosineSimilarity(c.a, c.b)
			if math.Abs(float64(got-c.want)) > 1e-6 {
				t.Errorf("CosineSimilarity(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}

// TestCosineSimilarityMatchesNormalizedDotProduct confirms that, for the
// L2-normalized vectors Generate emits, cosine similarity equals the raw dot
// product — the property clusterByCosine relies on.
func TestCosineSimilarityMatchesNormalizedDotProduct(t *testing.T) {
	e := &Embedder{}
	a := e.normalize([]float32{0.3, 0.7, -0.2, 0.5})
	b := e.normalize([]float32{0.25, 0.6, -0.1, 0.55})

	var dot float32
	for i := range a {
		dot += a[i] * b[i]
	}
	got := CosineSimilarity(a, b)
	if math.Abs(float64(got-dot)) > 1e-6 {
		t.Errorf("cosine %v != dot %v for normalized vectors", got, dot)
	}
}
