package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/sqlutil"
)

// SimilarEmailsByText POST /emails/similar-by-text — embed query text and return nearest emails for this user.
func (h *EmailHandler) SimilarEmailsByText(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorEmails(w, r) {
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
		SELECT e.id, e.date, e.from_address, e.to_addresses, e.subject, e.snippet, e.plain_text, e.has_attachments, emb.distance
		FROM email_embeddings emb
		JOIN emails e ON e.id = emb.rowid
		WHERE emb.embedding MATCH ? AND emb.k = ?
		  AND COALESCE(e.user_id, 0) = ?
		  AND e.user_deleted = FALSE
		ORDER BY emb.distance ASC
		LIMIT ?`, vecBlob, vecKCandidate(req.N), uid, req.N)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("vector search failed: %v", err))
		return
	}
	defer rows.Close()

	results := make([]map[string]any, 0, req.N)
	for rows.Next() {
		var (
			id             int64
			date           sqlutil.NullDBTime
			fromAddr       *string
			toAddrs        *string
			subject        *string
			snippet        *string
			plainText      *string
			hasAttachments bool
			distance       any
		)
		if err := rows.Scan(&id, &date, &fromAddr, &toAddrs, &subject, &snippet, &plainText, &hasAttachments, &distance); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("scan: %v", err))
			return
		}
		preview := emailSimilarityPreview(snippet, plainText)
		row := map[string]any{
			"id":              id,
			"from_address":    derefString(fromAddr),
			"to_addresses":    derefString(toAddrs),
			"subject":         derefString(subject),
			"snippet":         preview,
			"plain_text":      preview,
			"has_attachments": hasAttachments,
			"distance":        jsonSafeVecDistance(distance),
		}
		if date.Valid {
			row["date"] = date.Time.Format(time.RFC3339)
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("iterate: %v", err))
		return
	}
	writeJSON(w, map[string]any{"results": results, "count": len(results), "query_normalized": norm})
}

func emailSimilarityPreview(snippet, plainText *string) string {
	if snippet != nil {
		s := strings.TrimSpace(*snippet)
		if s != "" {
			return s
		}
	}
	if plainText != nil {
		s := strings.TrimSpace(*plainText)
		if s == "" {
			return ""
		}
		const maxPreview = 600
		if len(s) <= maxPreview {
			return s
		}
		return s[:maxPreview] + "…"
	}
	return ""
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
