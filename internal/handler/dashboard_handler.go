package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/daveontour/digitalmuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// DashboardHandler handles GET /api/dashboard and GET /api/subject-configuration.
type DashboardHandler struct {
	dashSvc    *service.DashboardService
	subjectSvc *service.SubjectConfigService
}

// NewDashboardHandler creates a DashboardHandler.
func NewDashboardHandler(dashSvc *service.DashboardService, subjectSvc *service.SubjectConfigService) *DashboardHandler {
	return &DashboardHandler{dashSvc: dashSvc, subjectSvc: subjectSvc}
}

// RegisterRoutes mounts the dashboard and subject-configuration routes.
func (h *DashboardHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/dashboard", h.GetDashboard)
	r.Get("/api/subject-configuration", h.GetSubjectConfiguration)
	r.Post("/api/subject-configuration", h.UpsertSubjectConfiguration)
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

// UpsertSubjectConfiguration handles POST /api/subject-configuration.
func (h *DashboardHandler) UpsertSubjectConfiguration(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SubjectName            string  `json:"subject_name"`
		SystemInstructions     string  `json:"system_instructions"`
		CoreSystemInstructions *string `json:"core_system_instructions"`
		Gender                 *string `json:"gender"`
		FamilyName             *string `json:"family_name"`
		OtherNames             *string `json:"other_names"`
		EmailAddresses         *string `json:"email_addresses"`
		PhoneNumbers           *string `json:"phone_numbers"`
		WhatsAppHandle         *string `json:"whatsapp_handle"`
		InstagramHandle        *string `json:"instagram_handle"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.SubjectName == "" {
		writeError(w, http.StatusBadRequest, "subject_name is required")
		return
	}
	resp, err := h.subjectSvc.CreateOrUpdate(r.Context(), service.SubjectConfigUpdateParams{
		SubjectName:            body.SubjectName,
		SystemInstructions:     body.SystemInstructions,
		CoreSystemInstructions: body.CoreSystemInstructions,
		Gender:                 body.Gender,
		FamilyName:             body.FamilyName,
		OtherNames:             body.OtherNames,
		EmailAddresses:         body.EmailAddresses,
		PhoneNumbers:           body.PhoneNumbers,
		WhatsAppHandle:         body.WhatsAppHandle,
		InstagramHandle:        body.InstagramHandle,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error saving subject configuration: %s", err))
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
