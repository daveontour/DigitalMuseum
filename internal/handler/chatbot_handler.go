package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// chatBotMaxAttachmentBytes caps a single uploaded file — bounds both DB growth and how much
// context-window space one attachment's extracted text can occupy.
const chatBotMaxAttachmentBytes = 15 << 20 // 15 MiB

// ChatBotHandler handles all /chatbot/* endpoints — the generic ChatBot chat modality, fully
// separate from ChatHandler's /chat/* (the persona chat).
type ChatBotHandler struct {
	svc          *service.ChatBotService
	sessionStore *keystore.SessionMasterStore
}

// NewChatBotHandler creates a ChatBotHandler.
func NewChatBotHandler(svc *service.ChatBotService, sessionStore *keystore.SessionMasterStore) *ChatBotHandler {
	return &ChatBotHandler{svc: svc, sessionStore: sessionStore}
}

// RegisterRoutes mounts the chatbot endpoints on r.
func (h *ChatBotHandler) RegisterRoutes(r chi.Router) {
	r.Get("/chatbot/context-status", h.GetContextStatus)
	r.Post("/chatbot/generate", h.Generate)
	r.Post("/chatbot/conversations", h.CreateConversation)
	r.Get("/chatbot/conversations", h.ListConversations)
	r.Get("/chatbot/conversations/{id}", h.GetConversation)
	r.Put("/chatbot/conversations/{id}", h.UpdateConversation)
	r.Delete("/chatbot/conversations/{id}", h.DeleteConversation)
	r.Get("/chatbot/conversations/{id}/turns", h.GetTurns)
	r.Post("/chatbot/conversations/{id}/clear-history", h.ClearConversationHistory)
	r.Post("/chatbot/conversations/{id}/attachments", h.UploadAttachment)
	r.Get("/chatbot/conversations/{id}/attachments", h.ListPendingAttachments)
	r.Delete("/chatbot/attachments/{attachment_id}", h.DeleteAttachment)
	r.Get("/chatbot/attachments/{attachment_id}/download", h.DownloadAttachment)
}

// GET /chatbot/context-status
func (h *ChatBotHandler) GetContextStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"llm_tools_count": h.svc.ContextStatus(r.Context(), r),
	})
}

// POST /chatbot/generate
func (h *ChatBotHandler) Generate(w http.ResponseWriter, r *http.Request) {
	var req model.ChatBotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}
	if req.Provider == "" {
		req.Provider = "auto"
	}

	resp, err := h.svc.GenerateResponse(r.Context(), r, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, resp)
}

// POST /chatbot/conversations
func (h *ChatBotHandler) CreateConversation(w http.ResponseWriter, r *http.Request) {
	var req model.ChatBotConversationCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Title == "" {
		req.Title = "New Chat"
	}

	conv, err := h.svc.CreateConversation(r.Context(), req.Title)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, chatBotConversationResponse(conv, 0))
}

// GET /chatbot/conversations
func (h *ChatBotHandler) ListConversations(w http.ResponseWriter, r *http.Request) {
	var limit *int
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = &n
		}
	}

	convs, err := h.svc.ListConversations(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	ids := make([]int64, len(convs))
	for i, c := range convs {
		ids[i] = c.ID
	}
	counts, _ := h.svc.TurnCountsBatch(r.Context(), ids)

	result := make([]map[string]any, 0, len(convs))
	for _, c := range convs {
		result = append(result, chatBotConversationResponse(c, counts[c.ID]))
	}
	writeJSON(w, result)
}

// GET /chatbot/conversations/{id}
func (h *ChatBotHandler) GetConversation(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	conv, err := h.svc.GetConversation(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if conv == nil {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}
	turns, err := h.svc.GetTurns(r.Context(), id, 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	turnsData := make([]map[string]any, 0, len(turns))
	for _, t := range turns {
		turnsData = append(turnsData, chatBotTurnResponse(t))
	}
	result := chatBotConversationResponse(conv, int64(len(turns)))
	result["turns"] = turnsData
	writeJSON(w, result)
}

// PUT /chatbot/conversations/{id}
func (h *ChatBotHandler) UpdateConversation(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req model.ChatBotConversationUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	conv, err := h.svc.UpdateConversation(r.Context(), id, req.Title)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if conv == nil {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}
	writeJSON(w, chatBotConversationResponse(conv, 0))
}

// DELETE /chatbot/conversations/{id}
func (h *ChatBotHandler) DeleteConversation(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	conv, err := h.svc.GetConversation(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if conv == nil {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}
	if err := h.svc.DeleteConversation(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"success": true, "message": "Conversation deleted successfully"})
}

// GET /chatbot/conversations/{id}/turns
func (h *ChatBotHandler) GetTurns(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	limit := 100
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	turns, err := h.svc.GetTurns(r.Context(), id, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result := make([]map[string]any, 0, len(turns))
	for _, t := range turns {
		result = append(result, chatBotTurnResponse(t))
	}
	writeJSON(w, result)
}

// POST /chatbot/conversations/{id}/clear-history
func (h *ChatBotHandler) ClearConversationHistory(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	conv, err := h.svc.GetConversation(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if conv == nil {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}
	n, err := h.svc.ClearConversationHistory(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"success": true, "turns_deleted": n})
}

// POST /chatbot/conversations/{id}/attachments — multipart, single "file" field, mirrors
// document_handler.go's Create.
func (h *ChatBotHandler) UploadAttachment(w http.ResponseWriter, r *http.Request) {
	conversationID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := r.ParseMultipartForm(chatBotMaxAttachmentBytes + (1 << 20)); err != nil {
		writeError(w, http.StatusBadRequest, "could not parse multipart form")
		return
	}
	f, fh, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(io.LimitReader(f, chatBotMaxAttachmentBytes+1))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read uploaded file")
		return
	}
	if len(data) == 0 {
		writeError(w, http.StatusBadRequest, "uploaded file is empty")
		return
	}
	if len(data) > chatBotMaxAttachmentBytes {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("file exceeds the %d MB limit", chatBotMaxAttachmentBytes/(1<<20)))
		return
	}

	ct := fh.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}

	att, err := h.svc.UploadAttachment(r.Context(), conversationID, fh.Filename, ct, data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, chatBotAttachmentResponse(att))
}

// GET /chatbot/conversations/{id}/attachments — pending (not yet sent) attachments.
func (h *ChatBotHandler) ListPendingAttachments(w http.ResponseWriter, r *http.Request) {
	conversationID, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	atts, err := h.svc.ListPendingAttachments(r.Context(), conversationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result := make([]map[string]any, 0, len(atts))
	for _, a := range atts {
		result = append(result, chatBotAttachmentResponse(a))
	}
	writeJSON(w, result)
}

// DELETE /chatbot/attachments/{attachment_id}
func (h *ChatBotHandler) DeleteAttachment(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "attachment_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid attachment id")
		return
	}
	deleted, err := h.svc.DeleteAttachment(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "attachment not found, already sent, or not owned")
		return
	}
	writeJSON(w, map[string]any{"success": true})
}

// GET /chatbot/attachments/{attachment_id}/download
func (h *ChatBotHandler) DownloadAttachment(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "attachment_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid attachment id")
		return
	}
	att, err := h.svc.DownloadAttachment(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if att == nil {
		writeError(w, http.StatusNotFound, "attachment not found")
		return
	}
	w.Header().Set("Content-Type", att.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, att.Filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(att.Data)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func chatBotConversationResponse(c *model.ChatBotConversation, turnCount int64) map[string]any {
	m := map[string]any{
		"id":              c.ID,
		"title":           c.Title,
		"provider":        c.Provider,
		"created_at":      c.CreatedAt.Format("2006-01-02T15:04:05.999999"),
		"updated_at":      c.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
		"last_message_at": nil,
		"turn_count":      turnCount,
	}
	if c.LastMessageAt.Valid {
		m["last_message_at"] = c.LastMessageAt.Time.Format("2006-01-02T15:04:05.999999")
	}
	return m
}

func chatBotTurnResponse(t *model.ChatBotTurn) map[string]any {
	return map[string]any{
		"id":             t.ID,
		"user_input":     t.UserInput,
		"response_text":  t.ResponseText,
		"provider":       t.Provider,
		"temperature":    t.Temperature,
		"turn_number":    t.TurnNumber,
		"created_at":     t.CreatedAt.Format("2006-01-02T15:04:05.999999"),
		"attachment_ids": t.AttachmentIDs,
	}
}

func chatBotAttachmentResponse(a *model.ChatBotAttachment) map[string]any {
	return map[string]any{
		"id":               a.ID,
		"conversation_id":  a.ConversationID,
		"turn_id":          a.TurnID,
		"filename":         a.Filename,
		"content_type":     a.ContentType,
		"kind":             a.Kind,
		"size":             a.Size,
		"extracted_text":   a.ExtractedText,
		"extraction_error": a.ExtractionError,
		"created_at":       a.CreatedAt.Format("2006-01-02T15:04:05.999999"),
	}
}
