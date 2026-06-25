package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

type MessageSimilarityHandler struct {
	pool         *sql.DB
	embeddingSvc *service.EmbeddingService
}

func NewMessageSimilarityHandler(pool *sql.DB, embeddingSvc *service.EmbeddingService) *MessageSimilarityHandler {
	return &MessageSimilarityHandler{
		pool:         pool,
		embeddingSvc: embeddingSvc,
	}
}

func (h *MessageSimilarityHandler) RegisterRoutes(r chi.Router) {
	r.Post("/imessages/similarity-search", h.Search)
	r.Post("/imessages/similarity-search/unique", h.SearchUnique)
}

func (h *MessageSimilarityHandler) Search(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorMessagesChat(w, r) {
		return
	}
	if h.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "message similarity search not configured")
		return
	}
	if h.embeddingSvc == nil || !h.embeddingSvc.IsAvailable() {
		writeError(w, http.StatusServiceUnavailable, "embedding service not available")
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
	queryText := strings.TrimSpace(req.Text)
	if queryText == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	if req.N <= 0 {
		req.N = 3
	}
	if req.N > 25 {
		req.N = 25
	}

	ctx := r.Context()
	uid := appctx.UserIDFromCtx(ctx)
	vec, err := h.embeddingSvc.EmbedText(ctx, queryText)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("embedding failed: %v", err))
		return
	}
	vecBlob, err := sqlite_vec.SerializeFloat32(vec)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to serialize embedding: %v", err))
		return
	}

	rows, err := h.pool.QueryContext(ctx, `
		SELECT rowid, int_ids, distance
		FROM message_embeddings
		WHERE embedding MATCH ? AND k = ?
		ORDER BY distance ASC
	`, vecBlob, req.N)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("vector search failed: %v", err))
		return
	}
	defer func() { _ = rows.Close() }()

	type resultItem struct {
		RowID    int64 `json:"row_id"`
		Distance any   `json:"distance"`
		IntIDs   []int64
		Messages []map[string]any `json:"messages"`
	}
	results := make([]resultItem, 0, req.N)
	for rows.Next() {
		var (
			rowID    int64
			intIDsJS string
			distance any
			ids      []int64
		)
		if err := rows.Scan(&rowID, &intIDsJS, &distance); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("scan match row: %v", err))
			return
		}
		if strings.TrimSpace(intIDsJS) != "" {
			if err := json.Unmarshal([]byte(intIDsJS), &ids); err != nil {
				writeError(w, http.StatusInternalServerError, fmt.Sprintf("parse int_ids for row %d: %v", rowID, err))
				return
			}
		}
		results = append(results, resultItem{
			RowID:    rowID,
			Distance: distance,
			IntIDs:   ids,
		})
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("iterate vector results: %v", err))
		return
	}
	// Important: close the vector-search cursor before issuing nested queries.
	// The SQLite pool is configured with max open conns = 1, so querying messages
	// while this cursor is still open can block.
	_ = rows.Close()

	for i := range results {
		msgs, err := h.loadMessagesByIDs(ctx, uid, results[i].IntIDs)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("load messages for row %d: %v", results[i].RowID, err))
			return
		}
		results[i].Messages = msgs
	}

	out := make([]map[string]any, 0, len(results))
	for _, r := range results {
		out = append(out, map[string]any{
			"row_id":     r.RowID,
			"distance":   r.Distance,
			"int_ids":    r.IntIDs,
			"messages":   r.Messages,
			"query_text": queryText,
		})
	}
	writeJSON(w, map[string]any{"results": out, "count": len(out)})
}

func (h *MessageSimilarityHandler) SearchUnique(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorMessagesChat(w, r) {
		return
	}
	if h.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "message similarity search not configured")
		return
	}
	if h.embeddingSvc == nil || !h.embeddingSvc.IsAvailable() {
		writeError(w, http.StatusServiceUnavailable, "embedding service not available")
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
	queryText := strings.TrimSpace(req.Text)
	if queryText == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	if req.N <= 0 {
		req.N = 3
	}
	if req.N > 25 {
		req.N = 25
	}

	ctx := r.Context()
	uid := appctx.UserIDFromCtx(ctx)
	vec, err := h.embeddingSvc.EmbedText(ctx, queryText)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("embedding failed: %v", err))
		return
	}
	vecBlob, err := sqlite_vec.SerializeFloat32(vec)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to serialize embedding: %v", err))
		return
	}

	rows, err := h.pool.QueryContext(ctx, `
		SELECT rowid, int_ids, distance
		FROM message_embeddings
		WHERE embedding MATCH ? AND k = ?
		ORDER BY distance ASC
	`, vecBlob, req.N)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("vector search failed: %v", err))
		return
	}
	defer func() { _ = rows.Close() }()

	type resultItem struct {
		RowID    int64 `json:"row_id"`
		Distance any   `json:"distance"`
		IntIDs   []int64
	}
	results := make([]resultItem, 0, req.N)
	for rows.Next() {
		var (
			rowID    int64
			intIDsJS string
			distance any
			ids      []int64
		)
		if err := rows.Scan(&rowID, &intIDsJS, &distance); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("scan match row: %v", err))
			return
		}
		if strings.TrimSpace(intIDsJS) != "" {
			if err := json.Unmarshal([]byte(intIDsJS), &ids); err != nil {
				writeError(w, http.StatusInternalServerError, fmt.Sprintf("parse int_ids for row %d: %v", rowID, err))
				return
			}
		}
		results = append(results, resultItem{
			RowID:    rowID,
			Distance: distance,
			IntIDs:   ids,
		})
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("iterate vector results: %v", err))
		return
	}
	_ = rows.Close()

	combinedIDs := make([]int64, 0, req.N*11)
	seen := make(map[int64]struct{}, req.N*11)
	for _, r := range results {
		for _, id := range r.IntIDs {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			combinedIDs = append(combinedIDs, id)
		}
	}

	messages, err := h.loadMessagesByIDs(ctx, uid, combinedIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("load unique messages failed: %v", err))
		return
	}

	out := make([]map[string]any, 0, len(results))
	for _, r := range results {
		out = append(out, map[string]any{
			"row_id":   r.RowID,
			"distance": r.Distance,
			"int_ids":  r.IntIDs,
		})
	}
	writeJSON(w, map[string]any{
		"results":         out,
		"count":           len(out),
		"query_text":      queryText,
		"unique_int_ids":  combinedIDs,
		"unique_messages": messages,
	})
}

func (h *MessageSimilarityHandler) loadMessagesByIDs(ctx context.Context, uid int64, ids []int64) ([]map[string]any, error) {
	if len(ids) == 0 {
		return []map[string]any{}, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, uid)

	q := fmt.Sprintf(`
		SELECT id, COALESCE(chat_session, ''), COALESCE(text, '')
		FROM messages
		WHERE id IN (%s)
		  AND COALESCE(user_id, 0) = ?
	`, strings.Join(placeholders, ","))
	rows, err := h.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	byID := map[int64]map[string]any{}
	for rows.Next() {
		var (
			id          int64
			chatSession string
			text        string
		)
		if err := rows.Scan(&id, &chatSession, &text); err != nil {
			return nil, err
		}
		byID[id] = map[string]any{
			"id":           id,
			"chat_session": chatSession,
			"text":         text,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		if m, ok := byID[id]; ok {
			out = append(out, m)
		}
	}
	return out, nil
}
