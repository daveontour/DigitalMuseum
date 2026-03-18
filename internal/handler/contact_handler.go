package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/daveontour/digitalmuseum/internal/repository"
	"github.com/daveontour/digitalmuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// ContactHandler handles contacts, email-matches, email-exclusions,
// email-classifications, and the relationship graph.
type ContactHandler struct {
	svc *service.ContactService
}

// NewContactHandler creates a ContactHandler.
func NewContactHandler(svc *service.ContactService) *ContactHandler {
	return &ContactHandler{svc: svc}
}

// RegisterRoutes mounts all contact-related routes.
func (h *ContactHandler) RegisterRoutes(r chi.Router) {
	// Graph
	r.Get("/relationship/strength", h.RelationshipStrength)

	// Contacts
	r.Get("/contacts/names", h.ListNames)
	r.Get("/contacts", h.List)
	r.Post("/contacts/bulk-delete", h.BulkDelete)
	r.Delete("/contacts/{contact_id}", h.Delete)
	r.Patch("/contacts/update-classification", h.UpdateClassification)

	// Email matches
	r.Get("/email-matches", h.ListEmailMatches)
	r.Post("/email-matches", h.CreateEmailMatch)
	r.Get("/email-matches/{match_id}", h.GetEmailMatch)
	r.Put("/email-matches/{match_id}", h.UpdateEmailMatch)
	r.Delete("/email-matches/{match_id}", h.DeleteEmailMatch)

	// Email exclusions
	r.Get("/email-exclusions", h.ListEmailExclusions)
	r.Post("/email-exclusions", h.CreateEmailExclusion)
	r.Get("/email-exclusions/{excl_id}", h.GetEmailExclusion)
	r.Put("/email-exclusions/{excl_id}", h.UpdateEmailExclusion)
	r.Delete("/email-exclusions/{excl_id}", h.DeleteEmailExclusion)

	// Email classifications
	r.Get("/email-classifications/options", h.GetEmailClassificationOptions)
	r.Get("/email-classifications", h.ListEmailClassifications)
	r.Post("/email-classifications", h.CreateEmailClassification)
	r.Get("/email-classifications/{cls_id}", h.GetEmailClassification)
	r.Put("/email-classifications/{cls_id}", h.UpdateEmailClassification)
	r.Delete("/email-classifications/{cls_id}", h.DeleteEmailClassification)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func parseContactID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "contact_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "contact_id must be an integer")
		return 0, false
	}
	return id, true
}

func parseMatchID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "match_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "match_id must be an integer")
		return 0, false
	}
	return id, true
}

func parseExclID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "excl_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "excl_id must be an integer")
		return 0, false
	}
	return id, true
}

func parseClsID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "cls_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "cls_id must be an integer")
		return 0, false
	}
	return id, true
}

func parseBoolQuery(r *http.Request, key string) *bool {
	v := r.URL.Query().Get(key)
	if v == "" {
		return nil
	}
	b := v == "true" || v == "1"
	return &b
}

// ── Relationship graph ────────────────────────────────────────────────────────

func (h *ContactHandler) RelationshipStrength(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var types, sources []string
	if t := q.Get("types"); t != "" {
		for _, s := range strings.Split(t, ",") {
			if s = strings.TrimSpace(strings.ToLower(s)); s != "" {
				types = append(types, s)
			}
		}
	}
	if s := q.Get("sources"); s != "" {
		for _, v := range strings.Split(s, ",") {
			if v = strings.TrimSpace(strings.ToLower(v)); v != "" {
				sources = append(sources, v)
			}
		}
	}
	maxNodes := 100
	if mn := q.Get("max_nodes"); mn != "" {
		if n, err := strconv.Atoi(mn); err == nil && n >= 1 && n <= 1000 {
			maxNodes = n
		}
	}

	graph, err := h.svc.GetRelationshipGraph(r.Context(), types, sources, maxNodes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error building relationship graph: %s", err))
		return
	}
	writeJSON(w, graph)
}

// ── Contacts ──────────────────────────────────────────────────────────────────

func (h *ContactHandler) ListNames(w http.ResponseWriter, r *http.Request) {
	names, err := h.svc.ListNames(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error listing contacts: %s", err))
		return
	}
	out := make([]map[string]any, 0, len(names))
	for _, n := range names {
		out = append(out, map[string]any{"id": n.ID, "name": n.Name})
	}
	writeJSON(w, map[string]any{"contacts": out})
}

func (h *ContactHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 0
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			limit = n
		}
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	p := repository.ContactListParams{
		Name:             q.Get("name"),
		Email:            q.Get("email"),
		Search:           q.Get("search"),
		IsSubject:        parseBoolQuery(r, "is_subject"),
		IsGroup:          parseBoolQuery(r, "is_group"),
		HasMessages:      parseBoolQuery(r, "has_messages"),
		EmailContainsAt:  parseBoolQuery(r, "email_contains_at"),
		ExcludePhoneNums: parseBoolQuery(r, "exclude_phone_numbers"),
		Limit:            limit,
		Offset:           offset,
		OrderBy:          q.Get("order_by"),
		Order:            q.Get("order"),
	}

	contacts, total, err := h.svc.ListShort(r.Context(), p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error listing contacts: %s", err))
		return
	}

	out := make([]map[string]any, 0, len(contacts))
	for _, c := range contacts {
		out = append(out, map[string]any{
			"id":           c.ID,
			"name":         c.Name,
			"email":        c.Email,
			"numemail":     c.NumEmails,
			"facebookid":   c.FacebookID,
			"numfacebook":  c.NumFacebook,
			"whatsappid":   c.WhatsAppID,
			"numwhatsapp":  c.NumWhatsApp,
			"imessageid":   c.IMessageID,
			"numimessages": c.NumIMessages,
			"smsid":        c.SMSID,
			"numsms":       c.NumSMS,
			"instagramid":  c.InstagramID,
			"numinstagram": c.NumInstagram,
		})
	}
	writeJSON(w, map[string]any{"contacts": out, "total": total})
}

func (h *ContactHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseContactID(w, r)
	if !ok {
		return
	}
	deleted, err := h.svc.Delete(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error deleting contact: %s", err))
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, fmt.Sprintf("contact not found: id=%d", id))
		return
	}
	writeJSON(w, map[string]any{"message": "Contact deleted", "id": id})
}

func (h *ContactHandler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	deleted, skipped, err := h.svc.BulkDelete(r.Context(), req.IDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error bulk-deleting contacts: %s", err))
		return
	}
	if deleted == nil {
		deleted = []int64{}
	}
	if skipped == nil {
		skipped = []int64{}
	}
	writeJSON(w, map[string]any{"deleted": deleted, "skipped": skipped})
}

func (h *ContactHandler) UpdateClassification(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name           string `json:"name"`
		Classification string `json:"classification"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Classification = strings.TrimSpace(req.Classification)
	if req.Name == "" || req.Classification == "" {
		writeError(w, http.StatusBadRequest, "name and classification are required")
		return
	}
	if err := h.svc.UpdateClassification(r.Context(), req.Name, req.Classification); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error updating classification: %s", err))
		return
	}
	writeJSON(w, map[string]any{"updated": true, "name": req.Name, "classification": req.Classification})
}

// ── Email matches ─────────────────────────────────────────────────────────────

func (h *ContactHandler) ListEmailMatches(w http.ResponseWriter, r *http.Request) {
	matches, err := h.svc.ListEmailMatches(r.Context(), r.URL.Query().Get("primary_name"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error listing email matches: %s", err))
		return
	}
	out := make([]map[string]any, 0, len(matches))
	for _, m := range matches {
		out = append(out, map[string]any{
			"id":           m.ID,
			"primary_name": m.PrimaryName,
			"email":        m.Email,
			"created_at":   m.CreatedAt.Format("2006-01-02T15:04:05.999999"),
			"updated_at":   m.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
		})
	}
	writeJSON(w, out)
}

func (h *ContactHandler) GetEmailMatch(w http.ResponseWriter, r *http.Request) {
	id, ok := parseMatchID(w, r)
	if !ok {
		return
	}
	m, err := h.svc.GetEmailMatchByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving email match: %s", err))
		return
	}
	if m == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("email match not found: id=%d", id))
		return
	}
	writeJSON(w, map[string]any{
		"id":           m.ID,
		"primary_name": m.PrimaryName,
		"email":        m.Email,
		"created_at":   m.CreatedAt.Format("2006-01-02T15:04:05.999999"),
		"updated_at":   m.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
	})
}

func (h *ContactHandler) CreateEmailMatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PrimaryName string `json:"primary_name"`
		Email       string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.PrimaryName = strings.TrimSpace(req.PrimaryName)
	req.Email = strings.TrimSpace(req.Email)
	if req.PrimaryName == "" || req.Email == "" {
		writeError(w, http.StatusBadRequest, "primary_name and email are required")
		return
	}
	m, err := h.svc.CreateEmailMatch(r.Context(), req.PrimaryName, req.Email)
	if err != nil {
		if strings.HasPrefix(err.Error(), "conflict:") {
			writeError(w, http.StatusConflict, strings.TrimPrefix(err.Error(), "conflict:"))
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error creating email match: %s", err))
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{
		"id":           m.ID,
		"primary_name": m.PrimaryName,
		"email":        m.Email,
		"created_at":   m.CreatedAt.Format("2006-01-02T15:04:05.999999"),
		"updated_at":   m.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
	})
}

func (h *ContactHandler) UpdateEmailMatch(w http.ResponseWriter, r *http.Request) {
	id, ok := parseMatchID(w, r)
	if !ok {
		return
	}
	var req struct {
		PrimaryName *string `json:"primary_name"`
		Email       *string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	m, err := h.svc.UpdateEmailMatch(r.Context(), id, req.PrimaryName, req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error updating email match: %s", err))
		return
	}
	if m == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("email match not found: id=%d", id))
		return
	}
	writeJSON(w, map[string]any{
		"id":           m.ID,
		"primary_name": m.PrimaryName,
		"email":        m.Email,
		"created_at":   m.CreatedAt.Format("2006-01-02T15:04:05.999999"),
		"updated_at":   m.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
	})
}

func (h *ContactHandler) DeleteEmailMatch(w http.ResponseWriter, r *http.Request) {
	id, ok := parseMatchID(w, r)
	if !ok {
		return
	}
	deleted, err := h.svc.DeleteEmailMatch(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error deleting email match: %s", err))
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, fmt.Sprintf("email match not found: id=%d", id))
		return
	}
	writeJSON(w, map[string]any{"deleted": true, "id": id})
}

// ── Email exclusions ──────────────────────────────────────────────────────────

func (h *ContactHandler) ListEmailExclusions(w http.ResponseWriter, r *http.Request) {
	excls, err := h.svc.ListEmailExclusions(r.Context(),
		r.URL.Query().Get("search"),
		parseBoolQuery(r, "name_email"),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error listing email exclusions: %s", err))
		return
	}
	out := make([]map[string]any, 0, len(excls))
	for _, e := range excls {
		out = append(out, map[string]any{
			"id":         e.ID,
			"email":      e.Email,
			"name":       e.Name,
			"name_email": e.NameEmail,
			"created_at": e.CreatedAt.Format("2006-01-02T15:04:05.999999"),
			"updated_at": e.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
		})
	}
	writeJSON(w, out)
}

func (h *ContactHandler) GetEmailExclusion(w http.ResponseWriter, r *http.Request) {
	id, ok := parseExclID(w, r)
	if !ok {
		return
	}
	e, err := h.svc.GetEmailExclusionByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving email exclusion: %s", err))
		return
	}
	if e == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("email exclusion not found: id=%d", id))
		return
	}
	writeJSON(w, map[string]any{
		"id":         e.ID,
		"email":      e.Email,
		"name":       e.Name,
		"name_email": e.NameEmail,
		"created_at": e.CreatedAt.Format("2006-01-02T15:04:05.999999"),
		"updated_at": e.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
	})
}

func (h *ContactHandler) CreateEmailExclusion(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email     string `json:"email"`
		Name      string `json:"name"`
		NameEmail bool   `json:"name_email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	e, err := h.svc.CreateEmailExclusion(r.Context(), req.Email, req.Name, req.NameEmail)
	if err != nil {
		if strings.HasPrefix(err.Error(), "conflict:") {
			writeError(w, http.StatusConflict, strings.TrimPrefix(err.Error(), "conflict:"))
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error creating email exclusion: %s", err))
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{
		"id":         e.ID,
		"email":      e.Email,
		"name":       e.Name,
		"name_email": e.NameEmail,
		"created_at": e.CreatedAt.Format("2006-01-02T15:04:05.999999"),
		"updated_at": e.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
	})
}

func (h *ContactHandler) UpdateEmailExclusion(w http.ResponseWriter, r *http.Request) {
	id, ok := parseExclID(w, r)
	if !ok {
		return
	}
	var req struct {
		Email     *string `json:"email"`
		Name      *string `json:"name"`
		NameEmail *bool   `json:"name_email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	e, err := h.svc.UpdateEmailExclusion(r.Context(), id, req.Email, req.Name, req.NameEmail)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error updating email exclusion: %s", err))
		return
	}
	if e == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("email exclusion not found: id=%d", id))
		return
	}
	writeJSON(w, map[string]any{
		"id":         e.ID,
		"email":      e.Email,
		"name":       e.Name,
		"name_email": e.NameEmail,
		"created_at": e.CreatedAt.Format("2006-01-02T15:04:05.999999"),
		"updated_at": e.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
	})
}

func (h *ContactHandler) DeleteEmailExclusion(w http.ResponseWriter, r *http.Request) {
	id, ok := parseExclID(w, r)
	if !ok {
		return
	}
	deleted, err := h.svc.DeleteEmailExclusion(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error deleting email exclusion: %s", err))
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, fmt.Sprintf("email exclusion not found: id=%d", id))
		return
	}
	writeJSON(w, map[string]any{"deleted": true, "id": id})
}

// ── Email classifications ─────────────────────────────────────────────────────

func (h *ContactHandler) ListEmailClassifications(w http.ResponseWriter, r *http.Request) {
	cls, err := h.svc.ListEmailClassifications(r.Context(),
		r.URL.Query().Get("name"),
		r.URL.Query().Get("classification"),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error listing email classifications: %s", err))
		return
	}
	out := make([]map[string]any, 0, len(cls))
	for _, c := range cls {
		out = append(out, map[string]any{
			"id":             c.ID,
			"name":           c.Name,
			"classification": c.Classification,
			"created_at":     c.CreatedAt.Format("2006-01-02T15:04:05.999999"),
			"updated_at":     c.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
		})
	}
	writeJSON(w, out)
}

func (h *ContactHandler) GetEmailClassification(w http.ResponseWriter, r *http.Request) {
	id, ok := parseClsID(w, r)
	if !ok {
		return
	}
	c, err := h.svc.GetEmailClassificationByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error retrieving email classification: %s", err))
		return
	}
	if c == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("email classification not found: id=%d", id))
		return
	}
	writeJSON(w, map[string]any{
		"id":             c.ID,
		"name":           c.Name,
		"classification": c.Classification,
		"created_at":     c.CreatedAt.Format("2006-01-02T15:04:05.999999"),
		"updated_at":     c.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
	})
}

func (h *ContactHandler) CreateEmailClassification(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name           string `json:"name"`
		Classification string `json:"classification"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Classification = strings.TrimSpace(req.Classification)
	if req.Name == "" || req.Classification == "" {
		writeError(w, http.StatusBadRequest, "name and classification are required")
		return
	}
	c, err := h.svc.CreateEmailClassification(r.Context(), req.Name, req.Classification)
	if err != nil {
		if strings.HasPrefix(err.Error(), "conflict:") {
			writeError(w, http.StatusConflict, strings.TrimPrefix(err.Error(), "conflict:"))
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error creating email classification: %s", err))
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{
		"id":             c.ID,
		"name":           c.Name,
		"classification": c.Classification,
		"created_at":     c.CreatedAt.Format("2006-01-02T15:04:05.999999"),
		"updated_at":     c.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
	})
}

func (h *ContactHandler) UpdateEmailClassification(w http.ResponseWriter, r *http.Request) {
	id, ok := parseClsID(w, r)
	if !ok {
		return
	}
	var req struct {
		Name           *string `json:"name"`
		Classification *string `json:"classification"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	c, err := h.svc.UpdateEmailClassification(r.Context(), id, req.Name, req.Classification)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error updating email classification: %s", err))
		return
	}
	if c == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("email classification not found: id=%d", id))
		return
	}
	writeJSON(w, map[string]any{
		"id":             c.ID,
		"name":           c.Name,
		"classification": c.Classification,
		"created_at":     c.CreatedAt.Format("2006-01-02T15:04:05.999999"),
		"updated_at":     c.UpdatedAt.Format("2006-01-02T15:04:05.999999"),
	})
}

func (h *ContactHandler) DeleteEmailClassification(w http.ResponseWriter, r *http.Request) {
	id, ok := parseClsID(w, r)
	if !ok {
		return
	}
	deleted, err := h.svc.DeleteEmailClassification(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error deleting email classification: %s", err))
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, fmt.Sprintf("email classification not found: id=%d", id))
		return
	}
	writeJSON(w, map[string]any{"deleted": true, "id": id})
}

// GetEmailClassificationOptions returns valid classification values for the dropdown.
// Matches Python relationships.REL_TYPE_KEYS.
func (h *ContactHandler) GetEmailClassificationOptions(w http.ResponseWriter, r *http.Request) {
	classifications := []string{
		"friend", "family", "colleague", "acquaintance", "business",
		"social", "promotional", "spam", "important", "unknown",
	}
	writeJSON(w, map[string]any{"classifications": classifications})
}
