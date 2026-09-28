package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	appai "github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// MCPServersHandler handles /api/mcp-servers/* endpoints — the deployment-wide list of MCP
// servers the app discovers AI tools from (the bundled cmd/mcpserver plus any additional ones).
type MCPServersHandler struct {
	svc     *service.MCPServersService
	authSvc *service.AuthService
}

// NewMCPServersHandler creates an MCPServersHandler.
func NewMCPServersHandler(svc *service.MCPServersService, authSvc *service.AuthService) *MCPServersHandler {
	return &MCPServersHandler{svc: svc, authSvc: authSvc}
}

// RegisterRoutes mounts MCP server routes.
func (h *MCPServersHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/mcp-servers", h.List)
	r.Post("/api/mcp-servers", h.Create)
	r.Patch("/api/mcp-servers/{id}", h.Update)
	r.Delete("/api/mcp-servers/{id}", h.Delete)
	r.Post("/api/mcp-servers/test", h.Test)
}

// requireOwner is the auth gate for every route here: MCP server config includes credentials
// (auth_token) for endpoints the archive owner explicitly chooses to trust with tool-call
// context, so — like the AI Tool Access and API Keys tabs — this is owner-only, not just
// any authenticated session.
func (h *MCPServersHandler) requireOwner(w http.ResponseWriter, r *http.Request) bool {
	if !ArchiveOwnerAuthenticated(r, h.authSvc) {
		writeError(w, http.StatusForbidden, "owner session required")
		return false
	}
	return true
}

func parseMCPServerID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be an integer")
		return 0, false
	}
	return id, true
}

func parseMCPServerInput(w http.ResponseWriter, r *http.Request) (service.MCPServerInput, bool) {
	var in service.MCPServerInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return service.MCPServerInput{}, false
	}
	return in, true
}

// mcpServerJSON never echoes the raw auth_token back — only whether one is set, same convention
// as the API Keys tab (internal/handler/auth_handler.go returns *_set booleans, never raw keys).
func mcpServerJSON(m *service.MCPServer) map[string]any {
	return map[string]any{
		"id":             m.ID,
		"name":           m.Name,
		"endpoint_url":   m.EndpointURL,
		"auth_token_set": strings.TrimSpace(m.AuthToken) != "",
		"enabled":        m.Enabled,
		"is_builtin":     m.IsBuiltin,
		"sort_order":     m.SortOrder,
		"scope":          m.Scope,
	}
}

// List handles GET /api/mcp-servers.
func (h *MCPServersHandler) List(w http.ResponseWriter, r *http.Request) {
	if !h.requireOwner(w, r) {
		return
	}
	servers, err := h.svc.ListAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error listing mcp servers: %s", err))
		return
	}
	out := make([]map[string]any, 0, len(servers))
	for _, m := range servers {
		mm := m
		out = append(out, mcpServerJSON(&mm))
	}
	writeJSON(w, map[string]any{"servers": out})
}

// Create handles POST /api/mcp-servers.
func (h *MCPServersHandler) Create(w http.ResponseWriter, r *http.Request) {
	if !h.requireOwner(w, r) {
		return
	}
	in, ok := parseMCPServerInput(w, r)
	if !ok {
		return
	}
	m, err := h.svc.Create(r.Context(), in)
	if err != nil {
		if strings.HasPrefix(err.Error(), "conflict:") {
			writeError(w, http.StatusConflict, strings.TrimPrefix(err.Error(), "conflict:"))
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, mcpServerJSON(m))
}

// Update handles PATCH /api/mcp-servers/{id}.
func (h *MCPServersHandler) Update(w http.ResponseWriter, r *http.Request) {
	if !h.requireOwner(w, r) {
		return
	}
	id, ok := parseMCPServerID(w, r)
	if !ok {
		return
	}
	in, ok := parseMCPServerInput(w, r)
	if !ok {
		return
	}
	m, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		if strings.HasPrefix(err.Error(), "conflict:") {
			writeError(w, http.StatusConflict, strings.TrimPrefix(err.Error(), "conflict:"))
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if m == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("mcp server not found: id=%d", id))
		return
	}
	writeJSON(w, mcpServerJSON(m))
}

// Delete handles DELETE /api/mcp-servers/{id}.
func (h *MCPServersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if !h.requireOwner(w, r) {
		return
	}
	id, ok := parseMCPServerID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		if strings.HasPrefix(err.Error(), "conflict:") {
			writeError(w, http.StatusConflict, strings.TrimPrefix(err.Error(), "conflict:"))
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error deleting mcp server: %s", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type mcpServerTestBody struct {
	ID          *int64 `json:"id"`           // test an existing (saved) server, builtin included
	EndpointURL string `json:"endpoint_url"` // or test an unsaved endpoint directly
	AuthToken   string `json:"auth_token"`
}

// Test handles POST /api/mcp-servers/test — attempts a live tools/list against either an unsaved
// {endpoint_url, auth_token} pair (for the add/edit form's Test button before Save) or an
// existing saved server by {id} (including the builtin row, which tests DefaultMCPClient()
// rather than any endpoint_url/auth_token, since those columns are unused for it).
func (h *MCPServersHandler) Test(w http.ResponseWriter, r *http.Request) {
	if !h.requireOwner(w, r) {
		return
	}
	var body mcpServerTestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var client *appai.MCPClient
	if body.ID != nil {
		existing, err := h.svc.GetByID(r.Context(), *body.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("error loading mcp server: %s", err))
			return
		}
		if existing == nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("mcp server not found: id=%d", *body.ID))
			return
		}
		if existing.IsBuiltin {
			client = appai.DefaultMCPClient()
			if client == nil {
				writeJSON(w, map[string]any{"ok": false, "error": "MCP_SERVER_URL is not configured for this process"})
				return
			}
		} else {
			client = appai.NewMCPClient(existing.EndpointURL, existing.AuthToken)
		}
	} else {
		endpointURL := strings.TrimSpace(body.EndpointURL)
		if endpointURL == "" {
			writeError(w, http.StatusBadRequest, "endpoint_url is required")
			return
		}
		client = appai.NewMCPClient(endpointURL, strings.TrimSpace(body.AuthToken))
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	defs, err := client.ListToolDefinitions(ctx)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "tool_count": len(defs)})
}
