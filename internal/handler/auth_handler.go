package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// AuthHandler handles user registration, login, logout, profile, and
// password-change endpoints.
type AuthHandler struct {
	svc              *service.AuthService
	sensitiveSvc     *service.SensitiveService
	subjectConfigSvc *service.SubjectConfigService
	sessionStore     *keystore.SessionMasterStore
	secure           bool
	limiter          *authRateLimiter
}

// NewAuthHandler creates an AuthHandler.  secure must match the Secure flag
// used for the session cookie (true when served over HTTPS).
func NewAuthHandler(svc *service.AuthService, sensitiveSvc *service.SensitiveService, subjectConfigSvc *service.SubjectConfigService, sessionStore *keystore.SessionMasterStore, secure bool) *AuthHandler {
	return &AuthHandler{
		svc:              svc,
		sensitiveSvc:     sensitiveSvc,
		subjectConfigSvc: subjectConfigSvc,
		sessionStore:     sessionStore,
		secure:           secure,
		limiter:          newAuthRateLimiter(),
	}
}

// RegisterRoutes mounts /auth/* endpoints.
func (h *AuthHandler) RegisterRoutes(r chi.Router) {
	r.Post("/auth/register", h.Register)
	r.Post("/auth/login", h.Login)
	r.Post("/auth/logout", h.Logout)
	r.Get("/auth/me", h.Me)
	r.Patch("/auth/me/llm-settings", h.PatchLLMSettings)
	r.Post("/auth/change-password", h.ChangePassword)
}

// POST /auth/register
// Body: { "username": "...", "password": "...", "display_name": "...", "family_name": "...", "gender": "..." }
// Returns 201 on success.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if !h.limiter.Allow(realIP(r)) {
		writeError(w, http.StatusTooManyRequests, "too many requests — try again later")
		return
	}

	var req struct {
		Username    string `json:"username"`
		Email       string `json:"email"` // backward-compatible alias
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		FamilyName  string `json:"family_name"`
		Gender      string `json:"gender"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	username := strings.ToLower(strings.TrimSpace(req.Username))
	if username == "" {
		username = strings.ToLower(strings.TrimSpace(req.Email))
	}
	if username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}
	password := strings.ToLower(req.Password)
	if req.DisplayName == "" {
		writeError(w, http.StatusBadRequest, "first name is required")
		return
	}
	if req.Gender == "" {
		req.Gender = "Male"
	}

	user, err := h.svc.Register(r.Context(), username, password, req.DisplayName, req.FamilyName)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrRegistrationClosed):
			writeError(w, http.StatusConflict, err.Error())
		case errors.Is(err, service.ErrUsernameTaken):
			writeError(w, http.StatusConflict, err.Error())
		case errors.Is(err, service.ErrWeakPassword):
			writeError(w, http.StatusUnprocessableEntity, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "registration failed")
		}
		return
	}

	// Initialise the master keyring using the registration password, then
	// cache the password in RAM so the keyring is unlocked immediately.
	userCtx := context.WithValue(r.Context(), appctx.ContextKeyUserID, user.ID)
	seedPassword := ""
	if err := h.sensitiveSvc.InitKeyring(userCtx, password); err != nil {
		slog.Warn("keyring init on registration failed", "user_id", user.ID, "err", err)
	} else {
		seedPassword = password
		_ = h.sessionStore.Put(w, r, password, true)
	}
	if h.sensitiveSvc != nil {
		if err := h.sensitiveSvc.SeedNewOwnerDefaults(userCtx, seedPassword); err != nil {
			slog.Warn("archive owner defaults seed on registration failed", "user_id", user.ID, "err", err)
		}
	}

	// Create the subject configuration so the user lands directly on the main page.
	familyName := strings.TrimSpace(req.FamilyName)
	gender := req.Gender
	if _, err := h.subjectConfigSvc.CreateOrUpdate(userCtx, service.SubjectConfigUpdateParams{
		SubjectName: strings.TrimSpace(req.DisplayName),
		FamilyName:  &familyName,
		Gender:      &gender,
	}); err != nil {
		slog.Warn("subject config init on registration failed", "user_id", user.ID, "err", err)
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

// POST /auth/login
// Body: { "username": "...", "password": "..." }
// Sets dm_session cookie on success.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if !h.limiter.Allow(realIP(r)) {
		writeError(w, http.StatusTooManyRequests, "too many requests — try again later")
		return
	}

	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"` // backward-compatible alias
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		username = strings.TrimSpace(req.Email)
	}
	if username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}
	rawPassword := req.Password
	password := strings.ToLower(rawPassword)

	sessionID, user, err := h.svc.Login(r.Context(), username, rawPassword)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "invalid username or password")
			return
		}
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}

	h.setSessionCookie(w, sessionID, service.CookieMaxAge)

	// Auto-unlock the master keyring if the login password matches it.
	// For legacy users whose keyring uses a different password this is a no-op;
	// they will still see the manual unlock prompt.
	userCtx := context.WithValue(r.Context(), appctx.ContextKeyUserID, user.ID)
	if ok, _ := h.sensitiveSvc.VerifyMasterPassword(userCtx, password); ok {
		_ = h.sessionStore.Put(w, r, password, true)
	}

	writeJSON(w, map[string]any{
		"id":           user.ID,
		"email":        user.Email,
		"display_name": user.DisplayName,
	})
}

// POST /auth/logout
// Deletes the session and expires the cookie.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	sid := h.readSessionCookie(r)
	if sid != "" {
		_ = h.svc.Logout(r.Context(), sid)
	}
	h.setSessionCookie(w, "", -1)
	w.WriteHeader(http.StatusNoContent)
}

// GET /auth/me
// Returns the authenticated user's profile, or 401.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	auth, err := h.svc.Authenticate(r.Context(), h.readSessionCookie(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "session lookup failed")
		return
	}
	if auth == nil || auth.User == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	user := auth.User
	isVisitor := auth.IsVisitor
	out := map[string]any{
		"id":           user.ID,
		"email":        user.Email,
		"display_name": user.DisplayName,
		"first_name":   user.FirstName,
		"family_name":  user.FamilyName,
		"is_visitor":   isVisitor,
		// Only for signed-in archive owners — never expose owner admin flag to visitor sessions.
		"is_admin": !isVisitor && user.IsAdmin,
	}
	if isVisitor {
		a := auth.Access
		out["visitor_access"] = map[string]any{
			"restricted":            a.Restricted,
			"can_messages_chat":     a.AllowMessagesChat(),
			"can_emails":            a.AllowEmails(),
			"can_contacts":          a.AllowContacts(),
			"can_relationships":     a.AllowRelationships(),
			"can_sensitive_private": a.AllowSensitivePrivate(),
		}
	}
	owner, _ := h.svc.GetUserLLMStored(r.Context(), user.ID)
	if owner == nil {
		owner = &repository.UserLLMStored{}
	}
	if isVisitor {
		sid := h.readSessionCookie(r)
		vis, _ := h.svc.GetSessionVisitorLLM(r.Context(), sid)
		if vis == nil {
			vis = &repository.UserLLMStored{}
		}
		out["llm_settings"] = map[string]any{
			"session_scoped":               true,
			"gemini_api_key_set":           vis.GeminiAPIKey != "",
			"anthropic_api_key_set":        vis.AnthropicAPIKey != "",
			"deepseek_api_key_set":         vis.DeepSeekAPIKey != "",
			"tavily_api_key_set":           vis.TavilyAPIKey != "",
			"runpod_api_key_set":           vis.RunpodAPIKey != "",
			"elevenlabs_api_key_set":       vis.ElevenLabsAPIKey != "",
			"gemini_model":                 vis.GeminiModel,
			"claude_model":                 vis.ClaudeModel,
			"deepseek_model":               vis.DeepSeekModel,
			"runpod_endpoint_id":           vis.RunpodEndpointID,
			"runpod_workers":               vis.RunpodWorkers,
			"subject_gemini_api_key_set":   owner.GeminiAPIKey != "",
			"subject_anthropic_key_set":    owner.AnthropicAPIKey != "",
			"subject_deepseek_key_set":     owner.DeepSeekAPIKey != "",
			"subject_tavily_key_set":       owner.TavilyAPIKey != "",
			"subject_runpod_key_set":       owner.RunpodAPIKey != "",
			"subject_elevenlabs_key_set":   owner.ElevenLabsAPIKey != "",
			"subject_gemini_model":         owner.GeminiModel,
			"subject_claude_model":         owner.ClaudeModel,
			"subject_deepseek_model":       owner.DeepSeekModel,
			"subject_runpod_endpoint_id":   owner.RunpodEndpointID,
			"subject_runpod_workers":       owner.RunpodWorkers,
		}
	} else {
		out["llm_settings"] = map[string]any{
			"session_scoped":         false,
			"gemini_api_key_set":     owner.GeminiAPIKey != "",
			"anthropic_api_key_set":  owner.AnthropicAPIKey != "",
			"deepseek_api_key_set":   owner.DeepSeekAPIKey != "",
			"tavily_api_key_set":     owner.TavilyAPIKey != "",
			"runpod_api_key_set":     owner.RunpodAPIKey != "",
			"elevenlabs_api_key_set": owner.ElevenLabsAPIKey != "",
			"gemini_model":           owner.GeminiModel,
			"claude_model":           owner.ClaudeModel,
			"deepseek_model":         owner.DeepSeekModel,
			"runpod_endpoint_id":     owner.RunpodEndpointID,
			"runpod_workers":         owner.RunpodWorkers,
		}
	}
	writeJSON(w, out)
}

// PatchLLMSettings PATCH /auth/me/llm-settings
// JSON keys are optional. Present keys update overrides; empty string clears that override.
// Owners persist to users; visitors persist to the current session only (sessions.visitor_llm_overrides).
// API keys are never returned from GET /auth/me (only *_set flags).
func (h *AuthHandler) PatchLLMSettings(w http.ResponseWriter, r *http.Request) {
	auth, err := h.svc.Authenticate(r.Context(), h.readSessionCookie(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "session lookup failed")
		return
	}
	if auth == nil || auth.User == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	user := auth.User
	isVisitor := auth.IsVisitor
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read body")
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	patch := repository.UserLLMPatch{}
	if ptr, ok := decodeLLMJSONString(raw, "gemini_api_key"); ok {
		patch.GeminiAPIKey = ptr
	}
	if ptr, ok := decodeLLMJSONString(raw, "anthropic_api_key"); ok {
		patch.AnthropicAPIKey = ptr
	}
	if ptr, ok := decodeLLMJSONString(raw, "gemini_model"); ok {
		patch.GeminiModel = ptr
	}
	if ptr, ok := decodeLLMJSONString(raw, "claude_model"); ok {
		patch.ClaudeModel = ptr
	}
	if ptr, ok := decodeLLMJSONString(raw, "tavily_api_key"); ok {
		patch.TavilyAPIKey = ptr
	}
	if ptr, ok := decodeLLMJSONString(raw, "deepseek_api_key"); ok {
		patch.DeepSeekAPIKey = ptr
	}
	if ptr, ok := decodeLLMJSONString(raw, "deepseek_model"); ok {
		patch.DeepSeekModel = ptr
	}
	if ptr, ok := decodeLLMJSONString(raw, "runpod_api_key"); ok {
		patch.RunpodAPIKey = ptr
	}
	if ptr, ok := decodeLLMJSONString(raw, "elevenlabs_api_key"); ok {
		patch.ElevenLabsAPIKey = ptr
	}
	if ptr, ok := decodeLLMJSONString(raw, "runpod_endpoint_id"); ok {
		patch.RunpodEndpointID = ptr
	}
	if ptr, ok := decodeLLMJSONInt(raw, "runpod_workers"); ok {
		patch.RunpodWorkers = ptr
	}
	if isVisitor {
		sid := h.readSessionCookie(r)
		if err := h.svc.PatchSessionVisitorLLM(r.Context(), sid, patch); err != nil {
			if errors.Is(err, repository.ErrVisitorSessionLLMNotUpdated) {
				writeError(w, http.StatusBadRequest, "could not save session LLM settings (invalid or expired session)")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to save session settings")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := h.svc.PatchUserLLMSettings(r.Context(), user.ID, patch); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save settings")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeLLMJSONInt(raw map[string]json.RawMessage, key string) (*int, bool) {
	b, ok := raw[key]
	if !ok {
		return nil, false
	}
	if len(b) == 0 || string(b) == "null" {
		zero := 0
		return &zero, true
	}
	var n int
	if err := json.Unmarshal(b, &n); err != nil {
		return nil, true
	}
	return &n, true
}

func decodeLLMJSONString(raw map[string]json.RawMessage, key string) (*string, bool) {
	b, ok := raw[key]
	if !ok {
		return nil, false
	}
	if len(b) == 0 || string(b) == "null" {
		empty := ""
		return &empty, true
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, true
	}
	return &s, true
}

// POST /auth/change-password
// Body: { "current_password": "...", "new_password": "..." }
// Requires an active session.
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	auth, err := h.svc.Authenticate(r.Context(), h.readSessionCookie(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "session lookup failed")
		return
	}
	if auth == nil || auth.User == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	user := auth.User

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "current_password and new_password are required")
		return
	}
	currentPassword := req.CurrentPassword
	newPassword := req.NewPassword

	if err := h.svc.ChangePassword(r.Context(), user.ID, currentPassword, newPassword); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidCredentials):
			writeError(w, http.StatusUnauthorized, "current password is incorrect")
		case errors.Is(err, service.ErrWeakPassword):
			writeError(w, http.StatusUnprocessableEntity, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "password change failed")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ── Cookie helpers ────────────────────────────────────────────────────────────

func (h *AuthHandler) readSessionCookie(r *http.Request) string {
	c, err := r.Cookie(service.AuthSessionCookieName)
	if err != nil || c == nil {
		return ""
	}
	return c.Value
}

func (h *AuthHandler) setSessionCookie(w http.ResponseWriter, value string, maxAge int) {
	// Lax (not Strict) so the session is sent on top-level GET navigations back from
	// external OAuth/OIDC providers (e.g. Gmail connect). Strict would drop dm_session
	// on /gmail/auth/callback and force a login loop with no token saved.
	http.SetCookie(w, &http.Cookie{
		Name:     service.AuthSessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.secure,
	})
}

// ── IP extraction ─────────────────────────────────────────────────────────────

// realIP returns the client IP after chi's RealIP middleware has processed
// X-Forwarded-For / X-Real-IP.  Falls back to r.RemoteAddr.
func realIP(r *http.Request) string {
	if ip := r.RemoteAddr; ip != "" {
		// Strip port — RemoteAddr is "host:port" for TCP connections.
		if i := strings.LastIndex(ip, ":"); i > 0 {
			return ip[:i]
		}
		return ip
	}
	return "unknown"
}

// ── Rate limiter ──────────────────────────────────────────────────────────────

// authRateLimiter enforces a per-IP token-bucket limit of 10 requests/minute
// on the login and register endpoints.
type authRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
}

func newAuthRateLimiter() *authRateLimiter {
	rl := &authRateLimiter{
		buckets: make(map[string]*tokenBucket),
	}
	// Periodic cleanup so the map doesn't grow unboundedly.
	go rl.cleanupLoop()
	return rl
}

// Allow returns true if the IP is within rate limits.
func (rl *authRateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	b, ok := rl.buckets[ip]
	if !ok {
		b = newTokenBucket(10, 10.0/60.0) // capacity 10, refill 10/min
		rl.buckets[ip] = b
	}
	rl.mu.Unlock()
	return b.take()
}

func (rl *authRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-10 * time.Minute)
		for ip, b := range rl.buckets {
			b.mu.Lock()
			if b.last.Before(cutoff) {
				delete(rl.buckets, ip)
			}
			b.mu.Unlock()
		}
		rl.mu.Unlock()
	}
}

// tokenBucket is a simple token-bucket rate limiter for one IP address.
type tokenBucket struct {
	mu       sync.Mutex
	tokens   float64
	last     time.Time
	rate     float64 // tokens per second
	capacity float64
}

func newTokenBucket(capacity, ratePerSec float64) *tokenBucket {
	return &tokenBucket{
		tokens:   capacity,
		last:     time.Now(),
		rate:     ratePerSec,
		capacity: capacity,
	}
}

// take deducts one token, refilling based on elapsed time first.
// Returns false when no tokens remain.
func (b *tokenBucket) take() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.last).Seconds()
	b.tokens = min(b.capacity, b.tokens+elapsed*b.rate)
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}
