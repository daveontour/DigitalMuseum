package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	appai "github.com/daveontour/digitalmuseum/internal/ai"
	"github.com/daveontour/digitalmuseum/internal/keystore"
	"github.com/daveontour/digitalmuseum/internal/model"
	"github.com/daveontour/digitalmuseum/internal/repository"
	"github.com/daveontour/digitalmuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// ChatHandler handles all /chat/* endpoints.
type ChatHandler struct {
	svc          *service.ChatService
	cpRepo       *repository.CompleteProfileRepo
	sessionStore *keystore.SessionMasterStore
}

// NewChatHandler creates a ChatHandler.
func NewChatHandler(svc *service.ChatService, cpRepo *repository.CompleteProfileRepo, sessionStore *keystore.SessionMasterStore) *ChatHandler {
	return &ChatHandler{svc: svc, cpRepo: cpRepo, sessionStore: sessionStore}
}

// RegisterRoutes mounts the chat endpoints on r.
func (h *ChatHandler) RegisterRoutes(r chi.Router) {
	r.Get("/chat/availability", h.GetAvailability)
	r.Post("/chat/generate", h.Generate)
	r.Post("/chat/generate-random-question", h.GenerateRandomQuestion)
	r.Post("/chat/conversations", h.CreateConversation)
	r.Get("/chat/conversations", h.ListConversations)
	r.Get("/chat/conversations/{id}", h.GetConversation)
	r.Put("/chat/conversations/{id}", h.UpdateConversation)
	r.Delete("/chat/conversations/{id}", h.DeleteConversation)
	r.Get("/chat/conversations/{id}/turns", h.GetTurns)

	// Complete profile (must be before /chat/conversations/{id} to avoid "complete-profile" as id)
	r.Get("/chat/complete-profile/names", h.CompleteProfileListNames)
	r.Get("/chat/complete-profile", h.CompleteProfileGet)
	r.Put("/chat/complete-profile", h.CompleteProfileUpdate)
	r.Delete("/chat/complete-profile", h.CompleteProfileDelete)
	r.Post("/chat/complete-profile", h.CompleteProfileStart)
}

// GET /chat/availability
func (h *ChatHandler) GetAvailability(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]bool{
		"gemini_available": h.svc.GeminiAvailable(),
		"claude_available": h.svc.ClaudeAvailable(),
	})
}

// POST /chat/generate
func (h *ChatHandler) Generate(w http.ResponseWriter, r *http.Request) {
	var req model.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}
	if req.Provider == "" {
		req.Provider = "gemini"
	}

	resp, err := h.svc.GenerateResponse(r.Context(), r, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, resp)
}

// POST /chat/generate
func (h *ChatHandler) GenerateRandomQuestion(w http.ResponseWriter, r *http.Request) {
	var req model.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}
	if req.Provider == "" {
		req.Provider = "gemini"
	}

	resp, err := h.svc.GenerateRandomQuestion(r.Context(), r, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, resp)
}

// POST /chat/conversations
func (h *ChatHandler) CreateConversation(w http.ResponseWriter, r *http.Request) {
	var req model.ConversationCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if req.Voice == "" {
		req.Voice = "expert"
	}

	conv, err := h.svc.CreateConversation(r.Context(), req.Title, req.Voice)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, conversationResponse(conv, 0))
}

// GET /chat/conversations
func (h *ChatHandler) ListConversations(w http.ResponseWriter, r *http.Request) {
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

	result := make([]map[string]any, 0, len(convs))
	for _, c := range convs {
		count, _ := h.svc.TurnCount(r.Context(), c.ID)
		m := conversationResponse(c, count)
		result = append(result, m)
	}
	writeJSON(w, result)
}

// GET /chat/conversations/{id}
func (h *ChatHandler) GetConversation(w http.ResponseWriter, r *http.Request) {
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

	turns, err := h.svc.GetTurns(r.Context(), id, 30)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	turnsData := make([]map[string]any, 0, len(turns))
	for _, t := range turns {
		turnsData = append(turnsData, turnResponse(t))
	}

	result := conversationResponse(conv, int64(len(turns)))
	result["turns"] = turnsData
	writeJSON(w, result)
}

// PUT /chat/conversations/{id}
func (h *ChatHandler) UpdateConversation(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req model.ConversationUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	conv, err := h.svc.UpdateConversation(r.Context(), id, req.Title, req.Voice)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if conv == nil {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}
	writeJSON(w, conversationResponse(conv, 0))
}

// DELETE /chat/conversations/{id}
func (h *ChatHandler) DeleteConversation(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	// Verify it exists first
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
	writeJSON(w, map[string]any{
		"success": true,
		"message": "Conversation deleted successfully",
	})
}

// GET /chat/conversations/{id}/turns
func (h *ChatHandler) GetTurns(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	limit := 30
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 100 {
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
		result = append(result, turnResponse(t))
	}
	writeJSON(w, result)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func conversationResponse(c *model.ChatConversation, turnCount int64) map[string]any {
	m := map[string]any{
		"id":              c.ID,
		"title":           c.Title,
		"voice":           c.Voice,
		"created_at":      c.CreatedAt.Format("2006-01-02T15:04:05.999999"),
		"updated_at":      c.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
		"last_message_at": nil,
		"turn_count":      turnCount,
	}
	if c.LastMessageAt != nil {
		m["last_message_at"] = c.LastMessageAt.Format("2006-01-02T15:04:05.999999")
	}
	return m
}

func turnResponse(t *model.ChatTurn) map[string]any {
	return map[string]any{
		"id":            t.ID,
		"user_input":    t.UserInput,
		"response_text": t.ResponseText,
		"voice":         t.Voice,
		"temperature":   t.Temperature,
		"turn_number":   t.TurnNumber,
		"created_at":    t.CreatedAt.Format("2006-01-02T15:04:05.999999"),
	}
}

func parseIDParam(r *http.Request, param string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, param), 10, 64)
}

// ── Complete Profile ───────────────────────────────────────────────────────────

// CompleteProfileListNames handles GET /chat/complete-profile/names.
func (h *ChatHandler) CompleteProfileListNames(w http.ResponseWriter, r *http.Request) {
	names, err := h.cpRepo.ListNames(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error listing complete profile names: "+err.Error())
		return
	}
	if names == nil {
		names = []string{}
	}
	writeJSON(w, map[string]any{"names": names})
}

// CompleteProfileGet handles GET /chat/complete-profile?name=...
func (h *ChatHandler) CompleteProfileGet(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	profile, err := h.cpRepo.GetByName(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error retrieving complete profile: "+err.Error())
		return
	}
	if profile == nil {
		writeError(w, http.StatusNotFound, "No complete profile found for '"+name+"'")
		return
	}
	p := *profile
	writeJSON(w, map[string]any{"name": name, "profile": p})
}

// CompleteProfileUpdate handles PUT /chat/complete-profile.
func (h *ChatHandler) CompleteProfileUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Profile string `json:"profile"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := h.cpRepo.Upsert(r.Context(), name, req.Profile); err != nil {
		writeError(w, http.StatusInternalServerError, "error updating complete profile: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"name": name, "message": "Profile updated"})
}

// CompleteProfileDelete handles DELETE /chat/complete-profile?name=...
func (h *ChatHandler) CompleteProfileDelete(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	deleted, err := h.cpRepo.DeleteByName(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error deleting complete profile: "+err.Error())
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "No complete profile found for '"+name+"'")
		return
	}
	writeJSON(w, map[string]any{"message": "Profile for '" + name + "' deleted"})
}

// CompleteProfileStart handles POST /chat/complete-profile.
// Starts background profile generation using messages and emails for the contact.
// Provider (gemini or claude) is optional; defaults to gemini.
func (h *ChatHandler) CompleteProfileStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FullName string `json:"full_name"`
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	name := strings.TrimSpace(req.FullName)
	if name == "" {
		writeError(w, http.StatusBadRequest, "full_name is required")
		return
	}
	provider := strings.TrimSpace(strings.ToLower(req.Provider))
	tier := appai.UnlockTierFromSession(h.sessionStore, r)
	var getRAM appai.RAMMasterGetter
	if h.sessionStore != nil {
		if p, ok := h.sessionStore.Get(r); ok {
			pw := p
			getRAM = func() (string, bool) { return pw, true }
		}
	}
	go func() {
		ctx := context.Background()
		if err := h.svc.GenerateCompleteProfile(ctx, name, provider, getRAM, tier); err != nil {
			log.Printf("[complete_profile] Error for '%s': %v", name, err)
		}
	}()
	writeJSON(w, map[string]any{
		"status":  "submitted",
		"message": "Complete profile generation started for '" + name + "'. Processing runs in the background.",
	})
}
