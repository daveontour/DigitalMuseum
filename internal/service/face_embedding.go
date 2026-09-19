package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/daveontour/aimuseum/internal/sqlutil"
)

const faceEmbeddingsVecTable = "face_embeddings"

// NormalizeFaceEmbedding returns vec scaled to unit length (L2 norm 1).
// vec0's built-in distance for the "embedding MATCH ?" KNN operator is L2
// (Euclidean); for unit vectors, L2² = 2 − 2·cos_sim, so normalizing here is
// what makes that distance behave like cosine similarity for face matching.
// Returns vec unchanged if its norm is zero (a degenerate all-zero embedding
// — should not happen with a real recognizer, but avoids a NaN/Inf result).
func NormalizeFaceEmbedding(vec []float32) []float32 {
	var sumSq float64
	for _, v := range vec {
		sumSq += float64(v) * float64(v)
	}
	norm := math.Sqrt(sumSq)
	if norm == 0 {
		return vec
	}
	out := make([]float32, len(vec))
	for i, v := range vec {
		out[i] = float32(float64(v) / norm)
	}
	return out
}

// FaceEmbeddingHelper upserts or deletes sqlite-vec rows for detected faces
// (media_item_faces), modeled directly on MediaTagEmbeddingHelper.
type FaceEmbeddingHelper struct {
	pool *sql.DB
}

// NewFaceEmbeddingHelper creates a helper. pool may not be nil.
func NewFaceEmbeddingHelper(pool *sql.DB) *FaceEmbeddingHelper {
	if pool == nil {
		return nil
	}
	return &FaceEmbeddingHelper{pool: pool}
}

// Sync normalizes rawEmbedding, serializes it, and upserts it into
// face_embeddings keyed by faceID (rowid). mediaItemID is stored in int_ids
// as a one-element JSON array, matching the shape Vec0Upsert's callers use
// elsewhere, even though a face embedding never fans out to more than one row.
func (h *FaceEmbeddingHelper) Sync(ctx context.Context, faceID, mediaItemID int64, rawEmbedding []float32) error {
	if h == nil || h.pool == nil {
		return fmt.Errorf("face embedding sync: helper not configured")
	}
	if len(rawEmbedding) == 0 {
		return fmt.Errorf("face embedding sync: empty embedding for face %d", faceID)
	}

	norm := NormalizeFaceEmbedding(rawEmbedding)
	blob, err := sqlite_vec.SerializeFloat32(norm)
	if err != nil {
		return fmt.Errorf("face embedding sync: serialize: %w", err)
	}

	intIDs, err := json.Marshal([]int64{mediaItemID})
	if err != nil {
		return fmt.Errorf("face embedding sync: marshal int_ids: %w", err)
	}

	if err := sqlutil.Vec0Upsert(ctx, h.pool, faceEmbeddingsVecTable, faceID, blob, string(intIDs)); err != nil {
		return fmt.Errorf("face embedding sync: upsert: %w", err)
	}
	return nil
}

// Delete removes the vec row for a face, e.g. when the face row itself is deleted.
func (h *FaceEmbeddingHelper) Delete(ctx context.Context, faceID int64) {
	if h == nil || h.pool == nil {
		return
	}
	if _, err := h.pool.ExecContext(ctx, `DELETE FROM `+faceEmbeddingsVecTable+` WHERE rowid = ?`, faceID); err != nil {
		slog.Warn("face embedding delete", "face_id", faceID, "err", err)
	}
}

// FaceMatch is one nearest-neighbor result from a face_embeddings similarity query.
type FaceMatch struct {
	FaceID   int64
	Distance float64
}

// FindSimilarFaces returns up to k faces whose embeddings are closest to
// faceID's own embedding (excluding faceID itself), ordered nearest first.
// Used by both clustering (JobFaceClustering) and live "suggest a contact for
// this cluster" lookups.
func (h *FaceEmbeddingHelper) FindSimilarFaces(ctx context.Context, faceID int64, k int) ([]FaceMatch, error) {
	if h == nil || h.pool == nil {
		return nil, fmt.Errorf("find similar faces: helper not configured")
	}

	var self []byte
	err := h.pool.QueryRowContext(ctx, `SELECT embedding FROM `+faceEmbeddingsVecTable+` WHERE rowid = ?`, faceID).Scan(&self)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find similar faces: lookup embedding: %w", err)
	}

	// k+1 because the query's own row matches itself at distance 0 and must be excluded below.
	rows, err := h.pool.QueryContext(ctx,
		`SELECT rowid, distance FROM `+faceEmbeddingsVecTable+` WHERE embedding MATCH ? AND k = ? ORDER BY distance ASC`,
		self, k+1,
	)
	if err != nil {
		return nil, fmt.Errorf("find similar faces: query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []FaceMatch
	for rows.Next() {
		var m FaceMatch
		if err := rows.Scan(&m.FaceID, &m.Distance); err != nil {
			return nil, err
		}
		if m.FaceID == faceID {
			continue
		}
		out = append(out, m)
		if len(out) >= k {
			break
		}
	}
	return out, rows.Err()
}
