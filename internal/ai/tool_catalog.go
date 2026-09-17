package ai

import (
	"context"
	"errors"
	"sync"
	"time"
)

// errMCPNotConfigured is returned by ToolCatalog.Refresh when no MCP server is registered at all
// (e.g. the bundled server's MCP_SERVER_URL is unset — `go run ./cmd/server` outside Electron —
// and no additional servers are configured either).
var errMCPNotConfigured = errors.New("no MCP servers configured")

// ToolCatalog is the process-wide cache of tool schemas discovered from every registered MCP
// server (internal/ai/mcp_registry.go) via real tools/list calls. It replaces the old static
// toolDefinitions() in provider.go — every AI tool now lives in an MCP server, not this app.
//
// Definitions are kept per source (server prefix) so one server's failure doesn't blank out
// another's previously-successful results — a transient outage on one additional server must
// not take down the bundled server's own tools, or vice versa.
//
// The cache is in-memory only (never persisted) since it's meant to reflect what the registered
// servers currently report, not a stale snapshot — a restart naturally re-discovers.
type ToolCatalog struct {
	mu          sync.RWMutex
	bySource    map[string][]map[string]any // key: "" for the bundled server, "<name>:" for an additional one
	errBySource map[string]error
	lastRefresh time.Time
}

var defaultToolCatalog = &ToolCatalog{}

// DefaultToolCatalog returns the process-wide catalog singleton.
func DefaultToolCatalog() *ToolCatalog { return defaultToolCatalog }

// SeedCatalogForTests directly sets the default catalog's cached definitions (as if they'd come
// from the bundled server), bypassing MCP discovery — for tests in other packages that need a
// known tool set without a real (or fake) MCP server round-trip. Not for production use.
func SeedCatalogForTests(defs []map[string]any) {
	defaultToolCatalog.mu.Lock()
	defer defaultToolCatalog.mu.Unlock()
	defaultToolCatalog.bySource = map[string][]map[string]any{"": defs}
	defaultToolCatalog.errBySource = nil
}

// Refresh re-fetches the tool list from every registered MCP server (DefaultMCPRegistry) via
// tools/list, independently per server, and replaces the cached definitions on success. A
// server that fails this round keeps its previous definitions (if any) rather than disappearing
// from the catalog — see the type doc. Additional-server tool names are namespaced
// "<server_name>:<tool_name>" before merging (see internal/ai/mcp_registry.go); the bundled
// server's stay unprefixed. Returns the first error encountered, if any, but the cache is still
// updated with whatever succeeded even when it returns an error (partial success).
func (c *ToolCatalog) Refresh(ctx context.Context) error {
	entries := DefaultMCPRegistry().Entries()
	if len(entries) == 0 {
		c.mu.Lock()
		c.errBySource = map[string]error{"": errMCPNotConfigured}
		c.lastRefresh = time.Now()
		c.mu.Unlock()
		return errMCPNotConfigured
	}

	newBySource := make(map[string][]map[string]any, len(entries))
	newErrs := make(map[string]error, len(entries))
	var firstErr error
	for _, e := range entries {
		defs, err := e.client.ListToolDefinitions(ctx)
		if err != nil {
			newErrs[e.prefix] = err
			if firstErr == nil {
				firstErr = err
			}
			c.mu.RLock()
			prev := c.bySource[e.prefix]
			c.mu.RUnlock()
			newBySource[e.prefix] = prev
			continue
		}
		if e.prefix == "" {
			newBySource[e.prefix] = defs
			continue
		}
		prefixed := make([]map[string]any, 0, len(defs))
		for _, d := range defs {
			nd := make(map[string]any, len(d))
			for k, v := range d {
				nd[k] = v
			}
			if name, _ := nd["name"].(string); name != "" {
				nd["name"] = e.prefix + name
			}
			prefixed = append(prefixed, nd)
		}
		newBySource[e.prefix] = prefixed
	}

	c.mu.Lock()
	c.bySource = newBySource
	c.errBySource = newErrs
	c.lastRefresh = time.Now()
	c.mu.Unlock()
	return firstErr
}

// Definitions returns the last successfully discovered tool schemas, merged across every
// registered MCP server ({name, description, parameters} maps, same shape the old static list
// used — additional-server tool names carry their "<server_name>:" prefix).
func (c *ToolCatalog) Definitions() []map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []map[string]any
	for _, defs := range c.bySource {
		out = append(out, defs...)
	}
	return out
}

// Status reports the last refresh outcome, for the config UI / diagnostics: total tool count
// across every source, when the last refresh attempt ran, and the first per-source error (if
// any) from that attempt.
func (c *ToolCatalog) Status() (toolCount int, lastRefresh time.Time, lastErr error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, defs := range c.bySource {
		toolCount += len(defs)
	}
	for _, err := range c.errBySource {
		if lastErr == nil {
			lastErr = err
		}
	}
	return toolCount, c.lastRefresh, lastErr
}

// RefreshWithRetry attempts Refresh up to attempts times, waiting delay between tries, stopping
// early on success (every server refreshed cleanly) or ctx cancellation. Used at server startup:
// the MCP server is spawned by Electron only after the Go server's own /health check passes (see
// electron/main.js), so it provably isn't up yet the instant this process boots — a bounded
// retry is what makes "discover tools at startup" actually converge once it comes up moments
// later, without making /health depend on it.
func (c *ToolCatalog) RefreshWithRetry(ctx context.Context, attempts int, delay time.Duration) error {
	var err error
	for i := 0; i < attempts; i++ {
		if err = c.Refresh(ctx); err == nil {
			return nil
		}
		if i == attempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return err
}
