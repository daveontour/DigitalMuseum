package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	appai "github.com/daveontour/digitalmuseum/internal/ai"
	appcrypto "github.com/daveontour/digitalmuseum/internal/crypto"
	"github.com/daveontour/digitalmuseum/internal/keystore"
	"github.com/daveontour/digitalmuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// LLMToolsAccessHandler reads/writes LLM tool visibility policy in private_store.
type LLMToolsAccessHandler struct {
	privateStore *service.PrivateStoreService
	sessionStore *keystore.SessionMasterStore
}

// NewLLMToolsAccessHandler constructs the handler.
func NewLLMToolsAccessHandler(privateStore *service.PrivateStoreService, sessionStore *keystore.SessionMasterStore) *LLMToolsAccessHandler {
	return &LLMToolsAccessHandler{privateStore: privateStore, sessionStore: sessionStore}
}

// RegisterRoutes mounts GET/PUT /api/settings/llm-tools-access.
func (h *LLMToolsAccessHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/settings/llm-tools-access", h.Get)
	r.Put("/api/settings/llm-tools-access", h.Put)
}

func (h *LLMToolsAccessHandler) masterPassword(w http.ResponseWriter, r *http.Request) (string, bool) {
	if v := strings.TrimSpace(r.Header.Get("X-Master-Password")); v != "" {
		return appcrypto.NormalizeKeyringPassword(v), true
	}
	mp := resolveMasterPassword(r.URL.Query().Get("master_password"), r, h.sessionStore)
	if strings.TrimSpace(mp) == "" {
		writeError(w, http.StatusForbidden, "keyring password required (unlock session or pass X-Master-Password)")
		return "", false
	}
	return mp, true
}

// Get returns all tools with no_key / visitor / master flags (default false if unset).
func (h *LLMToolsAccessHandler) Get(w http.ResponseWriter, r *http.Request) {
	mp, ok := h.masterPassword(w, r)
	if !ok {
		return
	}
	var policy appai.ToolAccessPolicy
	if h.privateStore != nil {
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
			"no_key":      rule.NoKey,
			"visitor":     rule.Visitor,
			"master":      rule.Master,
		})
	}
	writeJSON(w, map[string]any{"tools": out})
}

// PutBody is the JSON body for saving policy.
type llmToolsPutBody struct {
	Tools []struct {
		Name    string `json:"name"`
		NoKey   bool   `json:"no_key"`
		Visitor bool   `json:"visitor"`
		Master  bool   `json:"master"`
	} `json:"tools"`
}

// Put replaces the stored policy (merges only listed tools; unknown names ignored).
func (h *LLMToolsAccessHandler) Put(w http.ResponseWriter, r *http.Request) {
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
			NoKey:   t.NoKey,
			Visitor: t.Visitor,
			Master:  t.Master,
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
