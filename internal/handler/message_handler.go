package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// MessageHandler handles all /imessages/* read endpoints.
type MessageHandler struct {
	svc          *service.MessageService
	sessionStore *keystore.SessionMasterStore
}

// NewMessageHandler creates a MessageHandler.
func NewMessageHandler(svc *service.MessageService, sessionStore *keystore.SessionMasterStore) *MessageHandler {
	return &MessageHandler{svc: svc, sessionStore: sessionStore}
}

// RegisterRoutes mounts all message routes onto r.
func (h *MessageHandler) RegisterRoutes(r chi.Router) {
	// Specific sub-paths before parameterised {message_id}
	r.Get("/imessages/chat-sessions", h.GetChatSessions)
	r.Get("/imessages/conversation/{chat_session}", h.GetConversation)
	r.Delete("/imessages/conversation/{chat_session}", h.DeleteConversation)
	r.Post("/imessages/conversation/{chat_session}/summarize", h.SummarizeConversation)
	r.Post("/imessages/conversation/{chat_session}/prompt", h.ConversationPrompt)

	r.Get("/imessages/{message_id}/metadata", h.GetMetadata)
	r.Get("/imessages/{message_id}/attachment", h.GetAttachment)
}

// GetChatSessions handles GET /imessages/chat-sessions
func (h *MessageHandler) GetChatSessions(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorMessagesChat(w, r) {
		return
	}
	result, err := h.svc.GetChatSessions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving chat sessions: %s", err))
		return
	}
	writeJSON(w, result)
}

// GetConversation handles GET /imessages/conversation/{chat_session}
// The chat_session path segment may be URL-encoded; chi decodes it automatically.
// We additionally handle double-encoding by calling url.PathUnescape.
func (h *MessageHandler) GetConversation(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorMessagesChat(w, r) {
		return
	}
	raw := chi.URLParam(r, "chat_session")
	// chi already percent-decodes once; apply a second pass to handle clients
	// that double-encode (matches Python's explicit urllib.parse.unquote call).
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		decoded = raw
	}

	msgs, err := h.svc.GetConversationMessages(r.Context(), decoded)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving conversation: %s", err))
		return
	}
	writeJSON(w, map[string]any{"messages": msgs})
}

// GetMetadata handles GET /imessages/{message_id}/metadata
func (h *MessageHandler) GetMetadata(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorMessagesChat(w, r) {
		return
	}
	id, ok := parseMessageID(w, r)
	if !ok {
		return
	}

	resp, err := h.svc.GetMessageMetadata(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving message metadata: %s", err))
		return
	}
	if resp == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("message with ID %d not found", id))
		return
	}
	writeJSON(w, resp)
}

// GetAttachment handles GET /imessages/{message_id}/attachment?preview=bool
func (h *MessageHandler) GetAttachment(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorMessagesChat(w, r) {
		return
	}
	id, ok := parseMessageID(w, r)
	if !ok {
		return
	}

	preview := false
	if v := r.URL.Query().Get("preview"); v != "" {
		var err error
		preview, err = strconv.ParseBool(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "preview must be true or false")
			return
		}
	}

	// Check message exists first
	meta, err := h.svc.GetMessageMetadata(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving message: %s", err))
		return
	}
	if meta == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("message with ID %d not found", id))
		return
	}

	content, err := h.svc.GetAttachmentContent(r.Context(), id, preview)
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "no thumbnail"):
			writeError(w, http.StatusNotFound, "message attachment has no thumbnail available")
		case strings.Contains(msg, "no content"):
			writeError(w, http.StatusNotFound, "message attachment has no content")
		case strings.Contains(msg, "no blob"):
			writeError(w, http.StatusNotFound, "media blob for attachment not found")
		default:
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving attachment: %s", err))
		}
		return
	}
	if content == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("message with ID %d has no attachment", id))
		return
	}

	safe := strings.ReplaceAll(content.Filename, `"`, `\"`)
	w.Header().Set("Content-Type", content.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, safe))
	_, _ = w.Write(content.Data)
}

// DeleteConversation handles DELETE /imessages/conversation/{chat_session}
func (h *MessageHandler) DeleteConversation(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	raw := chi.URLParam(r, "chat_session")
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		decoded = raw
	}

	count, err := h.svc.DeleteConversation(r.Context(), decoded)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error deleting conversation: %s", err))
		return
	}
	writeJSON(w, map[string]any{
		"message":       fmt.Sprintf("Deleted %d messages", count),
		"deleted_count": count,
	})
}

// SummarizeConversation handles POST /imessages/conversation/{chat_session}/summarize
func (h *MessageHandler) SummarizeConversation(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorMessagesChat(w, r) {
		return
	}
	raw := chi.URLParam(r, "chat_session")
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		decoded = raw
	}

	summary, err := h.svc.SummarizeConversation(r.Context(), decoded)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error summarizing conversation: %s", err))
		return
	}
	writeJSON(w, map[string]any{"summary": summary})
}

const maxConversationPromptInstructionBytes = 32 * 1024

// ConversationPrompt handles POST /imessages/conversation/{chat_session}/prompt
// JSON body: { "instruction": "..." } — the instruction is sent to the LLM together with the conversation transcript (same source as /summarize).
func (h *MessageHandler) ConversationPrompt(w http.ResponseWriter, r *http.Request) {
	if !requireVisitorMessagesChat(w, r) {
		return
	}
	raw := chi.URLParam(r, "chat_session")
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		decoded = raw
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxConversationPromptInstructionBytes+64*1024))
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
	if len(instruction) > maxConversationPromptInstructionBytes {
		writeError(w, http.StatusBadRequest, "instruction exceeds maximum length")
		return
	}

	response, err := h.svc.RunConversationPrompt(r.Context(), decoded, instruction)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error running conversation prompt: %s", err))
		return
	}
	writeJSON(w, map[string]any{"response": response})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func parseMessageID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "message_id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "message_id must be an integer")
		return 0, false
	}
	return id, true
}
