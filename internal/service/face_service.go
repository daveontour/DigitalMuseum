package service

import (
	"context"
	"fmt"

	"github.com/daveontour/aimuseum/internal/repository"
)

// Clustering thresholds, expressed as squared-L2 distance between
// L2-normalized (unit) face embeddings — see NormalizeFaceEmbedding and
// TestL2DistanceMatchesCosineSimilarityForUnitVectors for why that distance
// behaves like cosine similarity here. Start as fixed constants; promote to
// per-archive app_configuration values later only if real data shows they
// need tuning.
const (
	// faceAutoAcceptL2Sq is how close a face's nearest match must be to an
	// already-named cluster (one linked to a Contact) to auto-join it without
	// review. Tighter than faceSameClusterL2Sq: a wrong auto-accept silently
	// mislabels a photo under someone's real name, which is worse than a
	// wrong same-cluster grouping (still reviewable, unnamed).
	faceAutoAcceptL2Sq = 0.8

	// faceSameClusterL2Sq is how close a face's nearest match must be to join
	// that face's cluster (named or not) when auto-accept didn't apply.
	faceSameClusterL2Sq = 1.1
)

// FaceService owns face/cluster business logic on top of FaceRepo and the
// face_embeddings vec0 table (via FaceEmbeddingHelper).
type FaceService struct {
	repo  *repository.FaceRepo
	embed *FaceEmbeddingHelper
}

// NewFaceService creates a FaceService. embed may be nil to disable
// clustering (detection can still run and store raw embeddings via a
// separately obtained FaceEmbeddingHelper — this only affects ClusterUnassignedFaces).
func NewFaceService(repo *repository.FaceRepo, embed *FaceEmbeddingHelper) *FaceService {
	return &FaceService{repo: repo, embed: embed}
}

// clusterCandidate is a face_embeddings nearest-neighbor match, enriched with
// the candidate face's current cluster/contact assignment — everything
// decideClusterAssignment needs to make a pure, DB-free decision.
type clusterCandidate struct {
	distance  float64
	clusterID *int64
	contactID *int64
}

// clusterDecision is decideClusterAssignment's verdict for one face.
type clusterDecision struct {
	joinExisting  bool
	clusterID     int64 // valid iff joinExisting
	viaAutoAccept bool  // true if joined via the named-cluster auto-accept path, vs. the looser same-cluster path
}

// decideClusterAssignment picks what a face with the given nearest-neighbor
// candidates should do: join an already-named cluster (auto-accept), join
// any existing cluster (named or not), or start a new one. candidates need
// not be pre-sorted; this function only considers each candidate's own
// distance and does not depend on input order. Candidates with a nil
// clusterID (the matched face itself has no cluster yet — e.g. a batch
// processing two mutually-nearest faces in the same run, the second of which
// hasn't been assigned yet) are skipped: there is nothing to join.
//
// A pure function so clustering behavior can be unit tested at threshold
// boundaries without a database.
func decideClusterAssignment(candidates []clusterCandidate) clusterDecision {
	var bestNamed *clusterCandidate
	var bestAny *clusterCandidate

	for i := range candidates {
		c := &candidates[i]
		if c.clusterID == nil {
			continue
		}
		if bestAny == nil || c.distance < bestAny.distance {
			bestAny = c
		}
		if c.contactID != nil && (bestNamed == nil || c.distance < bestNamed.distance) {
			bestNamed = c
		}
	}

	if bestNamed != nil && bestNamed.distance <= faceAutoAcceptL2Sq {
		return clusterDecision{joinExisting: true, clusterID: *bestNamed.clusterID, viaAutoAccept: true}
	}
	if bestAny != nil && bestAny.distance <= faceSameClusterL2Sq {
		return clusterDecision{joinExisting: true, clusterID: *bestAny.clusterID}
	}
	return clusterDecision{joinExisting: false}
}

// ClusterRunStats summarizes one ClusterUnassignedFaces run.
type ClusterRunStats struct {
	Processed   int
	JoinedNamed int // joined an already-named cluster (auto-accept path)
	JoinedOther int // joined an existing but unnamed cluster
	NewClusters int
	Errors      int
}

// ClusterUnassignedFaces processes every face without a cluster: finds its
// nearest neighbors via face_embeddings, decides whether to join an existing
// cluster or start a new one, and applies that decision. Faces are processed
// one at a time (not batched) so each face's own assignment is immediately
// visible to the similarity search for faces processed later in the same run
// — this is what lets several faces of the same newly-seen person converge
// onto one cluster within a single run, not just across runs.
func (s *FaceService) ClusterUnassignedFaces(ctx context.Context) (*ClusterRunStats, error) {
	if s.embed == nil {
		return nil, fmt.Errorf("cluster unassigned faces: embedding helper not configured")
	}

	ids, err := s.repo.ListUnclusteredFaceIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("cluster unassigned faces: list: %w", err)
	}

	stats := &ClusterRunStats{}
	const k = 5

	for _, faceID := range ids {
		stats.Processed++

		matches, err := s.embed.FindSimilarFaces(ctx, faceID, k)
		if err != nil {
			stats.Errors++
			continue
		}

		candidates := make([]clusterCandidate, 0, len(matches))
		for _, m := range matches {
			f, err := s.repo.GetFace(ctx, m.FaceID)
			if err != nil || f == nil {
				continue
			}
			candidates = append(candidates, clusterCandidate{
				distance:  m.Distance,
				clusterID: f.FaceClusterID,
				contactID: f.ContactID,
			})
		}

		decision := decideClusterAssignment(candidates)

		if decision.joinExisting {
			if err := s.repo.AssignFaceToCluster(ctx, faceID, decision.clusterID); err != nil {
				stats.Errors++
				continue
			}
			if decision.viaAutoAccept {
				stats.JoinedNamed++
			} else {
				stats.JoinedOther++
			}
			continue
		}

		clusterID, err := s.repo.CreateCluster(ctx, faceID)
		if err != nil {
			stats.Errors++
			continue
		}
		if err := s.repo.SetNewClusterRepresentativeFace(ctx, faceID, clusterID); err != nil {
			stats.Errors++
			continue
		}
		stats.NewClusters++
	}

	return stats, nil
}

// LinkClusterToContact links a cluster to a Contact (or unlinks it, when
// contactID is nil), propagating to every member face via FaceRepo.
func (s *FaceService) LinkClusterToContact(ctx context.Context, clusterID int64, contactID *int64) error {
	return s.repo.SetClusterContact(ctx, clusterID, contactID)
}

// DetachFace removes a face from its cluster (the "not this person" action).
// The next ClusterUnassignedFaces run will re-home it.
func (s *FaceService) DetachFace(ctx context.Context, faceID int64) error {
	return s.repo.DetachFace(ctx, faceID)
}

// IgnoreFace removes a face from its cluster and permanently excludes it
// from future clustering runs (a false positive, or a face the user simply
// doesn't want tracked) — unlike DetachFace, it is never re-homed.
func (s *FaceService) IgnoreFace(ctx context.Context, faceID int64) error {
	return s.repo.IgnoreFace(ctx, faceID)
}

// MoveFaceToCluster reassigns a face to a different existing cluster.
func (s *FaceService) MoveFaceToCluster(ctx context.Context, faceID, clusterID int64) error {
	return s.repo.AssignFaceToCluster(ctx, faceID, clusterID)
}
