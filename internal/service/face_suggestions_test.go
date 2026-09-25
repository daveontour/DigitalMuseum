package service

import (
	"math"
	"testing"
)

func unitVec(dim int, axes map[int]float32) []float32 {
	v := make([]float32, dim)
	for i, x := range axes {
		v[i] = x
	}
	return NormalizeFaceEmbedding(v)
}

func TestNearestNamedContact(t *testing.T) {
	const dim = 512
	query := unitVec(dim, map[int]float32{0: 1})
	named := []namedFaceEmbedding{
		{contactID: 1, vec: unitVec(dim, map[int]float32{0: 1, 1: 0.6})},  // ~0.54 away
		{contactID: 2, vec: unitVec(dim, map[int]float32{0: 1, 1: 0.1})},  // ~0.10 away — nearest
		{contactID: 3, vec: unitVec(dim, map[int]float32{2: 1})},          // ~1.41 away
		{contactID: 4, vec: unitVec(dim, map[int]float32{0: -1, 3: 0.2})}, // ~1.99 away
	}

	contactID, dist, ok := nearestNamedContact(query, named, faceSameClusterL2Sq)
	if !ok || contactID != 2 {
		t.Fatalf("want nearest contact 2, got %d ok=%v", contactID, ok)
	}
	if math.Abs(dist-0.0998) > 0.001 {
		t.Errorf("want distance ~0.0998, got %f", dist)
	}

	// Nothing within the threshold: only faces ~1.41 and ~1.99 away.
	if _, _, ok := nearestNamedContact(query, named[2:], faceSameClusterL2Sq); ok {
		t.Error("want no match when every named face is beyond the threshold")
	}

	// Order-independence: early abandonment must never discard the true nearest.
	reversed := []namedFaceEmbedding{named[3], named[2], named[0], named[1]}
	if contactID, _, _ := nearestNamedContact(query, reversed, faceSameClusterL2Sq); contactID != 2 {
		t.Errorf("want nearest contact 2 regardless of order, got %d", contactID)
	}

	if _, _, ok := nearestNamedContact(query, nil, faceSameClusterL2Sq); ok {
		t.Error("want no match with no named faces")
	}
}
