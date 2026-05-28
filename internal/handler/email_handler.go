// Package handler contains HTTP request handlers.
package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// EmailHandler handles all /emails/* read endpoints.
type EmailHandler struct {
	svc          *service.EmailService
	sessionStore *keystore.SessionMasterStore
	pool         *sql.DB
	embeddingSvc *service.EmbeddingService
}

// NewEmailHandler creates an EmailHandler.
func NewEmailHandler(svc *service.EmailService, sessionStore *keystore.SessionMasterStore, pool *sql.DB, embeddingSvc *service.EmbeddingService) *EmailHandler {
	return &EmailHandler{svc: svc, sessionStore: sessionStore, pool: pool, embeddingSvc: embeddingSvc}
}

// RegisterRoutes mounts all email routes onto r.
func (h *EmailHandler) RegisterRoutes(r chi.Router) {
	// Specific sub-paths before the parameterised {email_id} routes to avoid shadowing
	r.Get("/emails/folders", h.GetFolders)
	r.Get("/emails/label", h.GetByLabel)
	r.Get("/emails/search", h.Search)
	r.Post("/emails/similar-by-text", h.SimilarEmailsByText)
	r.Delete("/emails/bulk-delete", h.BulkDelete)

	r.Get("/emails/{email_id}/html", h.GetHTML)
	r.Get("/emails/{email_id}/text", h.GetText)
	r.Get("/emails/{email_id}/snippet", h.GetSnippet)
	r.Get("/emails/{email_id}/metadata", h.GetMetadata)
	r.Put("/emails/{email_id}", h.Update)
	r.Delete("/emails/{email_id}", h.Delete)

	r.Post("/emails/thread/{participant}/summarize", h.SummarizeThread)
	r.Post("/emails/thread/{participant}/prompt", h.ThreadPrompt)
}

// ── Read endpoints ─────────────────────────────────────────────────────────────

// GetFolders handles GET /emails/folders
// Returns distinct folder/label names from imported emails.
func (h *EmailHandler) GetFolders(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorEmails(w, r) {
		return
	}
	folders, err := h.svc.GetFolders(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"folders": folders})
}

// GetByLabel handles GET /emails/label?labels=INBOX&labels=IMPORTANT
func (h *EmailHandler) GetByLabel(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorEmails(w, r) {
		return
	}
	labels := r.URL.Query()["labels"]
	if len(labels) == 0 {
		writeError(w, http.StatusBadRequest,
			"at least one label must be provided as query parameter (e.g., ?labels=INBOX&labels=IMPORTANT)")
		return
	}

	result, err := h.svc.GetByLabels(r.Context(), labels)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error fetching emails: %s", err))
		return
	}
	writeJSON(w, result)
}

// Search handles GET /emails/search with optional query parameters.
func (h *EmailHandler) Search(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorEmails(w, r) {
		return
	}
	q := r.URL.Query()

	params := model.EmailSearchParams{}

	if v := q.Get("from_address"); v != "" {
		params.FromAddress = &v
	}
	if v := q.Get("to_address"); v != "" {
		params.ToAddress = &v
	}
	if v := q.Get("subject"); v != "" {
		params.Subject = &v
	}
	if v := q.Get("to_from"); v != "" {
		params.ToFrom = &v
	}
	if v := q.Get("month"); v != "" {
		m, err := strconv.Atoi(v)
		if err != nil || m < 1 || m > 12 {
			writeError(w, http.StatusBadRequest, "month must be an integer between 1 and 12")
			return
		}
		params.Month = &m
	}
	if v := q.Get("year"); v != "" {
		yr, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "year must be an integer")
			return
		}
		params.Year = &yr
	}
	if v := q.Get("has_attachments"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "has_attachments must be true or false")
			return
		}
		params.HasAttachments = &b
	}
	if v := strings.TrimSpace(q.Get("email_source")); v != "" && strings.ToLower(v) != "all" {
		params.SourceFilter = &v
	}

	result, err := h.svc.Search(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error searching emails: %s", err))
		return
	}
	writeJSON(w, result)
}

// GetHTML handles GET /emails/{email_id}/html
// Returns the raw HTML message; falls back to plain_text wrapped in minimal HTML.
func (h *EmailHandler) GetHTML(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorEmails(w, r) {
		return
	}
	id, ok := parseEmailID(w, r)
	if !ok {
		return
	}

	email, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error fetching email: %s", err))
		return
	}
	if email == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("email with ID %d not found", id))
		return
	}

	if email.RawMessage != nil && *email.RawMessage != "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(*email.RawMessage))
		return
	}

	if email.PlainText != nil && *email.PlainText != "" {
		html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <style>
        body {
            font-family: Arial, sans-serif;
            line-height: 1.6;
            max-width: 800px;
            margin: 20px auto;
            padding: 20px;
            white-space: pre-wrap;
            word-wrap: break-word;
        }
    </style>
</head>
<body>
%s
</body>
</html>`, escapeHTML(*email.PlainText))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
		return
	}

	writeError(w, http.StatusNotFound, fmt.Sprintf("email with ID %d has no HTML or text content", id))
}

// GetText handles GET /emails/{email_id}/text
func (h *EmailHandler) GetText(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorEmails(w, r) {
		return
	}
	id, ok := parseEmailID(w, r)
	if !ok {
		return
	}

	email, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error fetching email: %s", err))
		return
	}
	if email == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("email with ID %d not found", id))
		return
	}
	if email.PlainText == nil || *email.PlainText == "" {
		writeError(w, http.StatusNotFound, fmt.Sprintf("email with ID %d has no text content", id))
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(*email.PlainText))
}

// GetSnippet handles GET /emails/{email_id}/snippet
func (h *EmailHandler) GetSnippet(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorEmails(w, r) {
		return
	}
	id, ok := parseEmailID(w, r)
	if !ok {
		return
	}

	email, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error fetching email: %s", err))
		return
	}
	if email == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("email with ID %d not found", id))
		return
	}
	if email.Snippet == nil || *email.Snippet == "" {
		writeError(w, http.StatusNotFound, fmt.Sprintf("email with ID %d has no snippet", id))
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(*email.Snippet))
}

// GetMetadata handles GET /emails/{email_id}/metadata
func (h *EmailHandler) GetMetadata(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorEmails(w, r) {
		return
	}
	id, ok := parseEmailID(w, r)
	if !ok {
		return
	}

	resp, err := h.svc.GetMetadata(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error fetching email metadata: %s", err))
		return
	}
	if resp == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("email with ID %d not found", id))
		return
	}
	writeJSON(w, resp)
}

// ── Write endpoints ────────────────────────────────────────────────────────────

// Update handles PUT /emails/{email_id}
// Body: {"is_personal": bool, "is_business": bool, "is_important": bool, "use_by_ai": bool} (all optional)
func (h *EmailHandler) Update(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	id, ok := parseEmailID(w, r)
	if !ok {
		return
	}

	var body struct {
		IsPersonal  *bool `json:"is_personal"`
		IsBusiness  *bool `json:"is_business"`
		IsImportant *bool `json:"is_important"`
		UseByAI     *bool `json:"use_by_ai"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	found, err := h.svc.Update(r.Context(), id, service.EmailUpdateParams{
		IsPersonal:  body.IsPersonal,
		IsBusiness:  body.IsBusiness,
		IsImportant: body.IsImportant,
		UseByAI:     body.UseByAI,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error updating email: %s", err))
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, fmt.Sprintf("email with ID %d not found", id))
		return
	}
	writeJSON(w, map[string]string{"message": "Email updated successfully"})
}

// Delete handles DELETE /emails/{email_id}
func (h *EmailHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	id, ok := parseEmailID(w, r)
	if !ok {
		return
	}

	found, err := h.svc.SoftDelete(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error deleting email: %s", err))
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, fmt.Sprintf("email with ID %d not found", id))
		return
	}
	writeJSON(w, map[string]string{"message": "Email deleted successfully"})
}

// BulkDelete handles DELETE /emails/bulk-delete
// Body: {"email_ids": [1, 2, 3]}
func (h *EmailHandler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	var body struct {
		EmailIDs []int64 `json:"email_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(body.EmailIDs) == 0 {
		writeError(w, http.StatusBadRequest, "email_ids must be a non-empty list")
		return
	}

	deleted, err := h.svc.BulkSoftDelete(r.Context(), body.EmailIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error deleting emails: %s", err))
		return
	}
	writeJSON(w, map[string]any{
		"message":       fmt.Sprintf("Deleted %d emails", deleted),
		"deleted_count": deleted,
	})
}

// SummarizeThread handles POST /emails/thread/{participant}/summarize
func (h *EmailHandler) SummarizeThread(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorEmails(w, r) {
		return
	}
	participant := chi.URLParam(r, "participant")
	if participant == "" {
		writeError(w, http.StatusBadRequest, "participant is required")
		return
	}

	summary, err := h.svc.SummarizeThread(r.Context(), participant)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error summarizing thread: %s", err))
		return
	}
	writeJSON(w, map[string]string{"summary": summary})
}

const maxEmailThreadPromptInstructionBytes = 32 * 1024

// ThreadPrompt handles POST /emails/thread/{participant}/prompt
// JSON body: { "instruction": "..." } — same thread transcript as /summarize, with a custom instruction.
func (h *EmailHandler) ThreadPrompt(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorEmails(w, r) {
		return
	}
	participant := chi.URLParam(r, "participant")
	if participant == "" {
		writeError(w, http.StatusBadRequest, "participant is required")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxEmailThreadPromptInstructionBytes+64*1024))
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read body")
		return
	}
	var req struct {
		Instruction string `json:"instruction"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	instruction := strings.TrimSpace(req.Instruction)
	if instruction == "" {
		writeError(w, http.StatusBadRequest, "instruction is required")
		return
	}
	if len(instruction) > maxEmailThreadPromptInstructionBytes {
		writeError(w, http.StatusBadRequest, "instruction exceeds maximum length")
		return
	}

	response, err := h.svc.RunEmailThreadPrompt(r.Context(), participant, instruction)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error running email thread prompt: %s", err))
		return
	}
	writeJSON(w, map[string]any{"response": response})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func parseEmailID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "email_id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "email_id must be an integer")
		return 0, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Header already sent — can't change status; log would happen via middleware
		_ = err
	}
}

func writeError(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"detail": detail, "error": detail})
}

// escapeHTML replaces the five HTML special characters.
// Used when wrapping plain_text inside an HTML body.
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&#34;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}
