package handler

import (
	"encoding/json"
	"net/http"

	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// PamBotHandler serves the Pam Bot dementia companion endpoints.
type PamBotHandler struct {
	svc *service.PamBotService
}

// NewPamBotHandler creates a PamBotHandler.
func NewPamBotHandler(svc *service.PamBotService) *PamBotHandler {
	return &PamBotHandler{svc: svc}
}

// RegisterRoutes mounts the Pam Bot routes.
func (h *PamBotHandler) RegisterRoutes(r chi.Router) {
	r.Post("/api/pambot/message", h.Message)
	r.Get("/api/pambot/session", h.Session)
}

type pamBotMessageRequest struct {
	Action    string `json:"action"`
	TypedText string `json:"typed_text"`
}

// Message handles POST /api/pambot/message.
func (h *PamBotHandler) Message(w http.ResponseWriter, r *http.Request) {
	var req pamBotMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	if req.Action == "" {
		req.Action = "start"
	}

	result, err := h.svc.GeneratePamBotMessage(r.Context(), r, req.Action, req.TypedText)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// Session handles GET /api/pambot/session.
func (h *PamBotHandler) Session(w http.ResponseWriter, r *http.Request) {
	result, err := h.svc.GetSessionInfo(r.Context())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}
