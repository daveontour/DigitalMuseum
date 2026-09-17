package ai

import (
	"strings"
	"sync"
)

// MCPRegistryServer is the shape MCPServersService (internal/service) pushes into the registry
// on every load/write of the mcp_servers table. Defined here (not imported from service) so
// internal/ai — which internal/service already depends on — doesn't depend back on it.
type MCPRegistryServer struct {
	Name        string // display name; also the additional server's tool-name prefix
	EndpointURL string
	AuthToken   string
	Enabled     bool
	IsBuiltin   bool // true for exactly one row: the bundled cmd/mcpserver
}

// mcpRegistryEntry is one live, connectable server: the bundled one (prefix "", trusted) or an
// additional one (prefix "<name>:", untrusted).
type mcpRegistryEntry struct {
	prefix  string
	client  *MCPClient
	trusted bool
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
		if s.IsBuiltin {
			client := DefaultMCPClient()
			if client == nil {
				continue // MCP_SERVER_URL unset — no bundled server configured in this process at all
			}
			entries = append(entries, mcpRegistryEntry{prefix: "", client: client, trusted: true})
			continue
		}
		entries = append(entries, mcpRegistryEntry{
			prefix:  s.Name + ":",
			client:  NewMCPClient(s.EndpointURL, s.AuthToken),
			trusted: false,
		})
	}
	r.mu.Lock()
	r.entries = entries
	r.synced = true
	r.mu.Unlock()
}

// Entries returns a snapshot of the current registry entries, for ToolCatalog.Refresh to iterate.
// If SyncServers has never run yet (very first boot, before MCPServersService.EnsureRegistryLoaded
// completes), falls back to just the bundled server (if configured) so tool discovery never
// regresses to "nothing" while waiting for the DB-backed sync.
func (r *MCPRegistry) Entries() []mcpRegistryEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.synced {
		if client := DefaultMCPClient(); client != nil {
			return []mcpRegistryEntry{{prefix: "", client: client, trusted: true}}
		}
		return nil
	}
	out := make([]mcpRegistryEntry, len(r.entries))
	copy(out, r.entries)
	return out
}

// Resolve maps a (possibly namespaced) tool name — as the LLM called it — to the server that
// should handle it: the client to call, the bare name to actually send in the tools/call request
// (server prefixes are purely a main-app concept; the target server has no idea about them), and
// whether that server is trusted (gates secret hidden-args — see NewMCPToolExecutor).
func (r *MCPRegistry) Resolve(toolName string) (client *MCPClient, bareName string, trusted bool, ok bool) {
	entries := r.Entries()
	for _, e := range entries {
		if e.prefix != "" && strings.HasPrefix(toolName, e.prefix) {
			return e.client, strings.TrimPrefix(toolName, e.prefix), e.trusted, true
		}
	}
	for _, e := range entries {
		if e.prefix == "" {
			return e.client, toolName, e.trusted, true
		}
	}
	return nil, "", false, false
}
