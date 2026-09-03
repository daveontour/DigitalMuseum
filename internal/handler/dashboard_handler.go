package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// DashboardHandler handles GET /api/dashboard and GET /api/subject-configuration.
type DashboardHandler struct {
	dashSvc      *service.DashboardService
	subjectSvc   *service.SubjectConfigService
	sessionStore *keystore.SessionMasterStore
}

// NewDashboardHandler creates a DashboardHandler.
func NewDashboardHandler(dashSvc *service.DashboardService, subjectSvc *service.SubjectConfigService, sessionStore *keystore.SessionMasterStore) *DashboardHandler {
	return &DashboardHandler{dashSvc: dashSvc, subjectSvc: subjectSvc, sessionStore: sessionStore}
}

// RegisterRoutes mounts the dashboard and subject-configuration routes.
func (h *DashboardHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/dashboard", h.GetDashboard)
	r.Get("/api/import-modal-stats", h.GetImportModalStats)
	r.Get("/api/import-modal-embedding-progress", h.GetEmbeddingProgress)
	r.Get("/api/subject-configuration", h.GetSubjectConfiguration)
	r.Post("/api/subject-configuration", h.UpsertSubjectConfiguration)
	r.Put("/api/subject-configuration/writing-style-ai", h.PutWritingStyleAI)
	r.Put("/api/subject-configuration/psychological-profile-ai", h.PutPsychologicalProfileAI)
	r.Put("/api/system-instructions", h.PutAppSystemInstructions)
}

// GetDashboard handles GET /api/dashboard.
func (h *DashboardHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	resp, err := h.dashSvc.GetDashboard(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving dashboard: %s", err))
		return
	}
	writeJSON(w, resp)
}

// GetImportModalStats handles GET /api/import-modal-stats. Returns cheap entry counts
// only; searchable/embedding progress is fetched separately via GetEmbeddingProgress.
func (h *DashboardHandler) GetImportModalStats(w http.ResponseWriter, r *http.Request) {
	resp, err := h.dashSvc.GetImportModalStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving import modal stats: %s", err))
		return
	}
	writeJSON(w, resp)
}

// GetEmbeddingProgress handles GET /api/import-modal-embedding-progress?source=<key>.
// Each of the five embedding-progress sources (see repository.EmbeddingProgressSourceKeys)
// is fetched independently by the caller — one request per source — rather than in one
// batched call, so a single slow source's query doesn't hold up the others and its
// result can be rendered into the modal as soon as it's ready.
func (h *DashboardHandler) GetEmbeddingProgress(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(r.URL.Query().Get("source"))
	if key == "" {
		writeError(w, http.StatusBadRequest, "missing required query param: source")
		return
	}
	entry, err := h.dashSvc.GetEmbeddingProgressForSource(r.Context(), key)
	if err != nil {
		if errors.Is(err, repository.ErrUnknownEmbeddingSource) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown embedding progress source: %s", key))
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving embedding progress: %s", err))
		return
	}
	writeJSON(w, model.EmbeddingProgressSourceResponse{Key: key, Total: entry.Total, Pending: entry.Pending})
}

// UpsertSubjectConfiguration handles POST /api/subject-configuration.
func (h *DashboardHandler) UpsertSubjectConfiguration(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	var body struct {
		SubjectName      string          `json:"subject_name"`
		Gender           *string         `json:"gender"`
		FamilyName       *string         `json:"family_name"`
		OtherNames       *string         `json:"other_names"`
		EmailAddresses   *string         `json:"email_addresses"`
		PhoneNumbers     *string         `json:"phone_numbers"`
		WhatsAppHandle   *string         `json:"whatsapp_handle"`
		InstagramHandle  *string         `json:"instagram_handle"`
		SubjectContactID json.RawMessage `json:"subject_contact_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.SubjectName == "" {
		writeError(w, http.StatusBadRequest, "subject_name is required")
		return
	}

	contactIDSet := len(body.SubjectContactID) > 0
	var cid sql.NullInt64
	if contactIDSet {
		raw := strings.TrimSpace(string(body.SubjectContactID))
		if raw == "" || raw == "null" {
			cid = sql.NullInt64{Valid: false}
		} else {
			var v int64
			if err := json.Unmarshal(body.SubjectContactID, &v); err != nil {
				writeError(w, http.StatusBadRequest, "subject_contact_id must be a number or null")
				return
			}
			if v < 0 {
				cid = sql.NullInt64{Valid: false}
			} else {
				cid = sql.NullInt64{Valid: true, Int64: v}
			}
		}
	}

	resp, err := h.subjectSvc.CreateOrUpdate(r.Context(), service.SubjectConfigUpdateParams{
		SubjectName:         body.SubjectName,
		Gender:              body.Gender,
		FamilyName:          body.FamilyName,
		OtherNames:          body.OtherNames,
		EmailAddresses:      body.EmailAddresses,
		PhoneNumbers:        body.PhoneNumbers,
		WhatsAppHandle:      body.WhatsAppHandle,
		InstagramHandle:     body.InstagramHandle,
		SubjectContactIDSet: contactIDSet,
		SubjectContactID:    cid,
	})
	if err != nil {
		if errors.Is(err, service.ErrSubjectContactNotFound) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error saving subject configuration: %s", err))
		return
	}
	writeJSON(w, resp)
}

// PutWritingStyleAI handles PUT /api/subject-configuration/writing-style-ai.
func (h *DashboardHandler) PutWritingStyleAI(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	var body struct {
		WritingStyleAI string `json:"writing_style_ai"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	resp, err := h.subjectSvc.UpdateWritingStyleAIText(r.Context(), body.WritingStyleAI)
	if err != nil {
		if errors.Is(err, service.ErrNoSubjectConfiguration) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error saving writing style: %s", err))
		return
	}
	writeJSON(w, resp)
}

// PutPsychologicalProfileAI handles PUT /api/subject-configuration/psychological-profile-ai.
func (h *DashboardHandler) PutPsychologicalProfileAI(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	var body struct {
		PsychologicalProfileAI string `json:"psychological_profile_ai"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	resp, err := h.subjectSvc.UpdatePsychologicalProfileAIText(r.Context(), body.PsychologicalProfileAI)
	if err != nil {
		if errors.Is(err, service.ErrNoSubjectConfiguration) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error saving psychological profile: %s", err))
		return
	}
	writeJSON(w, resp)
}

// GetSubjectConfiguration handles GET /api/subject-configuration.
func (h *DashboardHandler) GetSubjectConfiguration(w http.ResponseWriter, r *http.Request) {
	resp, err := h.subjectSvc.GetConfiguration(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving subject configuration: %s", err))
		return
	}
	if resp == nil {
		writeError(w, http.StatusNotFound, "subject configuration not found")
		return
	}
	writeJSON(w, resp)
}

// PutAppSystemInstructions handles PUT /api/system-instructions (universal prompts).
func (h *DashboardHandler) PutAppSystemInstructions(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	var body struct {
		SystemInstructions         string `json:"system_instructions"`
		CoreSystemInstructions     string `json:"core_system_instructions"`
		QuestionSystemInstructions string `json:"question_system_instructions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.subjectSvc.UpdateAppSystemInstructions(r.Context(), service.AppSystemInstructionsUpdate{
		ChatInstructions:     body.SystemInstructions,
		CoreInstructions:     body.CoreSystemInstructions,
		QuestionInstructions: body.QuestionSystemInstructions,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error saving system instructions: %s", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
