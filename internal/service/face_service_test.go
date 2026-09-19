package service

import "testing"

func i64(v int64) *int64 { return &v }

func TestDecideClusterAssignment_EmptyCandidates(t *testing.T) {
	got := decideClusterAssignment(nil)
	if got.joinExisting {
		t.Fatalf("want new cluster for no candidates, got %+v", got)
	}
}

func TestDecideClusterAssignment_AllUnclustered(t *testing.T) {
	// Nearest matches exist but none has a cluster yet — nothing to join.
	got := decideClusterAssignment([]clusterCandidate{
		{distance: 0.1, clusterID: nil, contactID: nil},
		{distance: 0.2, clusterID: nil, contactID: nil},
	})
	if got.joinExisting {
		t.Fatalf("want new cluster when no candidate has a cluster, got %+v", got)
	}
}

func TestDecideClusterAssignment_NamedClusterWithinAutoAccept(t *testing.T) {
	got := decideClusterAssignment([]clusterCandidate{
		{distance: faceAutoAcceptL2Sq - 0.01, clusterID: i64(42), contactID: i64(7)},
	})
	if !got.joinExisting || got.clusterID != 42 || !got.viaAutoAccept {
		t.Fatalf("want auto-accept join to cluster 42, got %+v", got)
	}
}

func TestDecideClusterAssignment_NamedClusterAtExactBoundaryAccepts(t *testing.T) {
	// <= threshold, not strictly <, must still auto-accept.
	got := decideClusterAssignment([]clusterCandidate{
		{distance: faceAutoAcceptL2Sq, clusterID: i64(42), contactID: i64(7)},
	})
	if !got.joinExisting || !got.viaAutoAccept {
		t.Fatalf("want auto-accept join at exact threshold, got %+v", got)
	}
}

func TestDecideClusterAssignment_NamedClusterJustOverAutoAcceptFallsToSameCluster(t *testing.T) {
	// Just over the auto-accept threshold, but still within the looser
	// same-cluster threshold: should join, but NOT via auto-accept.
	got := decideClusterAssignment([]clusterCandidate{
		{distance: faceAutoAcceptL2Sq + 0.01, clusterID: i64(42), contactID: i64(7)},
	})
	if !got.joinExisting || got.clusterID != 42 || got.viaAutoAccept {
		t.Fatalf("want same-cluster (non-auto-accept) join to cluster 42, got %+v", got)
	}
}

func TestDecideClusterAssignment_UnnamedClusterWithinSameClusterThreshold(t *testing.T) {
	got := decideClusterAssignment([]clusterCandidate{
		{distance: faceSameClusterL2Sq - 0.01, clusterID: i64(99), contactID: nil},
	})
	if !got.joinExisting || got.clusterID != 99 || got.viaAutoAccept {
		t.Fatalf("want plain join to cluster 99, got %+v", got)
	}
}

func TestDecideClusterAssignment_BeyondAllThresholdsStartsNewCluster(t *testing.T) {
	got := decideClusterAssignment([]clusterCandidate{
		{distance: faceSameClusterL2Sq + 0.01, clusterID: i64(99), contactID: nil},
		{distance: faceSameClusterL2Sq + 0.5, clusterID: i64(7), contactID: i64(3)},
	})
	if got.joinExisting {
		t.Fatalf("want new cluster when every candidate is beyond threshold, got %+v", got)
	}
}

func TestDecideClusterAssignment_PrefersNamedOverCloserUnnamed(t *testing.T) {
	// An unnamed cluster is numerically closer, but a named cluster is still
	// within auto-accept range — named must win per the documented priority.
	got := decideClusterAssignment([]clusterCandidate{
		{distance: 0.1, clusterID: i64(10), contactID: nil}, // closer, unnamed
		{distance: 0.5, clusterID: i64(20), contactID: i64(1)}, // named, still within auto-accept
	})
	if !got.joinExisting || got.clusterID != 20 || !got.viaAutoAccept {
		t.Fatalf("want auto-accept join to named cluster 20 despite a closer unnamed match, got %+v", got)
	}
}

func TestDecideClusterAssignment_NamedTooFarFallsBackToCloserUnnamed(t *testing.T) {
	// The named cluster is beyond even the same-cluster threshold, so it's
	// not viable at all; a closer unnamed cluster within range should win.
	got := decideClusterAssignment([]clusterCandidate{
		{distance: 0.3, clusterID: i64(10), contactID: nil}, // unnamed, within same-cluster range
		{distance: faceSameClusterL2Sq + 1, clusterID: i64(20), contactID: i64(1)}, // named but far
	})
	if !got.joinExisting || got.clusterID != 10 || got.viaAutoAccept {
		t.Fatalf("want plain join to unnamed cluster 10, got %+v", got)
	}
}

func TestDecideClusterAssignment_IgnoresCandidatesWithoutCluster(t *testing.T) {
	// The nearest candidate has no cluster yet (skip it); the next one does
	// and is within range.
	got := decideClusterAssignment([]clusterCandidate{
		{distance: 0.01, clusterID: nil, contactID: nil},
		{distance: 0.9, clusterID: i64(55), contactID: nil},
	})
	if !got.joinExisting || got.clusterID != 55 {
		t.Fatalf("want join to cluster 55 skipping the unclustered nearer match, got %+v", got)
	}
}

func TestDecideClusterAssignment_OrderIndependent(t *testing.T) {
	a := []clusterCandidate{
		{distance: 0.5, clusterID: i64(20), contactID: i64(1)},
		{distance: 0.1, clusterID: i64(10), contactID: nil},
	}
	b := []clusterCandidate{a[1], a[0]} // reversed
	gotA := decideClusterAssignment(a)
	gotB := decideClusterAssignment(b)
	if gotA != gotB {
		t.Fatalf("decision should not depend on candidate order: got %+v vs %+v", gotA, gotB)
	}
}
