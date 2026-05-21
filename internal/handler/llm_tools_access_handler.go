package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	appai "github.com/daveontour/aimuseum/internal/ai"
	appcrypto "github.com/daveontour/aimuseum/internal/crypto"
	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// LLMToolsAccessHandler reads/writes LLM tool visibility policy in private_store.
type LLMToolsAccessHandler struct {
	privateStore *service.PrivateStoreService
	sessionStore *keystore.SessionMasterStore
	authSvc      *service.AuthService
}

// NewLLMToolsAccessHandler constructs the handler.
func NewLLMToolsAccessHandler(privateStore *service.PrivateStoreService, sessionStore *keystore.SessionMasterStore, authSvc *service.AuthService) *LLMToolsAccessHandler {
	return &LLMToolsAccessHandler{privateStore: privateStore, sessionStore: sessionStore, authSvc: authSvc}
}

func (h *LLMToolsAccessHandler) resolveLLMToolsMasterPassword(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-Master-Password")); v != "" {
		return appcrypto.NormalizeKeyringPassword(v)
	}
	return resolveMasterPassword(r.URL.Query().Get("master_password"), r, h.sessionStore)
}

// RegisterRoutes mounts GET/PUT /api/settings/llm-tools-access.
func (h *LLMToolsAccessHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/settings/llm-tools-access", h.Get)
	r.Put("/api/settings/llm-tools-access", h.Put)
}

func (h *LLMToolsAccessHandler) masterPassword(w http.ResponseWriter, r *http.Request) (string, bool) {
	mp := h.resolveLLMToolsMasterPassword(r)
	if strings.TrimSpace(mp) == "" {
		writeError(w, http.StatusForbidden, "keyring password required (unlock session or pass X-Master-Password)")
		return "", false
	}
	return mp, true
}

// Get returns all tools with a single enabled flag (default false if unset).
func (h *LLMToolsAccessHandler) Get(w http.ResponseWriter, r *http.Request) {
	mp := h.resolveLLMToolsMasterPassword(r)
	var policy appai.ToolAccessPolicy
	if strings.TrimSpace(mp) != "" && h.privateStore != nil {
		rec, err := h.privateStore.GetByKey(r.Context(), appai.LLMToolsAccessStoreKey, mp)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("load policy: %s", err))
			return
		}
		if rec != nil && strings.TrimSpace(rec.Value) != "" {
			p, err := appai.ParseToolAccessPolicyJSON(rec.Value)
			if err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid stored policy JSON: %s", err))
				return
			}
			policy = p
		}
	} else if !ArchiveOwnerAuthenticated(r, h.authSvc) {
		writeError(w, http.StatusForbidden, "keyring password required (unlock session or pass X-Master-Password)")
		return
	}
	out := make([]map[string]any, 0)
	for _, meta := range appai.AllToolMetas() {
		rule := appai.ToolAccessRule{}
		if policy != nil {
			if x, ok := policy[meta.Name]; ok {
				rule = x
			}
		}
		out = append(out, map[string]any{
			"name":        meta.Name,
			"description": meta.Description,
			"enabled":     rule.Enabled,
		})
	}
	writeJSON(w, map[string]any{"tools": out})
}

// PutBody is the JSON body for saving policy.
type llmToolsPutBody struct {
	Tools []struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	} `json:"tools"`
}

// Put replaces the stored policy (merges only listed tools; unknown names ignored).
func (h *LLMToolsAccessHandler) Put(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	mp, ok := h.masterPassword(w, r)
	if !ok {
		return
	}
	var body llmToolsPutBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	valid := map[string]struct{}{}
	for _, m := range appai.AllToolMetas() {
		valid[m.Name] = struct{}{}
	}
	policy := make(appai.ToolAccessPolicy)
	for _, t := range body.Tools {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			continue
		}
		if _, ok := valid[name]; !ok {
			continue
		}
		policy[name] = appai.ToolAccessRule{
			Enabled: t.Enabled,
		}
	}
	raw, err := appai.MarshalToolAccessPolicyJSON(policy)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("encode policy: %s", err))
		return
	}
	if h.privateStore == nil {
		writeError(w, http.StatusInternalServerError, "private store not configured")
		return
	}
	if err := h.privateStore.Upsert(r.Context(), appai.LLMToolsAccessStoreKey, raw, mp); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("save policy: %s", err))
		return
	}
	writeJSON(w, map[string]string{"message": "LLM tools access policy saved"})
}
