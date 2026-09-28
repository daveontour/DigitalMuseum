package ai

import (
	"strings"
	"sync"
)

// MCP server scopes. "archive" is the bundled archive-data server (cmd/mcpserver), used only by
// the persona chat. "chatbot" is the bundled ChatBot-only utility server (cmd/chatbotmcpserver),
// used only by the ChatBot feature. "shared" is any owner-added additional server, usable by
// both features. Exactly one is_builtin row exists per archive/chatbot scope.
const (
	ScopeArchive = "archive"
	ScopeChatbot = "chatbot"
	ScopeShared  = "shared"
)

// MCPRegistryServer is the shape MCPServersService (internal/service) pushes into the registry
// on every load/write of the mcp_servers table. Defined here (not imported from service) so
// internal/ai — which internal/service already depends on — doesn't depend back on it.
type MCPRegistryServer struct {
	Name        string // display name; also the additional/chatbot server's tool-name prefix
	EndpointURL string
	AuthToken   string
	Enabled     bool
	IsBuiltin   bool   // true for exactly two rows: the bundled archive and chatbot servers
	Scope       string // ScopeArchive | ScopeChatbot | ScopeShared
}

// mcpRegistryEntry is one live, connectable server: the archive-builtin one (prefix "",
// trusted), the chatbot-builtin one (prefix "<name>:", trusted), or an additional/shared one
// (prefix "<name>:", untrusted).
type mcpRegistryEntry struct {
	prefix  string
	client  *MCPClient
	trusted bool
	scope   string
}

// MCPRegistry is the process-wide map from tool name to the MCP server that serves it, rebuilt
// each time MCPServersService syncs the current mcp_servers rows. It's what makes
// NewMCPToolExecutor and ToolCatalog.Refresh multi-server-aware instead of assuming a single
// bundled client.
type MCPRegistry struct {
	mu      sync.RWMutex
	entries []mcpRegistryEntry
	synced  bool // false until SyncServers has been called at least once
}

var defaultMCPRegistry = &MCPRegistry{}

// DefaultMCPRegistry returns the process-wide registry singleton.
func DefaultMCPRegistry() *MCPRegistry { return defaultMCPRegistry }

// SyncServers rebuilds the registry's entries from the given rows (every enabled row —
// disabled servers, builtin included, are simply omitted). Safe to call repeatedly; each call
// fully replaces the previous entry list.
func (r *MCPRegistry) SyncServers(servers []MCPRegistryServer) {
	entries := make([]mcpRegistryEntry, 0, len(servers))
	for _, s := range servers {
		if !s.Enabled {
			continue
		}
		if s.IsBuiltin && s.Scope == ScopeChatbot {
			client := DefaultChatbotMCPClient()
			if client == nil {
				continue // CHATBOT_MCP_SERVER_URL unset — chatbot MCP server not configured in this process
			}
			// Namespaced like an additional server (not empty prefix) — ToolCatalog.bySource is
			// keyed by prefix, so sharing "" with the archive server would clobber one or the other.
			entries = append(entries, mcpRegistryEntry{prefix: s.Name + ":", client: client, trusted: true, scope: ScopeChatbot})
			continue
		}
		if s.IsBuiltin {
			client := DefaultMCPClient()
			if client == nil {
				continue // MCP_SERVER_URL unset — no bundled server configured in this process at all
			}
			entries = append(entries, mcpRegistryEntry{prefix: "", client: client, trusted: true, scope: ScopeArchive})
			continue
		}
		entries = append(entries, mcpRegistryEntry{
			prefix:  s.Name + ":",
			client:  NewMCPClient(s.EndpointURL, s.AuthToken),
			trusted: false,
			scope:   ScopeShared,
		})
	}
	r.mu.Lock()
	r.entries = entries
	r.synced = true
	r.mu.Unlock()
}

// Entries returns a snapshot of the current registry entries, for ToolCatalog.Refresh to iterate.
// If SyncServers has never run yet (very first boot, before MCPServersService.EnsureRegistryLoaded
// completes), falls back to just the bundled servers (archive/chatbot, whichever are configured)
// so tool discovery never regresses to "nothing" while waiting for the DB-backed sync.
func (r *MCPRegistry) Entries() []mcpRegistryEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.synced {
		var out []mcpRegistryEntry
		if client := DefaultMCPClient(); client != nil {
			out = append(out, mcpRegistryEntry{prefix: "", client: client, trusted: true, scope: ScopeArchive})
		}
		if client := DefaultChatbotMCPClient(); client != nil {
			out = append(out, mcpRegistryEntry{prefix: BuiltinChatbotToolPrefix, client: client, trusted: true, scope: ScopeChatbot})
		}
		return out
	}
	out := make([]mcpRegistryEntry, len(r.entries))
	copy(out, r.entries)
	return out
}

// BuiltinChatbotToolPrefix is the tool-name prefix used for the bundled ChatBot-only utility
// server before the first DB sync completes (see Entries' fallback) — matches the display name
// database.BuiltinChatbotMCPServerName ("ChatBot Tools") that SyncServers derives the prefix
// from once synced, so pre- and post-sync tool names for that server are identical.
const BuiltinChatbotToolPrefix = "ChatBot Tools:"

// Resolve maps a (possibly namespaced) tool name — as the LLM called it — to the server that
// should handle it: the client to call, the bare name to actually send in the tools/call request
// (server prefixes are purely a main-app concept; the target server has no idea about them), and
// whether that server is trusted (gates secret hidden-args — see NewMCPToolExecutor).
//
// allowedScopes restricts which servers may be resolved to, keyed by scope constant
// (ScopeArchive/ScopeChatbot/ScopeShared) — a match whose scope isn't present returns ok=false,
// same as "no server registered for this tool". This is defense-in-depth: even if a policy or
// tool-definition filtering bug ever advertised an out-of-scope tool to a caller, the executor
// itself still refuses to dispatch the call.
func (r *MCPRegistry) Resolve(toolName string, allowedScopes map[string]bool) (client *MCPClient, bareName string, trusted bool, ok bool) {
	entries := r.Entries()
	for _, e := range entries {
		if e.prefix != "" && strings.HasPrefix(toolName, e.prefix) {
			if !allowedScopes[e.scope] {
				return nil, "", false, false
			}
			return e.client, strings.TrimPrefix(toolName, e.prefix), e.trusted, true
		}
	}
	for _, e := range entries {
		if e.prefix == "" {
			if !allowedScopes[e.scope] {
				return nil, "", false, false
			}
			return e.client, toolName, e.trusted, true
		}
	}
	return nil, "", false, false
}
