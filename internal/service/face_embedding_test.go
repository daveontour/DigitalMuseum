package service

import (
	"math"
	"testing"
)

func vecNorm(v []float32) float64 {
	var sumSq float64
	for _, x := range v {
		sumSq += float64(x) * float64(x)
	}
	return math.Sqrt(sumSq)
}

func TestNormalizeFaceEmbedding_UnitLength(t *testing.T) {
	tests := [][]float32{
		{3, 4},           // 3-4-5 triangle, easy to hand-verify
		{1, 1, 1, 1},     // norm = 2
		{10, 0, 0},       // already axis-aligned
		{-2, -3, 6},      // norm = 7, includes negatives
		{0.001, 0.002, 0.003, 0.004, 0.005},
	}
	for _, v := range tests {
		got := NormalizeFaceEmbedding(v)
		gotNorm := vecNorm(got)
		if math.Abs(gotNorm-1.0) > 1e-6 {
			t.Errorf("NormalizeFaceEmbedding(%v) has norm %v, want ~1.0", v, gotNorm)
		}
		if len(got) != len(v) {
			t.Errorf("NormalizeFaceEmbedding(%v) changed length: got %d, want %d", v, len(got), len(v))
		}
	}
}

func TestNormalizeFaceEmbedding_PreservesDirection(t *testing.T) {
	// {3,4} normalized should be {0.6, 0.8} exactly.
	got := NormalizeFaceEmbedding([]float32{3, 4})
	want := []float32{0.6, 0.8}
	for i := range want {
		if math.Abs(float64(got[i]-want[i])) > 1e-6 {
			t.Errorf("component %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestNormalizeFaceEmbedding_ZeroVectorUnchanged(t *testing.T) {
	zero := []float32{0, 0, 0}
	got := NormalizeFaceEmbedding(zero)
	for i, v := range got {
		if v != 0 {
			t.Errorf("component %d: got %v, want 0 (zero vector should pass through unchanged, not NaN)", i, v)
		}
	}
}

// TestL2DistanceMatchesCosineSimilarityForUnitVectors documents and verifies
// the relationship FindSimilarFaces relies on: for unit vectors, squared L2
// distance equals 2 - 2*cos_sim. This is why normalizing before storage makes
// vec0's built-in L2 "MATCH" distance behave like cosine similarity without
// needing a custom distance function.
func TestL2DistanceMatchesCosineSimilarityForUnitVectors(t *testing.T) {
	a := NormalizeFaceEmbedding([]float32{1, 0, 0})
	b := NormalizeFaceEmbedding([]float32{0, 1, 0}) // orthogonal: cos_sim = 0, L2² should be 2
	c := NormalizeFaceEmbedding([]float32{1, 0, 0}) // identical: cos_sim = 1, L2² should be 0

	l2sq := func(x, y []float32) float64 {
		var s float64
		for i := range x {
			d := float64(x[i]) - float64(y[i])
			s += d * d
		}
		return s
	}
	cosSim := func(x, y []float32) float64 {
		var dot float64
		for i := range x {
			dot += float64(x[i]) * float64(y[i])
		}
		return dot
	}

	for _, tt := range []struct {
		name string
		x, y []float32
	}{
		{"orthogonal", a, b},
		{"identical", a, c},
	} {
		gotL2sq := l2sq(tt.x, tt.y)
		wantL2sq := 2 - 2*cosSim(tt.x, tt.y)
		if math.Abs(gotL2sq-wantL2sq) > 1e-6 {
			t.Errorf("%s: L2²=%v, want 2-2*cos_sim=%v", tt.name, gotL2sq, wantL2sq)
		}
	}
}
