package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/importer"
	"github.com/daveontour/aimuseum/internal/service"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
)

var artefactEmbeddingJob = importer.NewImportJob("Artefact embeddings", map[string]any{
	"status": "idle", "status_line": nil, "error_message": nil,
	"total": 0, "processed": 0, "embedded": 0, "skipped": 0, "errors": 0,
})

func (h *ArtefactHandler) ArtefactEmbeddingsBackfillStart(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	if err := artefactEmbeddingJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}
	if h.embeddingSvc == nil || !h.embeddingSvc.IsAvailable() {
		writeError(w, http.StatusServiceUnavailable, "embedding service not available — set LOCALAI_BASE_URL and LOCALAI_EMBEDDING_MODEL")
		return
	}
	var reqBody struct {
		ReprocessAll bool `json:"reprocess_all"`
	}
	_ = json.NewDecoder(r.Body).Decode(&reqBody)

	uid := appctx.UserIDFromCtx(r.Context())
	artefactEmbeddingJob.Start()
	modeLine := "fill missing embeddings only"
	if reqBody.ReprocessAll {
		modeLine = "reprocess all artefacts"
	}
	artefactEmbeddingJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting artefact embedding backfill (" + modeLine + ")...",
		"total": 0, "processed": 0, "embedded": 0, "skipped": 0, "errors": 0, "error_message": nil,
		"reprocess_all": reqBody.ReprocessAll,
	})
	artefactEmbeddingJob.Broadcast("status", map[string]any{"status_line": "Starting artefact embedding backfill (" + modeLine + ")..."})
	go runArtefactEmbeddingBackfill(h.svc, h.pool, artefactEmbeddingJob, uid, reqBody.ReprocessAll)
	writeJSON(w, map[string]any{"message": "Artefact embedding backfill started", "status": "started", "reprocess_all": reqBody.ReprocessAll})
}

func (h *ArtefactHandler) ArtefactEmbeddingsBackfillStream(w http.ResponseWriter, r *http.Request) {
	artefactEmbeddingJob.ServeSSE(w, r)
}

func (h *ArtefactHandler) ArtefactEmbeddingsBackfillCancel(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	writeJSON(w, artefactEmbeddingJob.Cancel())
}

func (h *ArtefactHandler) ArtefactEmbeddingsBackfillStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, artefactEmbeddingJob.Status())
}

func runArtefactEmbeddingBackfill(svc *service.ArtefactService, pool *sql.DB, job *importer.ImportJob, uid int64, reprocessAll bool) {
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
	defer job.Finish()

	rows, err := svc.ListForEmbeddingBackfill(ctx, !reprocessAll)
	if err != nil {
		job.UpdateState(map[string]any{
			"status": "failed", "status_line": fmt.Sprintf("Failed to list artefacts: %v", err),
			"error_message": err.Error(),
		})
		job.Broadcast("failed", job.GetState())
		return
	}

	total := len(rows)
	job.UpdateState(map[string]any{
		"total": total, "processed": 0, "embedded": 0, "skipped": 0, "errors": 0,
		"status_line":   fmt.Sprintf("Artefact embeddings: %d row(s) queued", total),
		"reprocess_all": reprocessAll,
	})
	job.Broadcast("progress", job.GetState())

	embedded, skipped, errorsCount := 0, 0, 0
	for i, row := range rows {
		if job.IsCancelled() {
			_ = recordImportControlLastRun(ctx, pool, uid, "artefact_embeddings", "cancelled", "cancelled")
			job.UpdateState(map[string]any{
				"status": "cancelled", "status_line": "Artefact embedding backfill cancelled.",
				"processed": i, "embedded": embedded, "skipped": skipped, "errors": errorsCount,
			})
			job.Broadcast("cancelled", job.GetState())
			return
		}
		input := service.BuildArtefactEmbeddingInput(row.Name, row.Description, row.Tags, row.Story)
		if input == "" {
			skipped++
			job.UpdateState(map[string]any{
				"processed": i + 1, "embedded": embedded, "skipped": skipped, "errors": errorsCount,
				"status_line": fmt.Sprintf("Processed %d/%d (skipped empty text)", i+1, total),
			})
			job.Broadcast("progress", job.GetState())
			continue
		}
		svc.SyncArtefactEmbedding(ctx, row.ID)
		embedded++
		job.UpdateState(map[string]any{
			"processed": i + 1, "embedded": embedded, "skipped": skipped, "errors": errorsCount,
			"status_line": fmt.Sprintf("Processed %d/%d (embedded artefact id %d)", i+1, total, row.ID),
		})
		job.Broadcast("progress", job.GetState())
	}

	if job.IsCancelled() {
		return
	}
	_ = recordImportControlLastRun(ctx, pool, uid, "artefact_embeddings", "completed", "")
	job.UpdateState(map[string]any{
		"status":      "completed",
		"status_line": fmt.Sprintf("Artefact embedding backfill complete: %d embedded, %d skipped (empty text), %d errors", embedded, skipped, errorsCount),
		"processed":   total,
		"embedded":    embedded,
		"skipped":     skipped,
		"errors":      errorsCount,
	})
	job.Broadcast("completed", job.GetState())
}

func (h *ArtefactHandler) SimilarArtefactsByText(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}
	if h.embeddingSvc == nil || !h.embeddingSvc.IsAvailable() {
		writeError(w, http.StatusServiceUnavailable, "embedding service not available — set LOCALAI_BASE_URL and LOCALAI_EMBEDDING_MODEL")
		return
	}
	var req struct {
		Text string `json:"text"`
		N    int    `json:"n"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	norm := normalizeFreeTextForEmbedding(req.Text)
	if norm == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	if req.N <= 0 {
		req.N = 25
	}
	if req.N > 50 {
		req.N = 50
	}
	ctx := r.Context()
	uid := appctx.UserIDFromCtx(ctx)
	if uid == 0 {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	vec, err := h.embeddingSvc.EmbedText(ctx, norm)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("embedding failed: %v", err))
		return
	}
	vecBlob, err := sqlite_vec.SerializeFloat32(vec)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("serialize embedding: %v", err))
		return
	}
	rows, err := h.pool.QueryContext(ctx, `
		SELECT a.id, a.name, a.description, a.tags, a.story, emb.distance
		FROM artefact_embeddings emb
		JOIN artefacts a ON a.id = emb.rowid
		WHERE emb.embedding MATCH ? AND emb.k = ?
		  AND COALESCE(a.user_id, 0) = ?
		ORDER BY emb.distance ASC
		LIMIT ?`, vecBlob, vecKCandidate(req.N), uid, req.N)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("vector search failed: %v", err))
		return
	}
	defer rows.Close()
	results := make([]map[string]any, 0, req.N)
	for rows.Next() {
		var id int64
		var name string
		var description, tags, story *string
		var distance any
		if err := rows.Scan(&id, &name, &description, &tags, &story, &distance); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("scan: %v", err))
			return
		}
		results = append(results, map[string]any{
			"id":          id,
			"name":        name,
			"description": description,
			"tags":        tags,
			"story":       story,
			"distance":    jsonSafeVecDistance(distance),
		})
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("iterate: %v", err))
		return
	}
	writeJSON(w, map[string]any{"results": results, "count": len(results), "query_normalized": norm})
}
