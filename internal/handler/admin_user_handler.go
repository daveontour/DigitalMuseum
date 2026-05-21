package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/daveontour/aimuseum/internal/appctx"
	appcrypto "github.com/daveontour/aimuseum/internal/crypto"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

const adminSessionCookieName = "dm_admin_sid"
const adminSessionTTL = 2 * time.Hour

// AdminUsersHandler provides the /admin UI and user-management API.
// Authentication uses the users table: the caller must have is_admin = true.
// Admin sessions are kept in an in-process map — separate from user sessions.
type AdminUsersHandler struct {
	userRepo         *repository.UserRepo
	authSvc          *service.AuthService
	sensitiveSvc     *service.SensitiveService
	subjectConfigSvc *service.SubjectConfigService
	dashboardSvc     *service.DashboardService
	billing          *repository.BillingRepo
	appInstr         *repository.AppSystemInstructionsRepo
	secure           bool
	sessions         adminSessions
}

// NewAdminUsersHandler creates an AdminUsersHandler.
func NewAdminUsersHandler(
	userRepo *repository.UserRepo,
	authSvc *service.AuthService,
	sensitiveSvc *service.SensitiveService,
	subjectConfigSvc *service.SubjectConfigService,
	dashboardSvc *service.DashboardService,
	billing *repository.BillingRepo,
	appInstr *repository.AppSystemInstructionsRepo,
	secure bool,
) *AdminUsersHandler {
	h := &AdminUsersHandler{
		userRepo:         userRepo,
		authSvc:          authSvc,
		sensitiveSvc:     sensitiveSvc,
		subjectConfigSvc: subjectConfigSvc,
		dashboardSvc:     dashboardSvc,
		billing:          billing,
		appInstr:         appInstr,
		secure:           secure,
	}
	h.sessions.m = make(map[string]time.Time)
	go h.sessions.cleanupLoop()
	return h
}

// RegisterRoutes mounts the admin routes.
func (h *AdminUsersHandler) RegisterRoutes(r chi.Router) {
	r.Get("/admin", h.GetPage)
	r.Post("/admin/login", h.Login)
	r.Post("/admin/logout", h.Logout)
	r.Get("/admin/users", h.ListUsers)
	r.Post("/admin/users", h.CreateUser)
	r.Patch("/admin/users/{id}", h.PatchUser)
	r.Delete("/admin/users/{id}", h.DeleteUser)
	r.Get("/admin/users/{id}/dashboard", h.GetUserDashboard)
	r.Get("/admin/llm-usage/users/{id}/summary", h.GetLLMUsageSummary)
	r.Get("/admin/llm-usage/users/{id}/events", h.GetLLMUsageEvents)
	r.Get("/admin/llm-usage/users/{id}/timeseries", h.GetLLMUsageTimeseries)
	r.Get("/admin/llm-usage/users/{id}/bill.pdf", h.GetLLMUsageBillPDF)
	r.Get("/admin/llm-usage/error-events", h.GetLLMUsageErrorEvents)
	r.Get("/admin/system-instructions", h.GetAdminSystemInstructions)
	r.Put("/admin/system-instructions", h.PutAdminSystemInstructions)
	r.Get("/admin/pambot-instructions", h.GetAdminPamBotInstructions)
	r.Put("/admin/pambot-instructions", h.PutAdminPamBotInstructions)
}

// GetAdminSystemInstructions returns the universal LLM system prompts (admin session).
func (h *AdminUsersHandler) GetAdminSystemInstructions(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.appInstr == nil {
		writeError(w, http.StatusServiceUnavailable, "system instructions store not configured")
		return
	}
	ins, err := h.appInstr.Get(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load system instructions")
		return
	}
	writeJSON(w, map[string]string{
		"system_instructions":          ins.ChatInstructions,
		"core_system_instructions":     ins.CoreInstructions,
		"question_system_instructions": ins.QuestionInstructions,
	})
}

// PutAdminSystemInstructions replaces the universal LLM system prompts (admin session).
func (h *AdminUsersHandler) PutAdminSystemInstructions(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.appInstr == nil {
		writeError(w, http.StatusServiceUnavailable, "system instructions store not configured")
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
	if err := h.appInstr.Upsert(r.Context(), body.SystemInstructions, body.CoreSystemInstructions, body.QuestionSystemInstructions); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save system instructions")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetAdminPamBotInstructions returns the Pam Bot companion persona instructions (admin session).
func (h *AdminUsersHandler) GetAdminPamBotInstructions(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.appInstr == nil {
		writeError(w, http.StatusServiceUnavailable, "system instructions store not configured")
		return
	}
	ins, err := h.appInstr.Get(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load pam bot instructions")
		return
	}
	writeJSON(w, map[string]string{"pam_bot_instructions": ins.PamBotInstructions})
}

// PutAdminPamBotInstructions replaces the Pam Bot companion persona instructions (admin session).
func (h *AdminUsersHandler) PutAdminPamBotInstructions(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.appInstr == nil {
		writeError(w, http.StatusServiceUnavailable, "system instructions store not configured")
		return
	}
	var body struct {
		PamBotInstructions string `json:"pam_bot_instructions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.appInstr.UpsertPamBotInstructions(r.Context(), body.PamBotInstructions); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save pam bot instructions")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /admin — serves the admin HTML page.
func (h *AdminUsersHandler) GetPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(adminPageHTML))
}

// POST /admin/login — { "email": "...", "password": "..." }
func (h *AdminUsersHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if _, err := h.authSvc.AdminLogin(r.Context(), req.Email, req.Password); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	sid := h.sessions.create()
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    sid,
		Path:     "/admin",
		MaxAge:   int(adminSessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   h.secure,
	})
	writeJSON(w, map[string]bool{"ok": true})
}

// POST /admin/logout
func (h *AdminUsersHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(adminSessionCookieName); err == nil {
		h.sessions.delete(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    "",
		Path:     "/admin",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   h.secure,
	})
	writeJSON(w, map[string]bool{"ok": true})
}

// GET /admin/users
func (h *AdminUsersHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	users, err := h.userRepo.ListAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	type userRow struct {
		ID                 int64  `json:"id"`
		Email              string `json:"email"`
		DisplayName        string `json:"display_name"`
		FirstName          string `json:"first_name"`
		FamilyName         string `json:"family_name"`
		IsActive           bool   `json:"is_active"`
		AllowServerLLMKeys bool   `json:"allow_server_llm_keys"`
		CreatedAt          string `json:"created_at"`
	}
	out := make([]userRow, 0, len(users))
	for _, u := range users {
		out = append(out, userRow{
			ID:                 u.ID,
			Email:              u.Email,
			DisplayName:        u.DisplayName,
			FirstName:          u.FirstName,
			FamilyName:         u.FamilyName,
			IsActive:           u.IsActive,
			AllowServerLLMKeys: u.AllowServerLLMKeys,
			CreatedAt:          u.CreatedAt.Format(time.RFC3339),
		})
	}
	writeJSON(w, out)
}

// PatchUser updates admin-editable fields on a user (currently allow_server_llm_keys only).
func (h *AdminUsersHandler) PatchUser(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var req struct {
		AllowServerLLMKeys *bool `json:"allow_server_llm_keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.AllowServerLLMKeys == nil {
		writeError(w, http.StatusBadRequest, "allow_server_llm_keys is required")
		return
	}
	if err := h.userRepo.SetAllowServerLLMKeys(r.Context(), id, *req.AllowServerLLMKeys); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update user")
		return
	}
	writeJSON(w, map[string]any{
		"ok":                    true,
		"allow_server_llm_keys": *req.AllowServerLLMKeys,
	})
}

// DELETE /admin/users/{id}
func (h *AdminUsersHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := h.userRepo.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /admin/users — { "email": "...", "password": "...", "display_name": "...", "family_name": "...", "gender": "..." }
func (h *AdminUsersHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	var req struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		FamilyName  string `json:"family_name"`
		Gender      string `json:"gender"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.Email == "" || req.Password == "" || req.DisplayName == "" {
		writeError(w, http.StatusBadRequest, "email, password, and first name are required")
		return
	}
	if len(req.Password) < 12 {
		writeError(w, http.StatusUnprocessableEntity, "password must be at least 12 characters")
		return
	}
	if req.Gender == "" {
		req.Gender = "Male"
	}

	hash, err := appcrypto.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	familyName := strings.TrimSpace(req.FamilyName)
	user, err := h.userRepo.Create(r.Context(), req.Email, hash, req.DisplayName, req.DisplayName, familyName)
	if err != nil {
		writeError(w, http.StatusConflict, "could not create user — email may already be registered")
		return
	}

	userCtx := context.WithValue(r.Context(), appctx.ContextKeyUserID, user.ID)

	// Initialise keyring so the user is ready to unlock immediately on first login.
	if err := h.sensitiveSvc.InitKeyring(userCtx, req.Password); err != nil {
		slog.Warn("admin: keyring init failed", "user_id", user.ID, "err", err)
	}

	// Create the subject configuration.
	gender := req.Gender
	if _, err := h.subjectConfigSvc.CreateOrUpdate(userCtx, service.SubjectConfigUpdateParams{
		SubjectName: req.DisplayName,
		FamilyName:  &familyName,
		Gender:      &gender,
	}); err != nil {
		slog.Warn("admin: subject config init failed", "user_id", user.ID, "err", err)
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{
		"id":           user.ID,
		"email":        user.Email,
		"display_name": user.DisplayName,
		"first_name":   user.FirstName,
		"family_name":  user.FamilyName,
	})
}

// GET /admin/users/{id}/dashboard
func (h *AdminUsersHandler) GetUserDashboard(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	userCtx := context.WithValue(r.Context(), appctx.ContextKeyUserID, id)
	dash, err := h.dashboardSvc.GetDashboard(userCtx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load dashboard")
		return
	}
	writeJSON(w, dash)
}

// GET /admin/llm-usage/users/{id}/summary?from=&to= (optional RFC3339 times)
func (h *AdminUsersHandler) GetLLMUsageSummary(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.billing == nil || h.billing.PgxPool() == nil {
		writeError(w, http.StatusServiceUnavailable, "billing database not configured")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	from, to, err := parseLLMUsageTimeRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid from or to (use RFC3339)")
		return
	}
	sum, byProv, byVis, err := h.billing.SummaryByUser(r.Context(), id, from, to)
	if err != nil {
		slog.Error("llm usage summary", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to load summary")
		return
	}
	writeJSON(w, map[string]any{
		"user_id":             id,
		"total_input_tokens":  sum.TotalInputTokens,
		"total_output_tokens": sum.TotalOutputTokens,
		"event_count":         sum.EventCount,
		"by_provider":         byProv,
		"by_visitor":          byVis,
	})
}

// GET /admin/llm-usage/users/{id}/events?from=&to=&limit=&offset=
func (h *AdminUsersHandler) GetLLMUsageEvents(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.billing == nil || h.billing.PgxPool() == nil {
		writeError(w, http.StatusServiceUnavailable, "billing database not configured")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	from, to, err := parseLLMUsageTimeRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid from or to (use RFC3339)")
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, e := strconv.Atoi(v); e == nil && n >= 0 {
			offset = n
		}
	}
	events, err := h.billing.ListEventsByUser(r.Context(), id, from, to, limit, offset)
	if err != nil {
		slog.Error("llm usage events", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to load events")
		return
	}
	if events == nil {
		events = []repository.LLMUsageEvent{}
	}
	writeJSON(w, map[string]any{"events": events, "limit": limit, "offset": offset})
}

// GET /admin/llm-usage/error-events?user_id=&from=&to=&limit=&offset=
// user_id empty or "all" returns failed events for all users; otherwise filters to that user.
func (h *AdminUsersHandler) GetLLMUsageErrorEvents(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.billing == nil || h.billing.PgxPool() == nil {
		writeError(w, http.StatusServiceUnavailable, "billing database not configured")
		return
	}
	var userID *int64
	if q := strings.TrimSpace(r.URL.Query().Get("user_id")); q != "" && !strings.EqualFold(q, "all") {
		id, err := strconv.ParseInt(q, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "invalid user_id")
			return
		}
		userID = &id
	}
	from, to, err := parseLLMUsageTimeRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid from or to (use RFC3339)")
		return
	}
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, e := strconv.Atoi(v); e == nil && n >= 0 {
			offset = n
		}
	}
	events, err := h.billing.ListFailedEvents(r.Context(), userID, from, to, limit, offset)
	if err != nil {
		slog.Error("llm usage error events", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to load error events")
		return
	}
	if events == nil {
		events = []repository.LLMUsageEvent{}
	}
	resp := map[string]any{"events": events, "limit": limit, "offset": offset}
	if userID != nil {
		resp["user_id"] = *userID
	} else {
		resp["user_id"] = nil
	}
	writeJSON(w, resp)
}

// GET /admin/llm-usage/users/{id}/timeseries?from=&to=
func (h *AdminUsersHandler) GetLLMUsageTimeseries(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.billing == nil || h.billing.PgxPool() == nil {
		writeError(w, http.StatusServiceUnavailable, "billing database not configured")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	from, to, err := parseLLMUsageTimeRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid from or to (use RFC3339)")
		return
	}
	buckets, err := h.billing.TimeseriesByUser5Min(r.Context(), id, from, to)
	if err != nil {
		slog.Error("llm usage timeseries", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to load timeseries")
		return
	}
	if buckets == nil {
		buckets = []repository.TimeseriesBucket{}
	}
	out := make([]map[string]any, 0, len(buckets))
	for _, b := range buckets {
		out = append(out, map[string]any{
			"bucket_start":  b.BucketStart.UTC().Format(time.RFC3339),
			"input_tokens":  b.InputTokens,
			"output_tokens": b.OutputTokens,
		})
	}
	writeJSON(w, map[string]any{"buckets": out})
}

// GET /admin/llm-usage/users/{id}/bill.pdf?from=&to=
func (h *AdminUsersHandler) GetLLMUsageBillPDF(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.billing == nil || h.billing.PgxPool() == nil {
		writeError(w, http.StatusServiceUnavailable, "billing database not configured")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	from, to, err := parseLLMUsageTimeRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid from or to (use RFC3339)")
		return
	}
	WriteLLMUsageBillPDF(w, r, h.userRepo, h.billing, id, from, to)
}

func parseLLMUsageTimeRange(r *http.Request) (from, to *time.Time, err error) {
	q := r.URL.Query()
	if s := q.Get("from"); s != "" {
		t, e := time.Parse(time.RFC3339, s)
		if e != nil {
			return nil, nil, e
		}
		from = &t
	}
	if s := q.Get("to"); s != "" {
		t, e := time.Parse(time.RFC3339, s)
		if e != nil {
			return nil, nil, e
		}
		to = &t
	}
	return from, to, nil
}

// requireAdmin checks the admin session cookie and returns false (writing 401) if invalid.
func (h *AdminUsersHandler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	c, err := r.Cookie(adminSessionCookieName)
	if err != nil || !h.sessions.valid(c.Value) {
		writeError(w, http.StatusUnauthorized, "admin authentication required")
		return false
	}
	return true
}

// RequireAdmin is an exported shim so ProfileHandler can reuse admin session logic.
func (h *AdminUsersHandler) RequireAdmin(w http.ResponseWriter, r *http.Request) bool {
	return h.requireAdmin(w, r)
}

// ── In-process admin session store ───────────────────────────────────────────

type adminSessions struct {
	mu sync.Mutex
	m  map[string]time.Time // sessionID → expiry
}

func (s *adminSessions) create() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	sid := hex.EncodeToString(b)
	s.mu.Lock()
	s.m[sid] = time.Now().Add(adminSessionTTL)
	s.mu.Unlock()
	return sid
}

func (s *adminSessions) valid(sid string) bool {
	s.mu.Lock()
	exp, ok := s.m[sid]
	s.mu.Unlock()
	return ok && time.Now().Before(exp)
}

func (s *adminSessions) delete(sid string) {
	s.mu.Lock()
	delete(s.m, sid)
	s.mu.Unlock()
}

func (s *adminSessions) cleanupLoop() {
	t := time.NewTicker(30 * time.Minute)
	defer t.Stop()
	for range t.C {
		now := time.Now()
		s.mu.Lock()
		for sid, exp := range s.m {
			if now.After(exp) {
				delete(s.m, sid)
			}
		}
		s.mu.Unlock()
	}
}

// ── Embedded admin page HTML ──────────────────────────────────────────────────

const adminPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Digital Museum — Admin</title>
<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.5.1/css/all.min.css">
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#0f1922;color:#cdd6e8;min-height:100vh}
.topbar{background:#19264d;padding:14px 24px;display:flex;align-items:center;justify-content:space-between;border-bottom:1px solid #2a3a6b}
.topbar h1{font-size:1.1rem;color:#fff;display:flex;align-items:center;gap:10px}
.topbar h1 i{color:#3b82f6}
#logout-btn{background:none;border:1px solid #2e4068;color:#8fa4c8;padding:6px 14px;border-radius:6px;cursor:pointer;font-size:0.85rem}
#logout-btn:hover{background:#2a3a6b;color:#fff}
.container{max-width:1300px;width:1300px;min-width:900px;margin:40px auto;padding:0 20px}
.card{background:#19264d;border-radius:10px;padding:32px;box-shadow:0 4px 24px rgba(0,0,0,0.3)}
.card-header{display:flex;align-items:center;justify-content:space-between;margin-bottom:20px}
.card-header h2{color:#fff;font-size:1.2rem;margin:0}
.form-group{margin-bottom:18px}
.form-group label{display:block;color:#8fa4c8;margin-bottom:6px;font-size:0.88rem}
.form-group input,.form-group select{width:100%;padding:10px 14px;border-radius:6px;border:1px solid #2e4068;background:#0f1922;color:#fff;font-size:1rem;outline:none}
.form-group input:focus,.form-group select:focus{border-color:#3b82f6}
.form-group textarea{width:100%;padding:10px 14px;border-radius:6px;border:1px solid #2e4068;background:#0f1922;color:#fff;font-size:0.95rem;outline:none;font-family:inherit;min-height:140px;resize:vertical;line-height:1.45}
.form-group textarea:focus{border-color:#3b82f6}
.form-row{display:grid;grid-template-columns:1fr 1fr;gap:14px}
.admin-pw-wrap{display:flex;align-items:stretch;width:100%;border-radius:6px;border:1px solid #2e4068;background:#0f1922;overflow:hidden}
.admin-pw-wrap:focus-within{border-color:#3b82f6}
.admin-pw-wrap input{flex:1;min-width:0;border:none!important;border-radius:0!important;background:transparent!important;padding:10px 12px!important}
.admin-pw-toggle{flex:0 0 44px;border:none;background:#162536;color:#8fa4c8;cursor:pointer;display:flex;align-items:center;justify-content:center;font-size:1rem;transition:background .15s,color .15s}
.admin-pw-toggle:hover{background:#243368;color:#e2e8f0}
.admin-archive-visible-login .admin-archive-visible-field-label{display:block;color:#8fa4c8;margin-bottom:6px;font-size:0.88rem}
.admin-archive-visible-login .admin-archive-enabled-label{display:flex;align-items:center;gap:8px;color:#cdd6e8;font-size:0.88rem;cursor:pointer;margin-top:0;width:fit-content;max-width:100%}
.btn{padding:10px 20px;border-radius:6px;border:none;font-size:0.95rem;font-weight:600;cursor:pointer;transition:background 0.15s}
.btn-primary{background:#3b82f6;color:#fff}.btn-primary:hover{background:#2563eb}
.btn-primary:disabled{background:#1e3a6b;cursor:not-allowed}
.btn-secondary{background:#2e4068;color:#e2e8f0}.btn-secondary:hover{background:#3d5288;color:#fff}
.btn-success{background:#16a34a;color:#fff;padding:8px 16px;font-size:0.88rem}.btn-success:hover{background:#15803d}
.btn-danger{background:#dc2626;color:#fff;padding:6px 12px;font-size:0.82rem}.btn-danger:hover{background:#b91c1c}
.btn-warning{background:#d97706;color:#fff;padding:6px 12px;font-size:0.82rem}.btn-warning:hover{background:#b45309}
.btn-compact-secondary{background:#2e4068;color:#e2e8f0;padding:6px 12px;font-size:0.82rem;font-weight:600;border-radius:6px;border:none;cursor:pointer;transition:background .15s,color .15s}
.btn-compact-secondary:hover:not(:disabled){background:#3d5288;color:#fff}
.btn-compact-secondary:disabled{opacity:0.45;cursor:not-allowed}
.btn-info{background:#0e7490;color:#fff;padding:6px 12px;font-size:0.82rem}.btn-info:hover{background:#0c5f75}
.error{color:#f87171;background:rgba(248,113,113,0.1);border:1px solid rgba(248,113,113,0.25);padding:10px 14px;border-radius:6px;margin-bottom:16px;font-size:0.9rem;display:none}
#login-section{display:flex;align-items:center;justify-content:center;min-height:100vh}
#admin-user-login-btn{position:fixed;top:14px;right:14px;z-index:100;padding:8px 14px;border-radius:6px;border:1px solid #2e4068;background:#19264d;color:#cdd6e8;font-size:0.88rem;font-weight:600;cursor:pointer;font-family:inherit;transition:background .15s,border-color .15s,color .15s}
#admin-user-login-btn:hover{background:#243368;border-color:#3b82f6;color:#fff}
#admin-section{display:none}
table{width:100%;border-collapse:collapse}
th{text-align:left;color:#8fa4c8;font-size:0.82rem;font-weight:600;padding:8px 12px;border-bottom:1px solid #2a3a6b}
td{padding:10px 12px;border-bottom:1px solid #1e2d4d;font-size:0.9rem;vertical-align:middle}
#llm-events-table,#err-events-table{table-layout:fixed;width:100%}
#llm-events-table td.llm-col-error,#err-events-table td.llm-col-error{word-break:break-word;overflow-wrap:anywhere;white-space:normal;vertical-align:top;font-size:0.82rem;line-height:1.35}
tr:last-child td{border-bottom:none}
tr:hover td{background:rgba(255,255,255,0.02)}
.badge{display:inline-block;padding:2px 8px;border-radius:4px;font-size:0.75rem;font-weight:600}
.badge-active{background:rgba(34,197,94,0.15);color:#4ade80}
.badge-inactive{background:rgba(248,113,113,0.15);color:#f87171}
.actions{display:flex;gap:6px;flex-wrap:wrap}
.modal-overlay{display:none;position:fixed;inset:0;background:rgba(0,0,0,0.6);z-index:100;align-items:center;justify-content:center;padding:20px}
.modal-overlay.open{display:flex}
.modal{background:#19264d;border-radius:10px;padding:28px;width:100%;max-width:460px;box-shadow:0 8px 32px rgba(0,0,0,0.5);max-height:90vh;overflow-y:auto}
.modal-wide{max-width:720px}
.modal h3{color:#fff;margin-bottom:18px}
.modal-actions{display:flex;gap:10px;justify-content:flex-end;margin-top:20px}
.btn-ghost{background:none;border:1px solid #2e4068;color:#8fa4c8;padding:8px 16px;font-size:0.9rem;border-radius:6px;cursor:pointer}.btn-ghost:hover{background:#2a3a6b}
.stat-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(180px,1fr));gap:12px;margin-bottom:20px}
.stat-card{background:#0f1922;border-radius:8px;padding:14px 16px;border:1px solid #2a3a6b}
.stat-card .label{color:#8fa4c8;font-size:0.78rem;margin-bottom:4px}
.stat-card .value{color:#fff;font-size:1.4rem;font-weight:700}
.stat-section{color:#3b82f6;font-size:0.8rem;font-weight:700;text-transform:uppercase;letter-spacing:0.05em;margin:16px 0 8px}
.admin-srv-keys-cell{cursor:pointer;display:inline-flex;align-items:center;justify-content:center}
.admin-srv-keys-cell input{width:18px;height:18px;cursor:pointer;accent-color:#3b82f6}
.admin-tabs{display:flex;gap:4px;padding:0 8px;border-bottom:1px solid #2a3a6b;background:#19264d;border-radius:10px 10px 0 0}
.admin-tab{background:none;border:none;color:#8fa4c8;padding:14px 22px;font-size:0.95rem;font-weight:600;cursor:pointer;border-bottom:2px solid transparent;margin-bottom:-1px;border-radius:0}
.admin-tab:hover{color:#cdd6e8}
.admin-tab.active{color:#fff;border-bottom-color:#3b82f6}
.admin-tab-panel{display:none;padding:32px}
.admin-tab-panel.active{display:block}
.admin-tab-shell{background:#19264d;border-radius:10px;overflow:hidden;box-shadow:0 4px 24px rgba(0,0,0,0.3)}
</style>
</head>
<body>
<button type="button" id="admin-user-login-btn">User Login</button>

<!-- Login screen -->
<div id="login-section">
  <div class="card" style="max-width:380px;width:100%">
    <h2 style="color:#fff;margin-bottom:20px"><i class="fas fa-lock" style="color:#3b82f6;margin-right:8px"></i>Admin Login</h2>
    <div id="login-error" class="error"></div>
    <div class="form-group">
      <label for="admin-email">Email</label>
      <input id="admin-email" type="email" placeholder="admin@example.com" autocomplete="username">
    </div>
    <div class="form-group">
      <label for="admin-password">Password</label>
      <input id="admin-password" type="password" placeholder="Enter admin password" autocomplete="current-password">
    </div>
    <button id="login-btn" class="btn btn-primary" style="width:100%">
      <i class="fas fa-sign-in-alt" style="margin-right:6px"></i>Sign In
    </button>
  </div>
</div>

<!-- Admin panel -->
<div id="admin-section">
  <div class="topbar">
    <h1><i class="fas fa-shield-halved"></i>Digital Museum — Admin</h1>
    <button id="logout-btn"><i class="fas fa-sign-out-alt" style="margin-right:6px"></i>Logout</button>
  </div>
  <div class="container">
    <div class="card admin-tab-shell">
      <div class="admin-tabs" role="tablist">
        <button type="button" class="admin-tab active" id="tab-btn-users" role="tab" aria-selected="true" aria-controls="tab-panel-users">Users</button>
        <button type="button" class="admin-tab" id="tab-btn-billing" role="tab" aria-selected="false" aria-controls="tab-panel-billing">Usage &amp; Billing</button>
        <button type="button" class="admin-tab" id="tab-btn-errors" role="tab" aria-selected="false" aria-controls="tab-panel-errors">Errors</button>
        <button type="button" class="admin-tab" id="tab-btn-sys" role="tab" aria-selected="false" aria-controls="tab-panel-sys">System instructions</button>
        <button type="button" class="admin-tab" id="tab-btn-pambot" role="tab" aria-selected="false" aria-controls="tab-panel-pambot">Pam Bot</button>
        <button type="button" class="admin-tab" id="tab-btn-archives" role="tab" aria-selected="false" aria-controls="tab-panel-archives">Archives</button>
      </div>
      <div id="tab-panel-users" class="admin-tab-panel active" role="tabpanel">
      <div class="card-header" style="padding:0 0 20px 0;margin-bottom:0">
        <h2 style="margin:0">Users</h2>
        <button class="btn btn-success" id="add-user-btn">
          <i class="fas fa-user-plus" style="margin-right:6px"></i>Add User
        </button>
      </div>
      <div id="users-error" class="error"></div>
      <table id="users-table">
        <thead>
          <tr>
            <th>ID</th><th>Email</th><th>Name</th><th>Status</th><th title="When enabled, user may use server GEMINI / ANTHROPIC / TAVILY keys if they have not set their own">Server keys</th><th>Registered</th><th>Actions</th>
          </tr>
        </thead>
        <tbody id="users-body"></tbody>
      </table>
      </div>

      <div id="tab-panel-billing" class="admin-tab-panel" role="tabpanel">
      <div class="card-header" style="padding:0 0 16px 0;margin-bottom:0">
        <h2 style="margin:0">LLM usage (billing)</h2>
      </div>
      <p style="color:#8fa4c8;font-size:0.88rem;margin-bottom:16px">Token totals and call counts include every LLM API attempt (success or failure). Failed rows show the error message. Chart: 5-minute buckets (input stacked below output).</p>
      <div class="form-row">
        <div class="form-group">
          <label for="llm-user-select">User</label>
          <select id="llm-user-select"><option value="">— Select user —</option></select>
        </div>
        <div class="form-group">
          <label for="llm-from">From (optional)</label>
          <input id="llm-from" type="datetime-local" />
        </div>
        <div class="form-group">
          <label for="llm-to">To (optional)</label>
          <input id="llm-to" type="datetime-local" />
        </div>
      </div>
      <div style="display:flex;flex-wrap:wrap;gap:10px;align-items:center">
      <button class="btn btn-primary" id="llm-load-btn" type="button">
        <i class="fas fa-sync-alt" style="margin-right:6px"></i>Load usage
      </button>
      <button class="btn btn-secondary" id="llm-pdf-btn" type="button" title="Download PDF usage statement for the selected user and date range">
        <i class="fas fa-file-pdf" style="margin-right:6px"></i>Download PDF bill
      </button>
      </div>
      <div id="llm-error" class="error" style="margin-top:12px"></div>
      <div id="llm-summary" class="stat-grid" style="margin-top:20px;display:none"></div>
      <div style="margin-top:20px">
        <div style="color:#8fa4c8;font-size:0.8rem;margin-bottom:8px">Tokens per 5-minute window</div>
        <canvas id="llm-usage-canvas" width="900" height="240" style="width:100%;max-width:100%;height:240px;background:#0f1922;border-radius:8px;border:1px solid #2a3a6b"></canvas>
        <div style="margin-top:8px;font-size:0.78rem;color:#8fa4c8">
          <span style="display:inline-block;width:10px;height:10px;background:#3b82f6;margin-right:6px;vertical-align:middle"></span>Input
          <span style="display:inline-block;width:10px;height:10px;background:#22c55e;margin-left:12px;margin-right:6px;vertical-align:middle"></span>Output
        </div>
      </div>
      <div style="margin-top:20px;overflow-x:auto">
        <table id="llm-events-table" style="min-width:600px;display:none">
          <thead><tr><th>Time (UTC)</th><th>Email</th><th>First name</th><th>Family name</th><th>Provider</th><th>Visitor</th><th>API key</th><th>In</th><th>Out</th><th>OK</th><th style="width:22%">Error</th><th>Model</th></tr></thead>
          <tbody id="llm-events-body"></tbody>
        </table>
      </div>
      </div>

      <div id="tab-panel-errors" class="admin-tab-panel" role="tabpanel">
      <div class="card-header" style="padding:0 0 16px 0;margin-bottom:0">
        <h2 style="margin:0">LLM errors</h2>
      </div>
      <p style="color:#8fa4c8;font-size:0.88rem;margin-bottom:16px">Failed LLM API calls only. Uses the same optional date range as billing (local time fields are converted to UTC for the query).</p>
      <div class="form-row">
        <div class="form-group">
          <label for="err-user-select">User</label>
          <select id="err-user-select"><option value="all">All users</option></select>
        </div>
        <div class="form-group">
          <label for="err-from">From (optional)</label>
          <input id="err-from" type="datetime-local" />
        </div>
        <div class="form-group">
          <label for="err-to">To (optional)</label>
          <input id="err-to" type="datetime-local" />
        </div>
      </div>
      <div style="display:flex;flex-wrap:wrap;gap:10px;align-items:center">
      <button class="btn btn-primary" id="err-load-btn" type="button">
        <i class="fas fa-sync-alt" style="margin-right:6px"></i>Load errors
      </button>
      </div>
      <div id="err-panel-error" class="error" style="margin-top:12px"></div>
      <p id="err-summary" style="margin-top:16px;color:#8fa4c8;font-size:0.88rem;display:none"></p>
      <div style="margin-top:12px;overflow-x:auto">
        <table id="err-events-table" style="min-width:600px;display:none">
          <thead><tr><th>Time (UTC)</th><th>User ID</th><th>Email</th><th>First name</th><th>Family name</th><th>Provider</th><th>Visitor</th><th>API key</th><th>In</th><th>Out</th><th style="width:22%">Error</th><th>Model</th></tr></thead>
          <tbody id="err-events-body"></tbody>
        </table>
      </div>
      </div>

      <div id="tab-panel-sys" class="admin-tab-panel" role="tabpanel">
      <div class="card-header" style="padding:0 0 16px 0;margin-bottom:0">
        <h2 style="margin:0">Universal LLM system instructions</h2>
      </div>
      <p style="color:#8fa4c8;font-size:0.88rem;margin-bottom:16px">These prompts apply to all users for chat and related flows. Changes take effect on the next request.</p>
      <div id="sys-instr-error" class="error" style="margin-bottom:12px"></div>
      <p id="sys-instr-ok" style="display:none;color:#4ade80;font-size:0.88rem;margin-bottom:12px">Saved.</p>
      <div class="form-group">
        <label for="sys-core">Core system instructions</label>
        <textarea id="sys-core" rows="8" spellcheck="false" placeholder="Loading\u2026"></textarea>
      </div>
      <div class="form-group">
        <label for="sys-chat">Chat system instructions</label>
        <textarea id="sys-chat" rows="8" spellcheck="false" placeholder="Loading\u2026"></textarea>
      </div>
      <div class="form-group">
        <label for="sys-question">Question / short-answer instructions</label>
        <textarea id="sys-question" rows="6" spellcheck="false" placeholder="Loading\u2026"></textarea>
      </div>
      <div style="display:flex;flex-wrap:wrap;gap:10px;align-items:center">
        <button class="btn btn-secondary" id="sys-reload-btn" type="button"><i class="fas fa-sync-alt" style="margin-right:6px"></i>Reload from server</button>
        <button class="btn btn-primary" id="sys-save-btn" type="button"><i class="fas fa-save" style="margin-right:6px"></i>Save</button>
      </div>
      </div>

      <div id="tab-panel-pambot" class="admin-tab-panel" role="tabpanel">
      <div class="card-header" style="padding:0 0 16px 0;margin-bottom:0">
        <h2 style="margin:0"><i class="fas fa-heart" style="color:#3b82f6;margin-right:8px"></i>Pam Bot — companion persona instructions</h2>
      </div>
      <p style="color:#8fa4c8;font-size:0.88rem;margin-bottom:16px">These instructions define how the Pam Bot memory companion speaks and behaves. They are loaded from the database at the start of every interaction. Changes take effect on the next request.</p>
      <div id="pambot-instr-error" class="error" style="margin-bottom:12px"></div>
      <p id="pambot-instr-ok" style="display:none;color:#4ade80;font-size:0.88rem;margin-bottom:12px">Saved.</p>
      <div class="form-group">
        <label for="pambot-instructions">Companion persona instructions</label>
        <textarea id="pambot-instructions" rows="18" spellcheck="false" placeholder="Loading\u2026" style="font-family:monospace;font-size:0.92rem"></textarea>
      </div>
      <div style="display:flex;flex-wrap:wrap;gap:10px;align-items:center">
        <button class="btn btn-secondary" id="pambot-reload-btn" type="button"><i class="fas fa-sync-alt" style="margin-right:6px"></i>Reload from server</button>
        <button class="btn btn-primary" id="pambot-save-btn" type="button"><i class="fas fa-save" style="margin-right:6px"></i>Save</button>
      </div>
      </div>

      <div id="tab-panel-archives" class="admin-tab-panel" role="tabpanel">
      <div class="card-header" style="padding:0 0 16px;margin-bottom:0">
        <h2 style="margin:0"><i class="fas fa-archive" style="color:#3b82f6;margin-right:8px"></i>Archives</h2>
      </div>
      <p style="color:#8fa4c8;font-size:.88rem;margin-bottom:16px">
        Archive profiles stored in the admin database.
        Disabling an archive hides it from the login dropdown.
        The server opens the archive marked as <strong style="color:#cdd6e8">startup default</strong>. 
      </p>
      <div id="archives-error" class="error" style="margin-bottom:12px;display:none"></div>
      <div style="border:1px solid #2e4068;border-radius:8px;padding:12px;margin-bottom:12px;background:#0f1922">
        <p style="color:#8fa4c8;font-size:0.82rem;margin:0 0 10px 0"><span style="color:#cdd6e8;font-weight:600;margin-bottom:8px;font-size:1.0rem">Create New Archive</span> Creates a new SQLite file at the path below, runs migrations, and registers the first owner account in that archive.</p>
        <div class="form-row">
          <div class="form-group">
            <label for="archive-add-first-name">First name *</label>
            <input id="archive-add-first-name" type="text" placeholder="e.g. Jane">
          </div>
          <div class="form-group">
            <label for="archive-add-family-name">Family name *</label>
            <input id="archive-add-family-name" type="text" placeholder="e.g. Smith">
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label for="archive-add-username">Username *</label>
            <input id="archive-add-username" type="text" placeholder="login username">
          </div>
          <div class="form-group admin-archive-visible-login">
            <label class="admin-archive-visible-field-label" for="archive-add-enabled">Visible on login</label>
            <label class="admin-archive-enabled-label" for="archive-add-enabled">
              <input id="archive-add-enabled" type="checkbox" checked style="accent-color:#3b82f6;width:auto;margin:0">
              Enabled
            </label>
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label for="archive-add-password">Password *</label>
            <div class="admin-pw-wrap">
              <input id="archive-add-password" type="password" placeholder="At least 12 characters" autocomplete="new-password">
              <button type="button" class="admin-pw-toggle" id="archive-add-password-toggle" aria-label="Show password" aria-pressed="false" title="Show password"><i class="far fa-eye" aria-hidden="true"></i></button>
            </div>
          </div>
          <div class="form-group">
            <label for="archive-add-password-confirm">Confirm password *</label>
            <div class="admin-pw-wrap">
              <input id="archive-add-password-confirm" type="password" placeholder="Re-enter" autocomplete="new-password">
              <button type="button" class="admin-pw-toggle" id="archive-add-password-confirm-toggle" aria-label="Show password" aria-pressed="false" title="Show password"><i class="far fa-eye" aria-hidden="true"></i></button>
            </div>
          </div>
        </div>
        <div class="form-group">
          <label for="archive-add-db-path">DB Path *</label>
          <div style="display:flex;gap:8px;align-items:center">
            <input id="archive-add-db-path" type="text" placeholder="Leave blank for default next to admin database" style="font-family:monospace;font-size:.88rem;flex:1">
            <button class="btn btn-secondary" id="archive-add-browse-btn" type="button"><i class="fas fa-folder-open" style="margin-right:6px"></i>Browse…</button>
          </div>
        </div>
        <div style="display:flex;align-items:center;gap:10px;flex-wrap:wrap;margin-top:8px">
          <button class="btn btn-primary" id="archive-add-btn" type="button"><i class="fas fa-plus" style="margin-right:6px"></i>Create archive</button>
        </div>
      </div>
      <div style="overflow-x:auto">
        <table id="archives-table" style="min-width:640px">
          <thead><tr>
            <th>Name</th><th>Username</th><th>DB Path</th>
            <th>Status</th><th>Startup default</th><th>Last Used</th><th>Actions</th>
          </tr></thead>
          <tbody id="archives-body"></tbody>
        </table>
      </div>
      </div>

    </div>
  </div>
</div>

<!-- Add user modal -->
<div class="modal-overlay" id="add-modal">
  <div class="modal">
    <h3><i class="fas fa-user-plus" style="color:#3b82f6;margin-right:8px"></i>Add User</h3>
    <div id="add-error" class="error"></div>
    <div class="form-group">
      <label>Email *</label>
      <input id="add-email" type="email" placeholder="user@example.com" autocomplete="off">
    </div>
    <div class="form-row">
      <div class="form-group">
        <label>First Name *</label>
        <input id="add-display-name" type="text" placeholder="e.g. Jane">
      </div>
      <div class="form-group">
        <label>Family Name</label>
        <input id="add-family-name" type="text" placeholder="e.g. Smith">
      </div>
    </div>
    <div class="form-group">
      <label>Gender</label>
      <select id="add-gender">
        <option value="Male">Male</option>
        <option value="Female">Female</option>
      </select>
    </div>
    <div class="form-row">
      <div class="form-group">
        <label>Password * (min 12 chars)</label>
        <input id="add-password" type="password" placeholder="Password" autocomplete="new-password">
      </div>
      <div class="form-group">
        <label>Confirm Password *</label>
        <input id="add-password-confirm" type="password" placeholder="Confirm" autocomplete="new-password">
      </div>
    </div>
    <div class="modal-actions">
      <button class="btn btn-ghost" id="add-cancel">Cancel</button>
      <button class="btn btn-primary" id="add-save">
        <i class="fas fa-user-plus" style="margin-right:6px"></i>Create User
      </button>
    </div>
  </div>
</div>

<!-- Delete confirm modal -->
<div class="modal-overlay" id="del-modal">
  <div class="modal">
    <h3>Delete User</h3>
    <p style="color:#f87171;margin-bottom:8px" id="del-modal-label"></p>
    <p style="color:#8fa4c8;font-size:0.88rem">This permanently deletes the user and all their archive data. This cannot be undone.</p>
    <div class="modal-actions">
      <button class="btn btn-ghost" id="del-cancel">Cancel</button>
      <button class="btn btn-danger" id="del-confirm">Delete</button>
    </div>
  </div>
</div>

<!-- Delete archive confirm modal -->
<div class="modal-overlay" id="del-archive-modal">
  <div class="modal">
    <h3>Delete Archive</h3>
    <p style="color:#f87171;margin-bottom:8px" id="del-archive-modal-label"></p>
    <p style="color:#8fa4c8;font-size:0.88rem;margin-bottom:6px">DB file: <span id="del-archive-modal-path" style="font-family:monospace;font-size:.82rem;word-break:break-all"></span></p>
    <p style="color:#8fa4c8;font-size:0.88rem">This removes the archive entry from the billing database. The .sqlite file is kept on disk unless you tick the option below.</p>
    <label style="display:flex;align-items:center;gap:8px;color:#cdd6e8;font-size:0.9rem;margin-top:14px;cursor:pointer">
      <input type="checkbox" id="del-archive-file" style="accent-color:#3b82f6">
      Also delete the .sqlite file from disk (cannot be undone)
    </label>
    <p id="del-archive-error" style="color:#f87171;font-size:0.85rem;margin-top:10px;display:none"></p>
    <div class="modal-actions">
      <button class="btn btn-ghost" id="del-archive-cancel">Cancel</button>
      <button class="btn btn-danger" id="del-archive-confirm">Delete</button>
    </div>
  </div>
</div>

<!-- Dashboard stats modal -->
<div class="modal-overlay" id="stats-modal">
  <div class="modal modal-wide">
    <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:16px">
      <h3 id="stats-modal-title" style="margin:0"><i class="fas fa-chart-bar" style="color:#3b82f6;margin-right:8px"></i>Archive Statistics</h3>
      <button class="btn btn-ghost" id="stats-close-top" style="padding:4px 10px;font-size:1.1rem;line-height:1" title="Close">&times;</button>
    </div>
    <div id="stats-body"><p style="color:#8fa4c8;text-align:center;padding:24px">Loading\u2026</p></div>
    <div class="modal-actions">
      <button class="btn btn-ghost" id="stats-close">Close</button>
    </div>
  </div>
</div>

<script>
(function() {
  let pendingDelUserId = null;

  async function apiFetch(path, opts) {
    return fetch(path, { credentials: 'same-origin', ...opts });
  }

  /** Prefer server writeError JSON shape ({ detail }) over legacy { error }. */
  function apiErrorMessage(d, fallback) {
    if (!d || typeof d !== 'object') return fallback;
    if (typeof d.detail === 'string' && d.detail !== '') return d.detail;
    if (typeof d.error === 'string' && d.error !== '') return d.error;
    if (typeof d.message === 'string' && d.message !== '') return d.message;
    return fallback;
  }

  function showAdminUserLoginBtn(visible) {
    var el = document.getElementById('admin-user-login-btn');
    if (el) el.style.display = visible ? 'block' : 'none';
  }
  document.getElementById('admin-user-login-btn').addEventListener('click', function() {
    window.location.href = '/login';
  });

  // ── Login ──────────────────────────────────────────────────────────────────
  ['admin-email', 'admin-password'].forEach(function(id) {
    document.getElementById(id).addEventListener('keydown', function(e) {
      if (e.key === 'Enter') document.getElementById('login-btn').click();
    });
  });

  document.getElementById('login-btn').addEventListener('click', async function() {
    const btn = this;
    const errEl = document.getElementById('login-error');
    const email = document.getElementById('admin-email').value;
    const pw = document.getElementById('admin-password').value;
    errEl.style.display = 'none';
    btn.disabled = true;
    btn.innerHTML = '<i class="fas fa-spinner fa-spin" style="margin-right:6px"></i>Signing in\u2026';
    try {
      const res = await apiFetch('/admin/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: email, password: pw })
      });
      if (res.ok) { showAdmin(); }
      else {
        const d = await res.json().catch(() => ({}));
        errEl.textContent = apiErrorMessage(d, 'Invalid password.');
        errEl.style.display = 'block';
      }
    } catch (e) {
      errEl.textContent = 'Network error.';
      errEl.style.display = 'block';
    } finally {
      btn.disabled = false;
      btn.innerHTML = '<i class="fas fa-sign-in-alt" style="margin-right:6px"></i>Sign In';
    }
  });

  // ── Logout ─────────────────────────────────────────────────────────────────
  document.getElementById('logout-btn').addEventListener('click', async function() {
    await apiFetch('/admin/logout', { method: 'POST' });
    document.getElementById('admin-section').style.display = 'none';
    document.getElementById('login-section').style.display = 'flex';
    document.getElementById('admin-email').value = '';
    document.getElementById('admin-password').value = '';
    showAdminUserLoginBtn(true);
  });

  function showAdmin() {
    document.getElementById('login-section').style.display = 'none';
    document.getElementById('admin-section').style.display = 'block';
    showAdminUserLoginBtn(false);
    loadUsers();
  }

  window.setAllowServerKeys = async function(userId, allow) {
    try {
      const res = await apiFetch('/admin/users/' + userId, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ allow_server_llm_keys: allow })
      });
      if (res.status === 401) {
        document.getElementById('admin-section').style.display = 'none';
        document.getElementById('login-section').style.display = 'flex';
        showAdminUserLoginBtn(true);
        return;
      }
      if (!res.ok) {
        const d = await res.json().catch(() => ({}));
        alert(apiErrorMessage(d, 'Failed to update setting.'));
        loadUsers();
      }
    } catch (e) {
      alert('Network error.');
      loadUsers();
    }
  };

  // ── Load / render users ────────────────────────────────────────────────────
  async function loadUsers() {
    const errEl = document.getElementById('users-error');
    errEl.style.display = 'none';
    try {
      const res = await apiFetch('/admin/users');
      if (res.status === 401) {
        document.getElementById('admin-section').style.display = 'none';
        document.getElementById('login-section').style.display = 'flex';
        showAdminUserLoginBtn(true);
        return;
      }
      if (!res.ok) throw new Error('Failed to load users');
      renderUsers((await res.json()) || []);
    } catch (e) {
      errEl.textContent = e.message;
      errEl.style.display = 'block';
    }
  }

  function renderUsers(users) {
    const tbody = document.getElementById('users-body');
    if (users.length === 0) {
      tbody.innerHTML = '<tr><td colspan="7" style="color:#8fa4c8;text-align:center;padding:24px">No users registered.</td></tr>';
      return;
    }
    tbody.innerHTML = users.map(u => {
      const date = new Date(u.created_at).toLocaleDateString();
      const badge = u.is_active
        ? '<span class="badge badge-active">Active</span>'
        : '<span class="badge badge-inactive">Inactive</span>';
      const allowKeys = u.allow_server_llm_keys !== false;
      return '<tr>' +
        '<td style="color:#8fa4c8">' + u.id + '</td>' +
        '<td>' + escHtml(u.email) + '</td>' +
        '<td>' + escHtml(u.display_name || '\u2014') + '</td>' +
        '<td>' + badge + '</td>' +
        '<td style="text-align:center"><label class="admin-srv-keys-cell" title="Allow fallback to server API keys when user has none">' +
        '<input type="checkbox" ' + (allowKeys ? 'checked ' : '') + ' onchange="setAllowServerKeys(' + u.id + ', this.checked)">' +
        '</label></td>' +
        '<td style="color:#8fa4c8">' + date + '</td>' +
        '<td><div class="actions">' +
          '<button class="btn btn-info" onclick="openStatsModal(' + u.id + ',\'' + escAttr(u.email) + '\')">' +
            '<i class="fas fa-chart-bar" style="margin-right:4px"></i>Stats</button>' +
          '<button class="btn btn-danger" onclick="openDelModal(' + u.id + ',\'' + escAttr(u.email) + '\')">' +
            '<i class="fas fa-trash" style="margin-right:4px"></i>Delete</button>' +
        '</div></td>' +
      '</tr>';
    }).join('');
    populateLLMUserSelect(users);
    populateErrUserSelect(users);
  }

  function populateLLMUserSelect(users) {
    const sel = document.getElementById('llm-user-select');
    if (!sel) return;
    const cur = sel.value;
    sel.innerHTML = '<option value="">— Select user —</option>' +
      users.map(function(u) {
        return '<option value="' + u.id + '">' + escHtml(u.email) + ' (#' + u.id + ')</option>';
      }).join('');
    if (cur) { sel.value = cur; }
  }

  function populateErrUserSelect(users) {
    const sel = document.getElementById('err-user-select');
    if (!sel) return;
    const cur = sel.value;
    sel.innerHTML = '<option value="all">All users</option>' +
      users.map(function(u) {
        return '<option value="' + u.id + '">' + escHtml(u.email) + ' (#' + u.id + ')</option>';
      }).join('');
    if (cur === 'all' || users.some(function(u) { return String(u.id) === cur; })) {
      sel.value = cur;
    } else {
      sel.value = 'all';
    }
  }

  function buildLLMQuery() {
    const p = new URLSearchParams();
    const f = document.getElementById('llm-from').value;
    const t = document.getElementById('llm-to').value;
    if (f) { p.set('from', new Date(f).toISOString()); }
    if (t) { p.set('to', new Date(t).toISOString()); }
    return p.toString();
  }

  function buildErrQuery() {
    const p = new URLSearchParams();
    const f = document.getElementById('err-from').value;
    const t = document.getElementById('err-to').value;
    if (f) { p.set('from', new Date(f).toISOString()); }
    if (t) { p.set('to', new Date(t).toISOString()); }
    const uid = document.getElementById('err-user-select').value;
    if (uid && uid !== 'all') { p.set('user_id', uid); }
    p.set('limit', '200');
    return p.toString();
  }

  async function loadLLMErrors() {
    const errEl = document.getElementById('err-panel-error');
    const sumEl = document.getElementById('err-summary');
    errEl.style.display = 'none';
    sumEl.style.display = 'none';
    sumEl.textContent = '';
    const q = buildErrQuery();
    const fmt = function(n) { return n == null ? '0' : Number(n).toLocaleString(); };
    try {
      const res = await apiFetch('/admin/llm-usage/error-events?' + q);
      if (res.status === 401) {
        document.getElementById('admin-section').style.display = 'none';
        document.getElementById('login-section').style.display = 'flex';
        showAdminUserLoginBtn(true);
        return;
      }
      if (!res.ok) throw new Error('Failed to load error events');
      const data = await res.json();
      const evs = data.events || [];
      const tb = document.getElementById('err-events-table');
      const tbody = document.getElementById('err-events-body');
      sumEl.style.display = 'block';
      sumEl.textContent = 'Showing ' + evs.length + ' row(s), newest first (limit 200).';
      if (evs.length === 0) {
        tb.style.display = 'none';
        tbody.innerHTML = '';
        sumEl.textContent = 'No failed LLM calls in this range.';
        return;
      }
      tb.style.display = 'table';
      tbody.innerHTML = evs.map(function(e) {
        const dt = e.created_at ? new Date(e.created_at).toISOString().replace('T', ' ').slice(0, 19) + ' UTC' : '\u2014';
        const vis = e.is_visitor ? 'Yes' : 'No';
        const mod = e.model_name ? escHtml(e.model_name) : '\u2014';
        const em = e.user_email ? escHtml(e.user_email) : '\u2014';
        const fn = e.user_first_name ? escHtml(e.user_first_name) : '\u2014';
        const fam = e.user_family_name ? escHtml(e.user_family_name) : '\u2014';
        const uidDisp = (e.user_id != null && e.user_id !== undefined) ? String(e.user_id) : '\u2014';
        var keyLab = '\u2014';
        if (e.used_server_llm_key === true) { keyLab = 'Server'; }
        else if (e.used_server_llm_key === false) { keyLab = 'User'; }
        var errCell = '\u2014';
        if (e.error_message) {
          errCell = '<span class="llm-err-text" style="color:#f87171">' + escHtml(String(e.error_message)) + '</span>';
        }
        return '<tr><td style="color:#8fa4c8">' + dt + '</td><td style="color:#8fa4c8">' + escHtml(uidDisp) + '</td><td>' + em + '</td><td>' + fn + '</td><td>' + fam + '</td><td>' + escHtml(e.provider) + '</td><td>' + vis + '</td><td>' + keyLab + '</td><td>' + fmt(e.input_tokens) + '</td><td>' + fmt(e.output_tokens) + '</td><td class="llm-col-error">' + errCell + '</td><td style="max-width:180px;word-break:break-all">' + mod + '</td></tr>';
      }).join('');
    } catch (e) {
      errEl.textContent = e.message || 'Failed to load.';
      errEl.style.display = 'block';
    }
  }

  document.getElementById('llm-load-btn').addEventListener('click', loadLLMUsage);
  document.getElementById('err-load-btn').addEventListener('click', loadLLMErrors);

  document.getElementById('llm-pdf-btn').addEventListener('click', function downloadLLMUsagePDF() {
    const uid = document.getElementById('llm-user-select').value;
    const errEl = document.getElementById('llm-error');
    errEl.style.display = 'none';
    if (!uid) {
      errEl.textContent = 'Select a user.';
      errEl.style.display = 'block';
      return;
    }
    const q = buildLLMQuery();
    window.location.href = '/admin/llm-usage/users/' + uid + '/bill.pdf' + (q ? '?' + q : '');
  });

  async function loadLLMUsage() {
    const uid = document.getElementById('llm-user-select').value;
    const errEl = document.getElementById('llm-error');
    errEl.style.display = 'none';
    if (!uid) {
      errEl.textContent = 'Select a user.';
      errEl.style.display = 'block';
      return;
    }
    const q = buildLLMQuery();
    const base = '/admin/llm-usage/users/' + uid + '/';
    try {
      const [resS, resE, resT] = await Promise.all([
        apiFetch(base + 'summary?' + q),
        apiFetch(base + 'events?' + q + '&limit=100'),
        apiFetch(base + 'timeseries?' + q)
      ]);
      if (resS.status === 401 || resE.status === 401 || resT.status === 401) {
        document.getElementById('admin-section').style.display = 'none';
        document.getElementById('login-section').style.display = 'flex';
        showAdminUserLoginBtn(true);
        return;
      }
      if (!resS.ok || !resE.ok || !resT.ok) {
        throw new Error('Failed to load LLM usage');
      }
      const summary = await resS.json();
      const evData = await resE.json();
      const tsData = await resT.json();
      const sumEl = document.getElementById('llm-summary');
      const fmt = function(n) { return n == null ? '0' : Number(n).toLocaleString(); };
      sumEl.style.display = 'grid';
      sumEl.innerHTML =
        '<div class="stat-card"><div class="label">Total input tokens</div><div class="value">' + fmt(summary.total_input_tokens) + '</div></div>' +
        '<div class="stat-card"><div class="label">Total output tokens</div><div class="value">' + fmt(summary.total_output_tokens) + '</div></div>' +
        '<div class="stat-card"><div class="label">API calls</div><div class="value">' + fmt(summary.event_count) + '</div></div>';
      if (summary.by_provider && summary.by_provider.length) {
        sumEl.innerHTML += '<div style="grid-column:1/-1" class="stat-section">By provider</div>';
        summary.by_provider.forEach(function(p) {
          sumEl.innerHTML += '<div class="stat-card"><div class="label">' + escHtml(p.provider) + '</div><div class="value" style="font-size:0.95rem">in ' + fmt(p.input_tokens) + ' / out ' + fmt(p.output_tokens) + '</div></div>';
        });
      }
      if (summary.by_visitor && summary.by_visitor.length) {
        sumEl.innerHTML += '<div style="grid-column:1/-1" class="stat-section">By session</div>';
        summary.by_visitor.forEach(function(v) {
          const lab = v.is_visitor ? 'Visitor session' : 'Owner session';
          sumEl.innerHTML += '<div class="stat-card"><div class="label">' + lab + '</div><div class="value" style="font-size:0.95rem">in ' + fmt(v.input_tokens) + ' / out ' + fmt(v.output_tokens) + '</div></div>';
        });
      }
      const tb = document.getElementById('llm-events-table');
      const tbody = document.getElementById('llm-events-body');
      const evs = evData.events || [];
      if (evs.length === 0) {
        tb.style.display = 'none';
      } else {
        tb.style.display = 'table';
        tbody.innerHTML = evs.map(function(e) {
          const dt = e.created_at ? new Date(e.created_at).toISOString().replace('T', ' ').slice(0, 19) + ' UTC' : '\u2014';
          const vis = e.is_visitor ? 'Yes' : 'No';
          const mod = e.model_name ? escHtml(e.model_name) : '\u2014';
          const em = e.user_email ? escHtml(e.user_email) : '\u2014';
          const fn = e.user_first_name ? escHtml(e.user_first_name) : '\u2014';
          const fam = e.user_family_name ? escHtml(e.user_family_name) : '\u2014';
          var keyLab = '\u2014';
          if (e.used_server_llm_key === true) { keyLab = 'Server'; }
          else if (e.used_server_llm_key === false) { keyLab = 'User'; }
          var okLab = (e.succeeded === false) ? 'No' : 'Yes';
          var errCell = '\u2014';
          if (e.error_message) {
            errCell = '<span class="llm-err-text" style="color:#f87171">' + escHtml(String(e.error_message)) + '</span>';
          }
          return '<tr><td style="color:#8fa4c8">' + dt + '</td><td>' + em + '</td><td>' + fn + '</td><td>' + fam + '</td><td>' + escHtml(e.provider) + '</td><td>' + vis + '</td><td>' + keyLab + '</td><td>' + fmt(e.input_tokens) + '</td><td>' + fmt(e.output_tokens) + '</td><td>' + okLab + '</td><td class="llm-col-error">' + errCell + '</td><td style="max-width:180px;word-break:break-all">' + mod + '</td></tr>';
        }).join('');
      }
      drawLlmChart(tsData.buckets || []);
    } catch (e) {
      errEl.textContent = e.message || 'Failed to load.';
      errEl.style.display = 'block';
    }
  }

  function drawLlmChart(buckets) {
    const canvas = document.getElementById('llm-usage-canvas');
    if (!canvas || !canvas.getContext) return;
    const ctx = canvas.getContext('2d');
    const cssW = canvas.clientWidth || 800;
    const cssH = 240;
    const dpr = window.devicePixelRatio || 1;
    canvas.width = Math.floor(cssW * dpr);
    canvas.height = Math.floor(cssH * dpr);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.fillStyle = '#0f1922';
    ctx.fillRect(0, 0, cssW, cssH);
    if (!buckets || buckets.length === 0) {
      ctx.fillStyle = '#8fa4c8';
      ctx.font = '14px sans-serif';
      ctx.fillText('No data in range', 12, cssH / 2);
      return;
    }
    var maxY = 0;
    buckets.forEach(function(b) {
      var inp = Number(b.input_tokens) || 0;
      var out = Number(b.output_tokens) || 0;
      if (inp + out > maxY) maxY = inp + out;
    });
    if (maxY < 1) maxY = 1;
    var padL = 44, padR = 12, padT = 12, padB = 28;
    var plotW = cssW - padL - padR;
    var plotH = cssH - padT - padB;
    var n = buckets.length;
    var barW = Math.max(2, plotW / n - 2);
    var yBase = padT + plotH;
    buckets.forEach(function(b, i) {
      var inp = Number(b.input_tokens) || 0;
      var out = Number(b.output_tokens) || 0;
      var x = padL + (i / n) * plotW + (plotW / n - barW) / 2;
      var hIn = (inp / maxY) * plotH;
      var hOut = (out / maxY) * plotH;
      ctx.fillStyle = '#3b82f6';
      ctx.fillRect(x, yBase - hIn, barW, hIn);
      ctx.fillStyle = '#22c55e';
      ctx.fillRect(x, yBase - hIn - hOut, barW, hOut);
    });
    ctx.strokeStyle = '#2a3a6b';
    ctx.beginPath();
    ctx.moveTo(padL, padT);
    ctx.lineTo(padL, yBase);
    ctx.lineTo(padL + plotW, yBase);
    ctx.stroke();
    ctx.fillStyle = '#8fa4c8';
    ctx.font = '11px sans-serif';
    ctx.fillText('0', 8, yBase);
    ctx.fillText(String(maxY), 4, padT + 10);
  }

  // ── Add user modal ─────────────────────────────────────────────────────────
  document.getElementById('add-user-btn').addEventListener('click', function() {
    document.getElementById('add-email').value = '';
    document.getElementById('add-display-name').value = '';
    document.getElementById('add-family-name').value = '';
    document.getElementById('add-gender').value = 'Male';
    document.getElementById('add-password').value = '';
    document.getElementById('add-password-confirm').value = '';
    document.getElementById('add-error').style.display = 'none';
    document.getElementById('add-modal').classList.add('open');
    document.getElementById('add-email').focus();
  });

  document.getElementById('add-cancel').addEventListener('click', function() {
    document.getElementById('add-modal').classList.remove('open');
  });

  document.getElementById('add-save').addEventListener('click', async function() {
    const btn = this;
    const errEl = document.getElementById('add-error');
    errEl.style.display = 'none';
    const email = document.getElementById('add-email').value.trim();
    const displayName = document.getElementById('add-display-name').value.trim();
    const familyName = document.getElementById('add-family-name').value.trim();
    const gender = document.getElementById('add-gender').value;
    const password = document.getElementById('add-password').value;
    const confirm = document.getElementById('add-password-confirm').value;
    if (!email || !displayName || !password) {
      errEl.textContent = 'Email, first name, and password are required.';
      errEl.style.display = 'block';
      return;
    }
    if (password !== confirm) {
      errEl.textContent = 'Passwords do not match.';
      errEl.style.display = 'block';
      return;
    }
    if (password.length < 12) {
      errEl.textContent = 'Password must be at least 12 characters.';
      errEl.style.display = 'block';
      return;
    }
    btn.disabled = true;
    btn.innerHTML = '<i class="fas fa-spinner fa-spin" style="margin-right:6px"></i>Creating\u2026';
    try {
      const res = await apiFetch('/admin/users', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, display_name: displayName, family_name: familyName, gender, password })
      });
      if (res.ok) {
        document.getElementById('add-modal').classList.remove('open');
        loadUsers();
      } else {
        const d = await res.json().catch(() => ({}));
        errEl.textContent = apiErrorMessage(d, 'Failed to create user.');
        errEl.style.display = 'block';
      }
    } catch (e) {
      errEl.textContent = 'Network error.';
      errEl.style.display = 'block';
    } finally {
      btn.disabled = false;
      btn.innerHTML = '<i class="fas fa-user-plus" style="margin-right:6px"></i>Create User';
    }
  });

  // ── Stats modal ────────────────────────────────────────────────────────────
  window.openStatsModal = async function(id, email) {
    document.getElementById('stats-modal-title').innerHTML =
      '<i class="fas fa-chart-bar" style="color:#3b82f6;margin-right:8px"></i>Archive Statistics &mdash; ' + escHtml(email);
    document.getElementById('stats-body').innerHTML =
      '<p style="color:#8fa4c8;text-align:center;padding:24px"><i class="fas fa-spinner fa-spin"></i> Loading\u2026</p>';
    document.getElementById('stats-modal').classList.add('open');
    try {
      const res = await apiFetch('/admin/users/' + id + '/dashboard');
      if (!res.ok) throw new Error('Failed to load stats');
      const d = await res.json();
      document.getElementById('stats-body').innerHTML = renderStats(d);
    } catch (e) {
      document.getElementById('stats-body').innerHTML =
        '<p style="color:#f87171;text-align:center;padding:24px">' + escHtml(e.message) + '</p>';
    }
  };

  document.getElementById('stats-close').addEventListener('click', function() {
    document.getElementById('stats-modal').classList.remove('open');
  });
  document.getElementById('stats-close-top').addEventListener('click', function() {
    document.getElementById('stats-modal').classList.remove('open');
  });

  function statCard(label, value) {
    return '<div class="stat-card"><div class="label">' + escHtml(label) +
      '</div><div class="value">' + escHtml(String(value)) + '</div></div>';
  }

  function renderStats(d) {
    const fmt = n => (n == null ? '0' : Number(n).toLocaleString());
    let html = '';

    html += '<div class="stat-section">Overview</div><div class="stat-grid">';
    html += statCard('Subject', d.subject_full_name || '\u2014');
    html += statCard('Emails', fmt(d.emails_count));
    html += statCard('Messages', fmt(d.total_messages));
    html += statCard('Contacts', fmt(d.contacts_count));
    html += statCard('Images', fmt(d.total_images));
    html += statCard('Artefacts', fmt(d.artefacts_count));
    html += '</div>';

    html += '<div class="stat-section">Media</div><div class="stat-grid">';
    html += statCard('Imported Images', fmt(d.imported_images));
    html += statCard('Reference Images', fmt(d.reference_images));
    html += statCard('Thumbnails', fmt(d.thumbnail_count) + ' (' + (d.thumbnail_percentage || 0).toFixed(1) + '%)');
    html += statCard('Facebook Albums', fmt(d.facebook_albums_count));
    html += statCard('Facebook Posts', fmt(d.facebook_posts_count));
    html += '</div>';

    html += '<div class="stat-section">Places &amp; Documents</div><div class="stat-grid">';
    html += statCard('Locations', fmt(d.locations_count));
    html += statCard('Places', fmt(d.places_count));
    html += statCard('Reference Docs', fmt(d.reference_docs_count));
    html += statCard('Docs Enabled', fmt(d.reference_docs_enabled));
    html += statCard('Complete Profiles', fmt(d.complete_profiles_count));
    html += '</div>';

    if (d.message_counts && Object.keys(d.message_counts).length) {
      html += '<div class="stat-section">Messages by Type</div><div class="stat-grid">';
      for (const [k, v] of Object.entries(d.message_counts)) {
        html += statCard(k, fmt(v));
      }
      html += '</div>';
    }

    if (d.messages_by_contact && d.messages_by_contact.length) {
      html += '<div class="stat-section">Top Contacts</div>';
      html += '<table style="margin-bottom:8px"><thead><tr><th>Name</th><th>Messages</th></tr></thead><tbody>';
      d.messages_by_contact.slice(0, 10).forEach(c => {
        html += '<tr><td>' + escHtml(c.name) + '</td><td>' + fmt(c.count) + '</td></tr>';
      });
      html += '</tbody></table>';
    }

    return html;
  }

  // ── Delete modal ───────────────────────────────────────────────────────────
  window.openDelModal = function(id, email) {
    pendingDelUserId = id;
    document.getElementById('del-modal-label').textContent = 'Delete user: ' + email;
    document.getElementById('del-modal').classList.add('open');
  };

  document.getElementById('del-cancel').addEventListener('click', function() {
    document.getElementById('del-modal').classList.remove('open');
  });

  document.getElementById('del-confirm').addEventListener('click', async function() {
    const btn = this;
    btn.disabled = true; btn.textContent = 'Deleting\u2026';
    try {
      const res = await apiFetch('/admin/users/' + pendingDelUserId, { method: 'DELETE' });
      document.getElementById('del-modal').classList.remove('open');
      if (res.ok) { loadUsers(); }
      else {
        const d = await res.json().catch(() => ({}));
        document.getElementById('users-error').textContent = apiErrorMessage(d, 'Delete failed.');
        document.getElementById('users-error').style.display = 'block';
      }
    } catch (e) { document.getElementById('del-modal').classList.remove('open'); }
    finally { btn.disabled = false; btn.textContent = 'Delete'; }
  });

  // ── Helpers ────────────────────────────────────────────────────────────────
  function escHtml(s) {
    return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
  }
  function escAttr(s) {
    return String(s).replace(/'/g,"\\'");
  }

  // ── Tabs: Users | Usage & Billing | Errors | System instructions | Pam Bot | Archives ─
  (function initAdminTabs() {
    var btnUsers = document.getElementById('tab-btn-users');
    var btnBilling = document.getElementById('tab-btn-billing');
    var btnErrors = document.getElementById('tab-btn-errors');
    var btnSys = document.getElementById('tab-btn-sys');
    var btnPambot = document.getElementById('tab-btn-pambot');
    var btnArchives = document.getElementById('tab-btn-archives');
    var panelUsers = document.getElementById('tab-panel-users');
    var panelBilling = document.getElementById('tab-panel-billing');
    var panelErrors = document.getElementById('tab-panel-errors');
    var panelSys = document.getElementById('tab-panel-sys');
    var panelPambot = document.getElementById('tab-panel-pambot');
    var panelArchives = document.getElementById('tab-panel-archives');
    if (!btnUsers || !btnBilling || !btnErrors || !btnSys || !btnPambot || !btnArchives ||
        !panelUsers || !panelBilling || !panelErrors || !panelSys || !panelPambot || !panelArchives) return;

    async function loadSystemInstructions() {
      var errEl = document.getElementById('sys-instr-error');
      var okEl = document.getElementById('sys-instr-ok');
      if (okEl) okEl.style.display = 'none';
      if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
      try {
        var res = await apiFetch('/admin/system-instructions');
        var d = await res.json().catch(function() { return {}; });
        if (!res.ok) throw new Error(apiErrorMessage(d, 'Failed to load'));
        var c = document.getElementById('sys-core');
        var ch = document.getElementById('sys-chat');
        var q = document.getElementById('sys-question');
        if (c) c.value = d.core_system_instructions || '';
        if (ch) ch.value = d.system_instructions || '';
        if (q) q.value = d.question_system_instructions || '';
      } catch (e) {
        if (errEl) {
          errEl.textContent = e.message || 'Network error';
          errEl.style.display = 'block';
        }
      }
    }

    async function loadPambotInstructions() {
      var errEl = document.getElementById('pambot-instr-error');
      var okEl = document.getElementById('pambot-instr-ok');
      if (okEl) okEl.style.display = 'none';
      if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
      try {
        var res = await apiFetch('/admin/pambot-instructions');
        var d = await res.json().catch(function() { return {}; });
        if (!res.ok) throw new Error(apiErrorMessage(d, 'Failed to load'));
        var ta = document.getElementById('pambot-instructions');
        if (ta) ta.value = d.pam_bot_instructions || '';
      } catch (e) {
        if (errEl) { errEl.textContent = e.message || 'Network error'; errEl.style.display = 'block'; }
      }
    }

    function setTab(which) {
      var isUsers = which === 'users';
      var isBilling = which === 'billing';
      var isErrors = which === 'errors';
      var isSys = which === 'sys';
      var isPambot = which === 'pambot';
      var isArchives = which === 'archives';
      btnUsers.classList.toggle('active', isUsers);
      btnBilling.classList.toggle('active', isBilling);
      btnErrors.classList.toggle('active', isErrors);
      btnSys.classList.toggle('active', isSys);
      btnPambot.classList.toggle('active', isPambot);
      btnArchives.classList.toggle('active', isArchives);
      btnUsers.setAttribute('aria-selected', isUsers);
      btnBilling.setAttribute('aria-selected', isBilling);
      btnErrors.setAttribute('aria-selected', isErrors);
      btnSys.setAttribute('aria-selected', isSys);
      btnPambot.setAttribute('aria-selected', isPambot);
      btnArchives.setAttribute('aria-selected', isArchives);
      panelUsers.classList.toggle('active', isUsers);
      panelBilling.classList.toggle('active', isBilling);
      panelErrors.classList.toggle('active', isErrors);
      panelSys.classList.toggle('active', isSys);
      panelPambot.classList.toggle('active', isPambot);
      panelArchives.classList.toggle('active', isArchives);
      if (isErrors) {
        var ef = document.getElementById('err-from');
        var et = document.getElementById('err-to');
        var lf = document.getElementById('llm-from');
        var lt = document.getElementById('llm-to');
        if (ef && et && lf && lt && !ef.value && !et.value) {
          ef.value = lf.value;
          et.value = lt.value;
        }
      }
      if (isSys) loadSystemInstructions();
      if (isPambot) loadPambotInstructions();
      if (isArchives) loadArchives();
    }
    btnUsers.addEventListener('click', function() { setTab('users'); });
    btnBilling.addEventListener('click', function() { setTab('billing'); });
    btnErrors.addEventListener('click', function() { setTab('errors'); });
    btnSys.addEventListener('click', function() { setTab('sys'); });
    btnPambot.addEventListener('click', function() { setTab('pambot'); });
    btnArchives.addEventListener('click', function() { setTab('archives'); });

    var sysReload = document.getElementById('sys-reload-btn');
    var sysSave = document.getElementById('sys-save-btn');
    if (sysReload) sysReload.addEventListener('click', function() { loadSystemInstructions(); });
    if (sysSave) sysSave.addEventListener('click', async function() {
      var btn = this;
      var errEl = document.getElementById('sys-instr-error');
      var okEl = document.getElementById('sys-instr-ok');
      if (okEl) okEl.style.display = 'none';
      if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
      btn.disabled = true;
      try {
        var res = await apiFetch('/admin/system-instructions', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            core_system_instructions: document.getElementById('sys-core').value,
            system_instructions: document.getElementById('sys-chat').value,
            question_system_instructions: document.getElementById('sys-question').value
          })
        });
        if (res.ok) {
          if (okEl) { okEl.style.display = 'block'; setTimeout(function() { if (okEl) okEl.style.display = 'none'; }, 4000); }
        } else {
          var d = await res.json().catch(function() { return {}; });
          if (errEl) {
            errEl.textContent = apiErrorMessage(d, 'Save failed');
            errEl.style.display = 'block';
          }
        }
      } catch (e) {
        if (errEl) {
          errEl.textContent = 'Network error';
          errEl.style.display = 'block';
        }
      } finally {
        btn.disabled = false;
      }
    });

    async function loadArchives() {
      var errEl = document.getElementById('archives-error');
      errEl.style.display = 'none';
      try {
        var res = await apiFetch('/admin/profiles');
        if (res.status === 401) { showAdminLogin(); return; }
        if (!res.ok) {
          var ed = await res.json().catch(function() { return {}; });
          throw new Error(apiErrorMessage(ed, 'Failed to load archives'));
        }
        var data = await res.json();
        renderArchives(data.profiles || data || []);
      } catch(e) { errEl.textContent = e.message; errEl.style.display = 'block'; }
    }

    var archiveAddBtn = document.getElementById('archive-add-btn');
    var archiveBrowseBtn = document.getElementById('archive-add-browse-btn');
    if (archiveBrowseBtn) archiveBrowseBtn.addEventListener('click', async function() {
      var errEl = document.getElementById('archives-error');
      var dbPathEl = document.getElementById('archive-add-db-path');
      errEl.style.display = 'none';
      errEl.textContent = '';
      if (!window.electronAPI || !window.electronAPI.showSaveDialog) {
        errEl.textContent = 'Browse is available in the desktop app only.';
        errEl.style.display = 'block';
        return;
      }
      try {
        var defaultPath = 'archive.sqlite';
        if (window.electronAPI && window.electronAPI.getAdminDataDir) {
          var dirRes = await window.electronAPI.getAdminDataDir();
          if (dirRes && dirRes.ok && dirRes.dir) {
            defaultPath = dirRes.dir.replace(/[\\/]+$/, '') + '/archive.sqlite';
          }
        }
        var res = await window.electronAPI.showSaveDialog({
          title: 'Save new archive database as',
          defaultPath: defaultPath,
          filters: [
            { name: 'SQLite Databases', extensions: ['sqlite', 'db'] },
            { name: 'All Files', extensions: ['*'] }
          ]
        });
        if (res && !res.canceled && res.filePath) {
          dbPathEl.value = res.filePath;
        }
      } catch(e) {
        errEl.textContent = 'Failed to open file picker.';
        errEl.style.display = 'block';
      }
    });
    if (archiveAddBtn) archiveAddBtn.addEventListener('click', async function() {
      var errEl = document.getElementById('archives-error');
      errEl.style.display = 'none';
      errEl.textContent = '';
      var firstEl = document.getElementById('archive-add-first-name');
      var familyEl = document.getElementById('archive-add-family-name');
      var usernameEl = document.getElementById('archive-add-username');
      var passwordEl = document.getElementById('archive-add-password');
      var confirmEl = document.getElementById('archive-add-password-confirm');
      var dbPathEl = document.getElementById('archive-add-db-path');
      var enabledEl = document.getElementById('archive-add-enabled');
      var displayName = (firstEl.value || '').trim();
      var familyName = (familyEl.value || '').trim();
      var username = (usernameEl.value || '').trim();
      var password = passwordEl.value;
      var confirm = confirmEl.value;
      var dbPath = (dbPathEl.value || '').trim();
      if (!displayName || !familyName || !username || !password) {
        errEl.textContent = 'First name, family name, username, and password are required.';
        errEl.style.display = 'block';
        return;
      }
      if (!dbPath && window.electronAPI && window.electronAPI.suggestArchiveDbPath) {
        var sug = await window.electronAPI.suggestArchiveDbPath(displayName + ' ' + familyName);
        if (sug && sug.ok && sug.dbPath) dbPath = sug.dbPath;
      }
      if (!dbPath) {
        errEl.textContent = 'DB path is required (leave blank in the desktop app to use the default next to the admin database).';
        errEl.style.display = 'block';
        return;
      }
      if (password.length < 12) {
        errEl.textContent = 'Password must be at least 12 characters.';
        errEl.style.display = 'block';
        return;
      }
      if (password !== confirm) {
        errEl.textContent = 'Passwords do not match.';
        errEl.style.display = 'block';
        return;
      }
      archiveAddBtn.disabled = true;
      var oldHtml = archiveAddBtn.innerHTML;
      archiveAddBtn.innerHTML = '<i class="fas fa-spinner fa-spin" style="margin-right:6px"></i>Creating…';
      try {
        var payload = {
          display_name: displayName,
          family_name: familyName,
          username: username,
          password: password,
          password_confirm: confirm,
          db_path: dbPath,
          enabled: !!enabledEl.checked
        };
        var res = await apiFetch('/admin/profiles', {
          method: 'POST',
          headers: {'Content-Type':'application/json'},
          body: JSON.stringify(payload)
        });
        if (res.status === 401) { showAdminLogin(); return; }
        if (!res.ok) {
          var d = await res.json().catch(function(){return{};});
          throw new Error(apiErrorMessage(d, 'Failed to create archive'));
        }
        firstEl.value = '';
        familyEl.value = '';
        usernameEl.value = '';
        passwordEl.type = 'password';
        confirmEl.type = 'password';
        passwordEl.value = '';
        confirmEl.value = '';
        ['archive-add-password-toggle','archive-add-password-confirm-toggle'].forEach(function(tid) {
          var b = document.getElementById(tid);
          if (!b) return;
          b.setAttribute('aria-pressed', 'false');
          b.setAttribute('aria-label', 'Show password');
          b.title = 'Show password';
          var ic = b.querySelector('i');
          if (ic) ic.className = 'far fa-eye';
        });
        dbPathEl.value = '';
        enabledEl.checked = true;
        loadArchives();
      } catch(e) {
        errEl.textContent = e && e.message ? e.message : 'Network error.';
        errEl.style.display = 'block';
      } finally {
        archiveAddBtn.disabled = false;
        archiveAddBtn.innerHTML = oldHtml;
      }
    });

    function wireAdminArchivePwToggle(toggleId, inputId) {
      var btn = document.getElementById(toggleId);
      var inp = document.getElementById(inputId);
      if (!btn || !inp) return;
      btn.addEventListener('click', function() {
        var show = inp.type === 'password';
        inp.type = show ? 'text' : 'password';
        btn.setAttribute('aria-pressed', show ? 'true' : 'false');
        btn.setAttribute('aria-label', show ? 'Hide password' : 'Show password');
        btn.title = show ? 'Hide password' : 'Show password';
        var i = btn.querySelector('i');
        if (i) { i.className = show ? 'far fa-eye-slash' : 'far fa-eye'; }
      });
    }
    wireAdminArchivePwToggle('archive-add-password-toggle', 'archive-add-password');
    wireAdminArchivePwToggle('archive-add-password-confirm-toggle', 'archive-add-password-confirm');

    function renderArchives(profiles) {
      var tbody = document.getElementById('archives-body');
      if (!profiles.length) {
        tbody.innerHTML = '<tr><td colspan="7" style="color:#8fa4c8;text-align:center;padding:24px">No archive profiles.</td></tr>';
        return;
      }
      tbody.innerHTML = profiles.map(function(p) {
        var lu = p.last_used ? new Date(p.last_used).toLocaleString() : '—';
        var badge = p.enabled
          ? '<span class="badge badge-active">Enabled</span>'
          : '<span class="badge badge-inactive">Disabled</span>';
        var defCell = p.is_default
          ? '<span class="badge badge-active" title="Opened at server startup when the file exists">Default</span>'
          : '<span style="color:#6b7c99">—</span>';
        var actionDefault = '';
        if (p.is_default) {
          actionDefault = '<button type="button" class="btn-compact-secondary" style="margin-right:6px;margin-bottom:4px" onclick="clearStartupDefaultArchive(\'' + escAttr(p.id) + '\')">Clear default</button>';
        } else if (p.enabled) {
          actionDefault = '<button type="button" class="btn-compact-secondary" style="margin-right:6px;margin-bottom:4px" onclick="setStartupDefaultArchive(\'' + escAttr(p.id) + '\')">Set Default</button>';
        } else {
          actionDefault = '<button type="button" class="btn-compact-secondary" style="margin-right:6px;margin-bottom:4px" disabled title="Enable this archive first">Set Default</button>';
        }
        return '<tr>' +
          '<td>' + escHtml(p.name) + '</td>' +
          '<td>' + escHtml(p.username || '—') + '</td>' +
          '<td style="font-family:monospace;font-size:.82rem;word-break:break-all">' + escHtml(p.db_path) + '</td>' +
          '<td>' + badge + '</td>' +
          '<td style="vertical-align:middle">' + defCell + '</td>' +
          '<td style="color:#8fa4c8">' + lu + '</td>' +
          '<td style="white-space:normal;max-width:320px">' + actionDefault +
            '<button class="btn btn-warning" style="margin-right:6px;margin-bottom:4px" onclick="toggleArchive(\'' +
              escAttr(p.id) + '\',' + (!p.enabled) + ')">' +
              (p.enabled ? 'Disable' : 'Enable') + '</button>' +
            '<button class="btn btn-danger" style="margin-bottom:4px" onclick="openDelArchiveModal(\'' +
              escAttr(p.id) + '\',\'' + escAttr(p.name) + '\',\'' + escAttr(p.db_path) + '\')">Delete</button>' +
          '</td>' +
          '</tr>';
      }).join('');
    }

    function showAdminLogin() {
      document.getElementById('admin-section').style.display = 'none';
      document.getElementById('login-section').style.display = 'flex';
      showAdminUserLoginBtn(true);
    }

    window.toggleArchive = async function(id, enable) {
      try {
        var res = await apiFetch('/admin/profiles/' + id, {
          method: 'PATCH',
          headers: {'Content-Type':'application/json'},
          body: JSON.stringify({enabled: enable})
        });
        if (res.status === 401) { showAdminLogin(); return; }
        if (!res.ok) { var d = await res.json().catch(function(){return{};}); alert(apiErrorMessage(d, 'Failed')); }
        loadArchives();
      } catch(e) { alert('Network error.'); }
    };

    window.setStartupDefaultArchive = async function(id) {
      try {
        var res = await apiFetch('/admin/profiles/' + id, {
          method: 'PATCH',
          headers: {'Content-Type':'application/json'},
          body: JSON.stringify({is_default: true})
        });
        if (res.status === 401) { showAdminLogin(); return; }
        if (!res.ok) {
          var d = await res.json().catch(function(){return{};});
          alert(apiErrorMessage(d, 'Failed to set startup default'));
          return;
        }
        loadArchives();
      } catch(e) { alert('Network error.'); }
    };

    window.clearStartupDefaultArchive = async function(id) {
      try {
        var res = await apiFetch('/admin/profiles/' + id, {
          method: 'PATCH',
          headers: {'Content-Type':'application/json'},
          body: JSON.stringify({is_default: false})
        });
        if (res.status === 401) { showAdminLogin(); return; }
        if (!res.ok) {
          var d = await res.json().catch(function(){return{};});
          alert(apiErrorMessage(d, 'Failed to clear startup default'));
          return;
        }
        loadArchives();
      } catch(e) { alert('Network error.'); }
    };

    // ── Delete archive modal ───────────────────────────────────────────────
    var pendingDelArchiveId = null;
    window.openDelArchiveModal = function(id, name, dbPath) {
      pendingDelArchiveId = id;
      document.getElementById('del-archive-modal-label').textContent = 'Delete archive: ' + name;
      document.getElementById('del-archive-modal-path').textContent = dbPath || '(unknown)';
      document.getElementById('del-archive-file').checked = false;
      var errEl = document.getElementById('del-archive-error');
      errEl.style.display = 'none';
      errEl.textContent = '';
      document.getElementById('del-archive-modal').classList.add('open');
    };
    var delArchiveCancel = document.getElementById('del-archive-cancel');
    if (delArchiveCancel) delArchiveCancel.addEventListener('click', function() {
      document.getElementById('del-archive-modal').classList.remove('open');
      pendingDelArchiveId = null;
    });
    var delArchiveConfirm = document.getElementById('del-archive-confirm');
    if (delArchiveConfirm) delArchiveConfirm.addEventListener('click', async function() {
      if (!pendingDelArchiveId) return;
      var btn = this;
      var errEl = document.getElementById('del-archive-error');
      var deleteFile = document.getElementById('del-archive-file').checked;
      btn.disabled = true; btn.textContent = 'Deleting\u2026';
      errEl.style.display = 'none'; errEl.textContent = '';
      try {
        var url = '/admin/profiles/' + pendingDelArchiveId + (deleteFile ? '?delete_file=true' : '');
        var res = await apiFetch(url, { method: 'DELETE' });
        if (res.status === 401) { showAdminLogin(); return; }
        var d = await res.json().catch(function(){return{};});
        if (!res.ok) {
          errEl.textContent = apiErrorMessage(d, 'Failed to delete archive.');
          errEl.style.display = 'block';
          return;
        }
        document.getElementById('del-archive-modal').classList.remove('open');
        pendingDelArchiveId = null;
        if (d.file_warning) {
          var aErr = document.getElementById('archives-error');
          aErr.textContent = d.file_warning;
          aErr.style.display = 'block';
        }
        loadArchives();
      } catch(e) {
        errEl.textContent = 'Network error.';
        errEl.style.display = 'block';
      } finally {
        btn.disabled = false; btn.textContent = 'Delete';
      }
    });

    var pambotReload = document.getElementById('pambot-reload-btn');
    var pambotSave = document.getElementById('pambot-save-btn');
    if (pambotReload) pambotReload.addEventListener('click', function() { loadPambotInstructions(); });
    if (pambotSave) pambotSave.addEventListener('click', async function() {
      var btn = this;
      var errEl = document.getElementById('pambot-instr-error');
      var okEl = document.getElementById('pambot-instr-ok');
      if (okEl) okEl.style.display = 'none';
      if (errEl) { errEl.style.display = 'none'; errEl.textContent = ''; }
      btn.disabled = true;
      try {
        var res = await apiFetch('/admin/pambot-instructions', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ pam_bot_instructions: document.getElementById('pambot-instructions').value })
        });
        if (res.ok) {
          if (okEl) { okEl.style.display = 'block'; setTimeout(function() { if (okEl) okEl.style.display = 'none'; }, 4000); }
        } else {
          var d = await res.json().catch(function() { return {}; });
          if (errEl) { errEl.textContent = apiErrorMessage(d, 'Save failed'); errEl.style.display = 'block'; }
        }
      } catch (e) {
        if (errEl) { errEl.textContent = 'Network error'; errEl.style.display = 'block'; }
      } finally {
        btn.disabled = false;
      }
    });
  })();

  // Auto-check if already logged in on page load.
  (async function() {
    const res = await apiFetch('/admin/users');
    if (res.ok) showAdmin();
  })();
})();
</script>
</body>
</html>`
