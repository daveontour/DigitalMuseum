package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/sqlutil"
)

const artefactEmbeddingsVecTable = "artefact_embeddings"

// BuildArtefactEmbeddingInput concatenates name, description, tags, and story for embedding.
func BuildArtefactEmbeddingInput(name string, description, tags, story *string) string {
	var parts []string
	if n := strings.TrimSpace(name); n != "" {
		parts = append(parts, "Name: "+n)
	}
	if description != nil {
		if d := normalizeArtefactTextField(*description); d != "" {
			parts = append(parts, "Description: "+d)
		}
	}
	if tags != nil {
		if t := NormalizeTagsForEmbedding(*tags); t != "" {
			parts = append(parts, "Tags: "+t)
		}
	}
	if story != nil {
		if s := normalizeArtefactTextField(*story); s != "" {
			parts = append(parts, "Story: "+s)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func normalizeArtefactTextField(raw string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
}

// ArtefactEmbeddingSignature is a stable fingerprint stored in vec int_ids for skip-on-backfill.
func ArtefactEmbeddingSignature(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}

// ArtefactEmbeddingHelper upserts or deletes sqlite-vec rows for artefact text fields.
type ArtefactEmbeddingHelper struct {
	pool *sql.DB
	repo *repository.ArtefactRepo
	svc  *EmbeddingService
}

// NewArtefactEmbeddingHelper creates a helper; svc may be nil to disable sync.
func NewArtefactEmbeddingHelper(pool *sql.DB, repo *repository.ArtefactRepo, svc *EmbeddingService) *ArtefactEmbeddingHelper {
	if pool == nil || repo == nil {
		return nil
	}
	return &ArtefactEmbeddingHelper{pool: pool, repo: repo, svc: svc}
}

// Sync refreshes or removes the vec row for one artefact from current DB fields.
func (h *ArtefactEmbeddingHelper) Sync(ctx context.Context, artefactID int64) {
	if h == nil || h.pool == nil || h.repo == nil {
		return
	}
	if h.svc == nil || !h.svc.IsAvailable() {
		return
	}
	a, err := h.repo.GetByID(ctx, artefactID)
	if err != nil || a == nil {
		return
	}
	input := BuildArtefactEmbeddingInput(a.Name, a.Description, a.Tags, a.Story)
	if input == "" {
		h.Delete(ctx, artefactID)
		return
	}
	vec, err := h.svc.EmbedText(ctx, input)
	if err != nil {
		slog.Warn("artefact embedding embed", "artefact_id", artefactID, "err", err)
		return
	}
	blob, err := sqlite_vec.SerializeFloat32(vec)
	if err != nil {
		slog.Warn("artefact embedding serialize", "artefact_id", artefactID, "err", err)
		return
	}
	sig := ArtefactEmbeddingSignature(input)
	if err := sqlutil.Vec0Upsert(ctx, h.pool, artefactEmbeddingsVecTable, artefactID, blob, sig); err != nil {
		slog.Warn("artefact embedding upsert", "artefact_id", artefactID, "err", err)
	}
}

// Delete removes the vec row for an artefact (best-effort).
func (h *ArtefactEmbeddingHelper) Delete(ctx context.Context, artefactID int64) {
	if h == nil || h.pool == nil {
		return
	}
	if _, err := h.pool.ExecContext(ctx, `DELETE FROM `+artefactEmbeddingsVecTable+` WHERE rowid = ?`, artefactID); err != nil {
		slog.Warn("artefact embedding delete", "artefact_id", artefactID, "err", err)
	}
}

// ArtefactEmbeddingSignatureMatches reports whether the vec row matches the current input fingerprint.
func ArtefactEmbeddingSignatureMatches(ctx context.Context, pool *sql.DB, artefactID int64, input string) (bool, error) {
	if pool == nil || input == "" {
		return false, nil
	}
	want := ArtefactEmbeddingSignature(input)
	var got string
	err := pool.QueryRowContext(ctx,
		`SELECT int_ids FROM `+artefactEmbeddingsVecTable+` WHERE rowid = ?`, artefactID,
	).Scan(&got)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return got != "" && got == want, nil
}
