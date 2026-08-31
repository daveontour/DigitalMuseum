package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	appai "github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

const toolTestResultMaxBytes = 100 * 1024

// LLMToolsTestHandler runs diagnostic executions of chat AI tools for the owner UI.
type LLMToolsTestHandler struct {
	pool          *sql.DB
	sessionStore  *keystore.SessionMasterStore
	subjectConfig *repository.SubjectConfigRepo
	chatSvc       *service.ChatService
	pepper        string
	authSvc       *service.AuthService
}

// NewLLMToolsTestHandler constructs LLMToolsTestHandler. The Tavily API key used by the
// web-search tool is resolved per-request from chatSvc (owner-configured only — there is
// no server-wide default).
func NewLLMToolsTestHandler(
	pool *sql.DB,
	sessionStore *keystore.SessionMasterStore,
	subjectConfig *repository.SubjectConfigRepo,
	chatSvc *service.ChatService,
	pepper string,
	authSvc *service.AuthService,
) *LLMToolsTestHandler {
	return &LLMToolsTestHandler{
		pool:          pool,
		sessionStore:  sessionStore,
		subjectConfig: subjectConfig,
		chatSvc:       chatSvc,
		pepper:        pepper,
		authSvc:       authSvc,
	}
}

// RegisterRoutes mounts tool test routes.
func (h *LLMToolsTestHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/settings/llm-tools/definitions", h.GetDefinitions)
	r.Post("/api/settings/llm-tools/test", h.TestTools)
}

func knownChatToolNames() map[string]struct{} {
	out := make(map[string]struct{})
	for _, td := range appai.GetToolDefinitions() {
		name, _ := td["name"].(string)
		if name != "" {
			out[name] = struct{}{}
		}
	}
	return out
}

func validateToolTestNames(names []string) (invalid []string) {
	valid := knownChatToolNames()
	for _, name := range names {
		n := strings.TrimSpace(name)
		if n == "" {
			continue
		}
		if _, ok := valid[n]; !ok {
			invalid = append(invalid, n)
		}
	}
	return invalid
}

func truncateToolTestResult(result map[string]any) (map[string]any, bool) {
	if result == nil {
		return nil, false
	}
	b, err := json.Marshal(result)
	if err != nil || len(b) <= toolTestResultMaxBytes {
		return result, false
	}
	truncated := map[string]any{
		"truncated": true,
		"preview":   string(b[:toolTestResultMaxBytes]) + "…",
	}
	return truncated, true
}

func (h *LLMToolsTestHandler) definitionsAllowed(w http.ResponseWriter, r *http.Request) bool {
	if h.sessionStore != nil {
		unlocked, master := h.sessionStore.SessionStatus(r)
		if unlocked && master {
			return true
		}
	}
	if ArchiveOwnerAuthenticated(r, h.authSvc) {
		return true
	}
	writeError(w, http.StatusForbidden, "authentication required")
	return false
}

func (h *LLMToolsTestHandler) loadSubjectName(ctx context.Context) string {
	subjectName := "the archive owner"
	if h.subjectConfig == nil {
		return subjectName
	}
	cfg, err := h.subjectConfig.GetFirst(ctx)
	if err != nil || cfg == nil {
		return subjectName
	}
	if s := strings.TrimSpace(cfg.SubjectName); s != "" {
		return s
	}
	return subjectName
}

func (h *LLMToolsTestHandler) getRAM(r *http.Request) appai.RAMMasterGetter {
	return func() (string, bool) {
		if h.sessionStore == nil || r == nil {
			return "", false
		}
		return h.sessionStore.Get(r)
	}
}

// GetDefinitions GET /api/settings/llm-tools/definitions
func (h *LLMToolsTestHandler) GetDefinitions(w http.ResponseWriter, r *http.Request) {
	if !h.definitionsAllowed(w, r) {
		return
	}
	writeJSON(w, map[string]any{"tools": appai.GetToolDefinitions()})
}

type toolTestRequestItem struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type toolTestRequestBody struct {
	Tools []toolTestRequestItem `json:"tools"`
}

// TestTools POST /api/settings/llm-tools/test
func (h *LLMToolsTestHandler) TestTools(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	if h.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}
	var body toolTestRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(body.Tools) == 0 {
		writeError(w, http.StatusBadRequest, "at least one tool is required")
		return
	}

	names := make([]string, 0, len(body.Tools))
	for _, t := range body.Tools {
		names = append(names, t.Name)
	}
	if invalid := validateToolTestNames(names); len(invalid) > 0 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown tool(s): %s", strings.Join(invalid, ", ")))
		return
	}

	ctx := r.Context()
	if appctx.UserIDFromCtx(ctx) == 0 {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	subjectName := h.loadSubjectName(ctx)
	tavilyKey := ""
	if h.chatSvc != nil {
		tavilyKey = h.chatSvc.EffectiveTavilyKey(ctx, r)
	}
	executor := appai.NewToolExecutor(h.pool, subjectName, tavilyKey, h.pepper, h.getRAM(r))

	results := make([]map[string]any, 0, len(body.Tools))
	for _, item := range body.Tools {
		name := strings.TrimSpace(item.Name)
		args := item.Arguments
		if args == nil {
			args = map[string]any{}
		}
		start := time.Now()
		result, err := executor(ctx, name, args)
		elapsed := time.Since(start).Milliseconds()
		entry := map[string]any{
			"name":        name,
			"arguments":   args,
			"duration_ms": elapsed,
		}
		if err != nil {
			entry["error"] = err.Error()
			entry["result"] = nil
		} else {
			entry["error"] = nil
			truncated, wasTruncated := truncateToolTestResult(result)
			entry["result"] = truncated
			if wasTruncated {
				entry["truncated"] = true
			}
		}
		results = append(results, entry)
	}
	writeJSON(w, map[string]any{"results": results})
}
