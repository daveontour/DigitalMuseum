package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpCallTimeout bounds every tools/list and tools/call round-trip, applied uniformly to the
// bundled server and every additional one. Additional servers can be arbitrary remote/
// third-party endpoints (see internal/ai/mcp_registry.go) that may be slow or unreachable in
// ways the local bundled server isn't — one hung server must not stall a chat turn or a catalog
// refresh that touches every registered server.
const mcpCallTimeout = 20 * time.Second

// MCPClient is a thin wrapper around an MCP ClientSession connected to the local
// tool-execution MCP server (cmd/mcpserver), which hosts every AI tool. See
// NewMCPToolExecutor (tools.go) and DefaultToolCatalog (tool_catalog.go).
type MCPClient struct {
	endpoint  string
	authToken string

	mu      sync.Mutex
	client  *mcp.Client
	session *mcp.ClientSession
}

// NewMCPClientFromEnv builds an MCPClient from MCP_SERVER_URL / MCP_AUTH_TOKEN, the env
// vars Electron injects into the main server's process (see electron/main.js). Returns nil
// when MCP_SERVER_URL is unset, so callers fall back to the in-process tool executor —
// this keeps `go run ./cmd/server` working without the MCP server configured.
func NewMCPClientFromEnv() *MCPClient {
	endpoint := strings.TrimSpace(os.Getenv("MCP_SERVER_URL"))
	if endpoint == "" {
		return nil
	}
	return &MCPClient{
		endpoint:  endpoint,
		authToken: strings.TrimSpace(os.Getenv("MCP_AUTH_TOKEN")),
	}
}

// NewMCPClient builds an MCPClient directly, bypassing env vars — for tests that need to point
// at a fake MCP server. Production code should use NewMCPClientFromEnv / DefaultMCPClient.
func NewMCPClient(endpoint, authToken string) *MCPClient {
	return &MCPClient{endpoint: endpoint, authToken: authToken}
}

var (
	defaultMCPClientOnce sync.Once
	defaultMCPClient     *MCPClient
)

// DefaultMCPClient returns a process-wide MCPClient built from env vars (nil when
// MCP_SERVER_URL is unset). Constructed once and reused for the life of the process.
func DefaultMCPClient() *MCPClient {
	defaultMCPClientOnce.Do(func() { defaultMCPClient = NewMCPClientFromEnv() })
	return defaultMCPClient
}

// SetDefaultMCPClientForTests overrides the process-wide MCPClient singleton — for tests that
// need DefaultMCPClient to point at a fake MCP server regardless of env vars or whether some
// earlier test already triggered the real env-based initialization. Not for production use.
func SetDefaultMCPClientForTests(c *MCPClient) {
	defaultMCPClientOnce.Do(func() {}) // consume the Once so a later real init can't clobber c
	defaultMCPClient = c
}

// authRoundTripper attaches the shared-secret bearer token to every request, so only
// this process (which holds MCP_AUTH_TOKEN) can call the local MCP server.
type authRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (t *authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.token != "" {
		req.Header.Set("Authorization", "Bearer "+t.token)
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

// ensureSession lazily connects (and reconnects after a failure) to the MCP server.
func (c *MCPClient) ensureSession(ctx context.Context) (*mcp.ClientSession, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session != nil {
		return c.session, nil
	}
	httpClient := &http.Client{Transport: &authRoundTripper{token: c.authToken}}
	c.client = mcp.NewClient(&mcp.Implementation{Name: "digitalmuseum-server", Version: "1.0.0"}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint:             c.endpoint,
		HTTPClient:           httpClient,
		DisableStandaloneSSE: true, // request/response only — no server-initiated messages needed
	}
	session, err := c.client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, err
	}
	c.session = session
	return session, nil
}

// dropSession discards the cached session (closing it best-effort) so the next call
// establishes a fresh one. The MCP server's session store is process-local in-memory state
// (cmd/mcpserver), so a cached session ID goes stale whenever that process restarts (an
// archive switch respawns it — see electron/main.js startMcpServerForCurrentArchive) or its
// session store evicts an idle session — neither of which this client can observe directly,
// only infer from the next call failing.
func (c *MCPClient) dropSession() {
	c.mu.Lock()
	old := c.session
	c.session = nil
	c.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
}

// withSession runs fn against the current (or newly established) session; on failure it
// assumes the session went stale (e.g. "session not found" after the MCP server process
// restarted), drops it, and retries once against a freshly connected session before giving up.
func (c *MCPClient) withSession(ctx context.Context, fn func(*mcp.ClientSession) error) error {
	session, err := c.ensureSession(ctx)
	if err != nil {
		return err
	}
	if err := fn(session); err != nil {
		c.dropSession()
		session, err = c.ensureSession(ctx)
		if err != nil {
			return err
		}
		return fn(session)
	}
	return nil
}

// CallTool invokes a tool on the MCP server and returns its decoded JSON result.
func (c *MCPClient) CallTool(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, mcpCallTimeout)
	defer cancel()
	var res *mcp.CallToolResult
	err := c.withSession(ctx, func(session *mcp.ClientSession) error {
		r, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		res = r
		return err
	})
	if err != nil {
		return nil, err
	}
	return decodeToolResult(res)
}

// ListToolDefinitions fetches tool schemas from the MCP server via the real MCP tools/list
// call, converted to the same {name, description, parameters} shape as the static
// definitions in provider.go so the two can be merged for the LLM.
func (c *MCPClient) ListToolDefinitions(ctx context.Context) ([]map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, mcpCallTimeout)
	defer cancel()
	var out []map[string]any
	err := c.withSession(ctx, func(session *mcp.ClientSession) error {
		out = nil
		for t, iterErr := range session.Tools(ctx, nil) {
			if iterErr != nil {
				return iterErr
			}
			out = append(out, map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.InputSchema,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func decodeToolResult(res *mcp.CallToolResult) (map[string]any, error) {
	if res == nil {
		return map[string]any{}, nil
	}
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			var out map[string]any
			if err := json.Unmarshal([]byte(tc.Text), &out); err == nil {
				return out, nil
			}
			return map[string]any{"result": tc.Text}, nil
		}
	}
	return map[string]any{}, nil
}
