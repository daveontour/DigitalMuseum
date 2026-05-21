package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// HaveAChatHandler handles the autonomous two-voice conversation endpoints and saved sessions.
type HaveAChatHandler struct {
	svc          *service.ChatService
	sessionStore *keystore.SessionMasterStore
	sessionRepo  *repository.HaveAChatSessionRepo
}

// NewHaveAChatHandler creates a HaveAChatHandler.
func NewHaveAChatHandler(svc *service.ChatService, sessionStore *keystore.SessionMasterStore, sessionRepo *repository.HaveAChatSessionRepo) *HaveAChatHandler {
	return &HaveAChatHandler{svc: svc, sessionStore: sessionStore, sessionRepo: sessionRepo}
}

// RegisterRoutes mounts have-a-chat routes on r.
func (h *HaveAChatHandler) RegisterRoutes(r chi.Router) {
	r.Post("/chat/have-a-chat/turn", h.Turn)
	r.Post("/api/have-a-chat/sessions", h.SaveSession)
	r.Get("/api/have-a-chat/sessions", h.ListSessions)
	r.Get("/api/have-a-chat/sessions/{id}", h.GetSession)
	r.Get("/api/have-a-chat/sessions/{id}/markdown", h.ExportMarkdown)
}

// POST /chat/have-a-chat/turn
func (h *HaveAChatHandler) Turn(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	var req model.HaveAChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.SpeakingSlot != "a" && req.SpeakingSlot != "b" {
		writeError(w, http.StatusBadRequest, "speaking_slot must be 'a' or 'b'")
		return
	}

	resp, err := h.svc.GenerateHaveAChatTurn(r.Context(), r, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, resp)
}

// POST /api/have-a-chat/sessions
func (h *HaveAChatHandler) SaveSession(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	var req model.HaveAChatSessionSave
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if len(req.History) == 0 {
		writeError(w, http.StatusBadRequest, "history must contain at least one turn")
		return
	}
	if req.TurnCount <= 0 {
		req.TurnCount = len(req.History)
	}
	id, err := h.sessionRepo.Create(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, model.HaveAChatSessionSaveResponse{ID: id})
}

// GET /api/have-a-chat/sessions
func (h *HaveAChatHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	limit := 50
	if ls := r.URL.Query().Get("limit"); ls != "" {
		if n, err := strconv.Atoi(ls); err == nil && n > 0 {
			limit = n
		}
	}
	items, err := h.sessionRepo.ListRecent(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"sessions": items})
}

// GET /api/have-a-chat/sessions/{id}
func (h *HaveAChatHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "invalid session id")
		return
	}
	detail, err := h.sessionRepo.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if detail == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	writeJSON(w, detail)
}

// GET /api/have-a-chat/sessions/{id}/markdown
func (h *HaveAChatHandler) ExportMarkdown(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "invalid session id")
		return
	}
	detail, err := h.sessionRepo.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if detail == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	md := buildHaveAChatSessionMarkdown(detail)
	fn := fmt.Sprintf("have-a-chat-%d.md", id)
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+fn+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(md))
}

func buildHaveAChatSessionMarkdown(d *model.HaveAChatSessionDetail) string {
	var b strings.Builder
	b.WriteString("# Have a Chat — saved transcript\n\n")
	b.WriteString(fmt.Sprintf("- **Session id:** %d\n", d.ID))
	if d.StoppedAt != "" {
		b.WriteString(fmt.Sprintf("- **Stopped at:** %s\n", d.StoppedAt))
	}
	if d.CreatedAt != "" {
		b.WriteString(fmt.Sprintf("- **Created at:** %s\n", d.CreatedAt))
	}
	if strings.TrimSpace(d.Topic) != "" {
		b.WriteString(fmt.Sprintf("- **Topic:** %s\n", strings.TrimSpace(d.Topic)))
	}
	b.WriteString(fmt.Sprintf("- **Voice A:** %s (%s)\n", d.VoiceA, d.ProviderA))
	b.WriteString(fmt.Sprintf("- **Voice B:** %s (%s)\n", d.VoiceB, d.ProviderB))
	b.WriteString(fmt.Sprintf("- **Banter mode:** %v\n", d.BanterMode))
	b.WriteString(fmt.Sprintf("- **Temperature:** %g\n", d.Temperature))
	b.WriteString(fmt.Sprintf("- **Allow explicit:** %v\n", d.AllowExplicit))
	b.WriteString(fmt.Sprintf("- **Turns recorded:** %d\n\n", len(d.History)))

	b.WriteString("## Transcript\n\n")
	for i, t := range d.History {
		label := speakerLabelHaveAChat(t.Speaker)
		b.WriteString(fmt.Sprintf("### %d — %s\n\n", i+1, label))
		b.WriteString(wrapMarkdownParagraph(strings.TrimSpace(t.Text)))
		b.WriteString("\n\n")
	}
	return b.String()
}

func speakerLabelHaveAChat(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "a":
		return "Voice A"
	case "b":
		return "Voice B"
	case "user":
		return "You (injected)"
	default:
		if s == "" {
			return "Unknown speaker"
		}
		return "Speaker " + s
	}
}

func wrapMarkdownParagraph(text string) string {
	if text == "" {
		return "_[empty]_"
	}
	if !strings.Contains(text, "\n") {
		return text
	}
	lines := strings.Split(text, "\n")
	var b strings.Builder
	for _, line := range lines {
		b.WriteString("> ")
		b.WriteString(strings.TrimSuffix(line, "\r"))
		b.WriteByte('\n')
	}
	return strings.TrimSuffix(b.String(), "\n")
}
