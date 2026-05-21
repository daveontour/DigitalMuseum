package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// IdentityProfileHandler handles the /api/identity-profile/* endpoints.
type IdentityProfileHandler struct {
	chatSvc          *service.ChatService
	documentSvc      *service.DocumentService
	documentRepo     *repository.DocumentRepo
	subjectConfigSvc *service.SubjectConfigService
	sessionStore     *keystore.SessionMasterStore
}

// NewIdentityProfileHandler creates an IdentityProfileHandler.
func NewIdentityProfileHandler(
	chatSvc *service.ChatService,
	documentSvc *service.DocumentService,
	documentRepo *repository.DocumentRepo,
	subjectConfigSvc *service.SubjectConfigService,
	sessionStore *keystore.SessionMasterStore,
) *IdentityProfileHandler {
	return &IdentityProfileHandler{
		chatSvc:          chatSvc,
		documentSvc:      documentSvc,
		documentRepo:     documentRepo,
		subjectConfigSvc: subjectConfigSvc,
		sessionStore:     sessionStore,
	}
}

// RegisterRoutes mounts identity profile routes.
func (h *IdentityProfileHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/identity-profile/status", h.GetStatus)
	r.Get("/api/identity-profile", h.GetProfile)
	r.Post("/api/identity-profile", h.SaveProfile)
	r.Post("/api/identity-profile/parse", h.ParseDocument)
}

// GetStatus returns {"has_profile": bool} — lightweight first-run check.
func (h *IdentityProfileHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	doc, err := h.documentRepo.FindIdentityProfile(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"has_profile": doc != nil})
}

// GetProfile returns the identity profile markdown content (decrypted).
func (h *IdentityProfileHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	doc, err := h.documentRepo.FindIdentityProfile(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if doc == nil {
		writeJSON(w, map[string]any{"exists": false})
		return
	}

	masterPW := ""
	if h.sessionStore != nil {
		if pw, ok := h.sessionStore.Get(r); ok {
			masterPW = pw
		}
	}

	notes := ""
	if doc.Notes != nil {
		notes = *doc.Notes
	}

	data, err := h.documentSvc.GetData(r.Context(), doc.ID, masterPW)
	if err != nil {
		// Encrypted and no password — return notes (form data) so wizard can re-populate.
		writeJSON(w, map[string]any{"exists": true, "content": "", "doc_id": doc.ID, "notes": notes, "encrypted": true})
		return
	}

	writeJSON(w, map[string]any{
		"exists":  true,
		"content": string(data),
		"doc_id":  doc.ID,
		"notes":   notes,
	})
}

// saveProfileBody is the JSON body for POST /api/identity-profile.
type saveProfileBody struct {
	Markdown       string `json:"markdown"`
	FormDataJSON   string `json:"form_data_json"`
	SubjectName    string `json:"subject_name"`
	Gender         string `json:"gender"`
	EmailAddresses string `json:"email_addresses"`
	PhoneNumbers   string `json:"phone_numbers"`
}

// SaveProfile creates or updates the identity profile reference document.
func (h *IdentityProfileHandler) SaveProfile(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read request body")
		return
	}
	var req saveProfileBody
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.Markdown) == "" {
		writeError(w, http.StatusBadRequest, "markdown is required")
		return
	}

	ctx := r.Context()
	data := []byte(req.Markdown)
	size := int64(len(data))

	masterPW := ""
	if h.sessionStore != nil {
		if pw, ok := h.sessionStore.Get(r); ok {
			masterPW = pw
		}
	}

	tags := strPtr("identity-profile")
	notes := strPtr(req.FormDataJSON)

	existing, err := h.documentRepo.FindIdentityProfile(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var docID int64
	if existing == nil {
		// Create new profile document
		filename := "identity_profile.md"
		contentType := "text/markdown"
		title := strPtr("Identity Profile")
		description := strPtr("Personal identity profile used by the AI assistant.")
		doc, err := h.documentSvc.Create(ctx,
			filename, contentType, size, data,
			title, description, nil, tags, nil, notes,
			false, true, false, true, masterPW,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create profile: "+err.Error())
			return
		}
		docID = doc.ID
	} else {
		docID = existing.ID
		if err := h.documentSvc.UpdateContent(ctx, docID, data, masterPW); err != nil {
			writeError(w, http.StatusInternalServerError, "update profile content: "+err.Error())
			return
		}
		// Update metadata — notes stores formData JSON for re-population
		if _, err := h.documentSvc.Update(ctx, docID, nil, nil, nil, tags, nil, notes, nil, nil); err != nil {
			writeError(w, http.StatusInternalServerError, "update profile metadata: "+err.Error())
			return
		}
	}

	// Sync scalar fields to subject_configuration
	if req.SubjectName != "" || req.Gender != "" || req.EmailAddresses != "" || req.PhoneNumbers != "" {
		params := service.SubjectConfigUpdateParams{
			SubjectName: req.SubjectName,
		}
		if req.Gender != "" {
			params.Gender = strPtr(req.Gender)
		}
		if req.EmailAddresses != "" {
			params.EmailAddresses = strPtr(req.EmailAddresses)
		}
		if req.PhoneNumbers != "" {
			params.PhoneNumbers = strPtr(req.PhoneNumbers)
		}
		if _, err := h.subjectConfigSvc.CreateOrUpdate(ctx, params); err != nil {
			// Non-fatal — log but continue
			_ = err
		}
	}

	writeJSON(w, map[string]any{"success": true, "doc_id": docID})
}

// parseDocumentBody is the JSON body for POST /api/identity-profile/parse.
type parseDocumentBody struct {
	Text string `json:"text"`
}

// ParseDocument uses the AI provider to extract structured identity fields from pasted text.
func (h *IdentityProfileHandler) ParseDocument(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read request body")
		return
	}
	var req parseDocumentBody
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeJSON(w, map[string]any{"fields": map[string]any{}, "warning": "no text provided"})
		return
	}

	fields, err := h.chatSvc.ExtractIdentityProfile(r.Context(), r, req.Text)
	if err != nil {
		writeJSON(w, map[string]any{"fields": map[string]any{}, "warning": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"fields": fields})
}

func strPtr(s string) *string {
	return &s
}
