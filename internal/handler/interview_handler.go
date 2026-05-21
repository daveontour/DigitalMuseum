package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// InterviewHandler handles the interview mode endpoints.
type InterviewHandler struct {
	svc           *service.ChatService
	interviewRepo *repository.InterviewRepo
	sessionStore  *keystore.SessionMasterStore
}

// NewInterviewHandler creates an InterviewHandler.
func NewInterviewHandler(svc *service.ChatService, interviewRepo *repository.InterviewRepo, sessionStore *keystore.SessionMasterStore) *InterviewHandler {
	return &InterviewHandler{svc: svc, interviewRepo: interviewRepo, sessionStore: sessionStore}
}

// RegisterRoutes mounts interview endpoints on r.
func (h *InterviewHandler) RegisterRoutes(r chi.Router) {
	r.Post("/interview/start", h.Start)
	r.Post("/interview/turn", h.Turn)
	r.Post("/interview/pause", h.Pause)
	r.Post("/interview/resume", h.Resume)
	r.Post("/interview/end", h.End)
	r.Get("/interview/list", h.List)
	r.Get("/interview/{id}", h.GetDetail)
	r.Get("/interview/{id}/turns", h.GetTurns)
}

// POST /interview/start
func (h *InterviewHandler) Start(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	var req model.StartInterviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Style == "" || req.Purpose == "" {
		writeError(w, http.StatusBadRequest, "style and purpose are required")
		return
	}
	resp, err := h.svc.StartInterview(r.Context(), r, req, h.interviewRepo)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, resp)
}

// POST /interview/turn
func (h *InterviewHandler) Turn(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	var req model.InterviewTurnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.InterviewID == 0 {
		writeError(w, http.StatusBadRequest, "interview_id is required")
		return
	}
	if req.Answer == "" {
		writeError(w, http.StatusBadRequest, "answer is required")
		return
	}
	resp, err := h.svc.GenerateInterviewTurn(r.Context(), r, req, h.interviewRepo)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, resp)
}

// POST /interview/pause
func (h *InterviewHandler) Pause(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	var req model.InterviewActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if err := h.svc.PauseInterview(r.Context(), req.InterviewID, h.interviewRepo); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "paused"})
}

// POST /interview/resume
func (h *InterviewHandler) Resume(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	var req model.InterviewActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	resp, err := h.svc.ResumeInterview(r.Context(), r, req.InterviewID, h.interviewRepo)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, resp)
}

// POST /interview/end
func (h *InterviewHandler) End(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	var req model.InterviewActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	resp, err := h.svc.EndInterview(r.Context(), r, req.InterviewID, h.interviewRepo)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, resp)
}

// GET /interview/list
func (h *InterviewHandler) List(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	stateFilter := r.URL.Query().Get("state")
	interviews, err := h.svc.ListInterviews(r.Context(), stateFilter, h.interviewRepo)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if interviews == nil {
		interviews = []*model.Interview{}
	}
	writeJSON(w, model.InterviewListResponse{Interviews: interviews})
}

// GET /interview/{id}
func (h *InterviewHandler) GetDetail(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid interview id")
		return
	}
	resp, err := h.svc.GetInterviewDetail(r.Context(), id, h.interviewRepo)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, resp)
}

// GET /interview/{id}/turns
func (h *InterviewHandler) GetTurns(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid interview id")
		return
	}
	turns, err := h.svc.GetInterviewTurns(r.Context(), id, h.interviewRepo)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if turns == nil {
		turns = []*model.InterviewTurn{}
	}
	writeJSON(w, map[string]any{"turns": turns})
}
