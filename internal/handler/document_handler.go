package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/daveontour/digitalmuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// DocumentHandler handles all /reference-documents/* endpoints.
type DocumentHandler struct {
	svc         *service.DocumentService
	sensitiveSvc *service.SensitiveService
}

// NewDocumentHandler creates a DocumentHandler.
func NewDocumentHandler(svc *service.DocumentService, sensitiveSvc *service.SensitiveService) *DocumentHandler {
	return &DocumentHandler{svc: svc, sensitiveSvc: sensitiveSvc}
}

// RegisterRoutes mounts all reference document routes.
func (h *DocumentHandler) RegisterRoutes(r chi.Router) {
	// Keyring management routes — must be before /{doc_id} to avoid chi matching them as IDs.
	r.Post("/reference-documents/init-keyring", h.InitKeyring)
	r.Post("/reference-documents/add-user", h.AddUser)
	r.Delete("/reference-documents/remove-user", h.RemoveUser)
	r.Get("/reference-documents/keyring-count", h.KeyringCount)
	r.Post("/reference-documents/encrypt-existing", h.EncryptExisting)

	r.Get("/reference-documents", h.List)
	r.Post("/reference-documents", h.Create)
	r.Get("/reference-documents/{doc_id}", h.GetByID)
	r.Put("/reference-documents/{doc_id}", h.Update)
	r.Delete("/reference-documents/{doc_id}", h.Delete)
	r.Get("/reference-documents/{doc_id}/download", h.Download)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func parseDocID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "doc_id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "doc_id must be an integer")
		return 0, false
	}
	return id, true
}

type docJSON struct {
	ID               int64   `json:"id"`
	Filename         string  `json:"filename"`
	Title            *string `json:"title"`
	Description      *string `json:"description"`
	Author           *string `json:"author"`
	ContentType      string  `json:"content_type"`
	Size             int64   `json:"size"`
	Tags             *string `json:"tags"`
	Categories       *string `json:"categories"`
	Notes            *string `json:"notes"`
	AvailableForTask bool    `json:"available_for_task"`
	IsPrivate        bool    `json:"is_private"`
	IsSensitive      bool    `json:"is_sensitive"`
	IsEncrypted      bool    `json:"is_encrypted"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

// ── List ──────────────────────────────────────────────────────────────────────

func (h *DocumentHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	search := q.Get("search")
	category := q.Get("category")
	tag := q.Get("tag")
	contentType := q.Get("content_type")
	var availableForTask *bool
	if v := q.Get("available_for_task"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "available_for_task must be true or false")
			return
		}
		availableForTask = &b
	}

	docs, err := h.svc.List(r.Context(), search, category, tag, contentType, availableForTask)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error listing documents: %s", err))
		return
	}

	out := make([]docJSON, 0, len(docs))
	for _, d := range docs {
		out = append(out, docJSON{
			ID: d.ID, Filename: d.Filename, Title: d.Title, Description: d.Description,
			Author: d.Author, ContentType: d.ContentType, Size: d.Size, Tags: d.Tags,
			Categories: d.Categories, Notes: d.Notes, AvailableForTask: d.AvailableForTask,
			IsPrivate: d.IsPrivate, IsSensitive: d.IsSensitive, IsEncrypted: d.IsEncrypted,
			CreatedAt: d.CreatedAt.Format("2006-01-02T15:04:05.999999"),
			UpdatedAt: d.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
		})
	}
	writeJSON(w, out)
}

// ── GetByID ───────────────────────────────────────────────────────────────────

func (h *DocumentHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, ok := parseDocID(w, r)
	if !ok {
		return
	}
	d, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving document: %s", err))
		return
	}
	if d == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("reference document with ID %d not found", id))
		return
	}
	writeJSON(w, docJSON{
		ID: d.ID, Filename: d.Filename, Title: d.Title, Description: d.Description,
		Author: d.Author, ContentType: d.ContentType, Size: d.Size, Tags: d.Tags,
		Categories: d.Categories, Notes: d.Notes, AvailableForTask: d.AvailableForTask,
		IsPrivate: d.IsPrivate, IsSensitive: d.IsSensitive, IsEncrypted: d.IsEncrypted,
		CreatedAt: d.CreatedAt.Format("2006-01-02T15:04:05.999999"),
		UpdatedAt: d.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
	})
}

// ── Download ──────────────────────────────────────────────────────────────────

func (h *DocumentHandler) Download(w http.ResponseWriter, r *http.Request) {
	id, ok := parseDocID(w, r)
	if !ok {
		return
	}
	d, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving document: %s", err))
		return
	}
	if d == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("reference document with ID %d not found", id))
		return
	}
	password := r.URL.Query().Get("password")
	data, err := h.svc.GetData(r.Context(), id, password)
	if err != nil {
		if errors.Is(err, service.ErrPasswordRequired) {
			writeError(w, http.StatusForbidden, "password required to access encrypted document")
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving document data: %s", err))
		return
	}
	if len(data) == 0 {
		writeError(w, http.StatusNotFound, fmt.Sprintf("reference document with ID %d has no file data", id))
		return
	}
	filename := d.Filename
	if filename == "" {
		filename = "document"
	}
	ct := d.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	safe := strings.ReplaceAll(filename, `"`, `\"`)
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, safe))
	_, _ = w.Write(data)
}

// ── Create ────────────────────────────────────────────────────────────────────

func (h *DocumentHandler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "could not parse multipart form")
		return
	}
	f, fh, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read uploaded file")
		return
	}
	if len(data) == 0 {
		writeError(w, http.StatusBadRequest, "uploaded file is empty")
		return
	}

	ct := fh.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}

	availableForTask := false
	if v := r.FormValue("available_for_task"); v != "" {
		availableForTask, _ = strconv.ParseBool(v)
	}
	isPrivate := false
	if v := r.FormValue("is_private"); v != "" {
		isPrivate, _ = strconv.ParseBool(v)
	}
	isSensitive := false
	if v := r.FormValue("is_sensitive"); v != "" {
		isSensitive, _ = strconv.ParseBool(v)
	}
	masterPassword := r.FormValue("master_password")

	optForm := func(key string) *string {
		if v := r.FormValue(key); v != "" {
			return &v
		}
		return nil
	}

	d, err := h.svc.Create(r.Context(),
		fh.Filename, ct, int64(len(data)), data,
		optForm("title"), optForm("description"), optForm("author"),
		optForm("tags"), optForm("categories"), optForm("notes"),
		availableForTask, isPrivate, isSensitive, masterPassword,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error creating document: %s", err))
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, docJSON{
		ID: d.ID, Filename: d.Filename, Title: d.Title, Description: d.Description,
		Author: d.Author, ContentType: d.ContentType, Size: d.Size, Tags: d.Tags,
		Categories: d.Categories, Notes: d.Notes, AvailableForTask: d.AvailableForTask,
		IsPrivate: d.IsPrivate, IsSensitive: d.IsSensitive, IsEncrypted: d.IsEncrypted,
		CreatedAt: d.CreatedAt.Format("2006-01-02T15:04:05.999999"),
		UpdatedAt: d.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
	})
}

// ── Update ────────────────────────────────────────────────────────────────────

func (h *DocumentHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseDocID(w, r)
	if !ok {
		return
	}
	var req struct {
		Title            *string `json:"title"`
		Description      *string `json:"description"`
		Author           *string `json:"author"`
		Tags             *string `json:"tags"`
		Categories       *string `json:"categories"`
		Notes            *string `json:"notes"`
		AvailableForTask *bool   `json:"available_for_task"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	d, err := h.svc.Update(r.Context(), id,
		req.Title, req.Description, req.Author, req.Tags, req.Categories, req.Notes, req.AvailableForTask)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error updating document: %s", err))
		return
	}
	if d == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("reference document with ID %d not found", id))
		return
	}
	writeJSON(w, docJSON{
		ID: d.ID, Filename: d.Filename, Title: d.Title, Description: d.Description,
		Author: d.Author, ContentType: d.ContentType, Size: d.Size, Tags: d.Tags,
		Categories: d.Categories, Notes: d.Notes, AvailableForTask: d.AvailableForTask,
		IsPrivate: d.IsPrivate, IsSensitive: d.IsSensitive, IsEncrypted: d.IsEncrypted,
		CreatedAt: d.CreatedAt.Format("2006-01-02T15:04:05.999999"),
		UpdatedAt: d.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
	})
}

// ── Delete ────────────────────────────────────────────────────────────────────

func (h *DocumentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseDocID(w, r)
	if !ok {
		return
	}
	d, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving document: %s", err))
		return
	}
	if d == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("reference document with ID %d not found", id))
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error deleting document: %s", err))
		return
	}
	writeJSON(w, map[string]string{"message": fmt.Sprintf("Reference document %d deleted successfully", id)})
}

// ── Keyring management ────────────────────────────────────────────────────────

func (h *DocumentHandler) InitKeyring(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Password == "" {
		writeError(w, http.StatusBadRequest, "password is required")
		return
	}
	if err := h.sensitiveSvc.GenerateKeyring(r.Context(), req.Password); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error initialising keyring: %s", err))
		return
	}
	writeJSON(w, map[string]string{"message": "keyring initialised"})
}

func (h *DocumentHandler) AddUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserPassword   string `json:"user_password"`
		MasterPassword string `json:"master_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := h.sensitiveSvc.AddUser(r.Context(), req.UserPassword, req.MasterPassword); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error adding user: %s", err))
		return
	}
	writeJSON(w, map[string]string{"message": "user added to keyring"})
}

func (h *DocumentHandler) RemoveUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserPassword   string `json:"user_password"`
		MasterPassword string `json:"master_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := h.sensitiveSvc.RemoveUser(r.Context(), req.UserPassword, req.MasterPassword); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error removing user: %s", err))
		return
	}
	writeJSON(w, map[string]string{"message": "user removed from keyring"})
}

func (h *DocumentHandler) KeyringCount(w http.ResponseWriter, r *http.Request) {
	count, err := h.sensitiveSvc.KeyCount(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error getting keyring count: %s", err))
		return
	}
	writeJSON(w, map[string]int64{"count": count})
}

func (h *DocumentHandler) EncryptExisting(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Password == "" {
		writeError(w, http.StatusBadRequest, "password is required")
		return
	}
	count, err := h.svc.EncryptExisting(r.Context(), req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error encrypting documents: %s", err))
		return
	}
	writeJSON(w, map[string]int{"encrypted": count})
}
