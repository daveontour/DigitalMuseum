package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"

	appai "github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
)

// MCPServer is the in-memory representation of one admin-managed MCP server row — either the
// bundled cmd/mcpserver (IsBuiltin true, exactly one such row) or a user-added additional server.
type MCPServer struct {
	ID          int64
	Name        string
	EndpointURL string
	AuthToken   string
	Enabled     bool
	IsBuiltin   bool
	SortOrder   int
	Scope       string
}

// MCPServerInput holds the editable fields for one MCP server. Name/EndpointURL/AuthToken are
// ignored when applied to the builtin row — see Update.
//
// AuthToken is a pointer because the API never echoes the stored token back (mcpServerJSON only
// exposes auth_token_set), so the edit form can't "resend what's already there" the way it does
// for Name/EndpointURL: nil (the field omitted, or JSON null) means "leave the stored token
// unchanged" on Update; Create has no existing token to preserve, so nil there just means "no
// token." A present-but-empty string is a deliberate clear.
type MCPServerInput struct {
	Name        string  `json:"name"`
	EndpointURL string  `json:"endpoint_url"`
	AuthToken   *string `json:"auth_token"`
	Enabled     bool    `json:"enabled"`
	SortOrder   int     `json:"sort_order"`
}

// MCPServersService manages the deployment-wide, admin-editable list of MCP servers the app
// discovers AI tools from (the bundled server plus any additional ones). Reads are cached
// in-process (invalidated on every write), and every load/write pushes the current server list
// into ai.DefaultMCPRegistry() so the tool-calling executor and catalog always see the latest
// enabled/disabled state and credentials without needing their own DB access.
type MCPServersService struct {
	repo *repository.MCPServersRepo

	mu    sync.RWMutex
	cache []MCPServer // nil = not loaded
}

// NewMCPServersService creates an MCPServersService.
func NewMCPServersService(repo *repository.MCPServersRepo) *MCPServersService {
	return &MCPServersService{repo: repo}
}

func toMCPServer(row *model.MCPServerRow) MCPServer {
	return MCPServer{
		ID:          row.ID,
		Name:        row.Name,
		EndpointURL: row.EndpointURL,
		AuthToken:   row.AuthToken,
		Enabled:     row.Enabled,
		IsBuiltin:   row.IsBuiltin,
		SortOrder:   row.SortOrder,
		Scope:       row.Scope,
	}
}

// syncRegistry pushes the given server list into the ai package's registry, translated to the
// shape it expects. Called after every load and write so the registry never drifts from the DB.
func syncRegistry(servers []MCPServer) {
	rows := make([]appai.MCPRegistryServer, 0, len(servers))
	for _, s := range servers {
		rows = append(rows, appai.MCPRegistryServer{
			Name:        s.Name,
			EndpointURL: s.EndpointURL,
			AuthToken:   s.AuthToken,
			Enabled:     s.Enabled,
			IsBuiltin:   s.IsBuiltin,
			Scope:       s.Scope,
		})
	}
	appai.DefaultMCPRegistry().SyncServers(rows)
}

func (s *MCPServersService) refreshCache(ctx context.Context) ([]MCPServer, error) {
	rows, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]MCPServer, 0, len(rows))
	for _, row := range rows {
		out = append(out, toMCPServer(row))
	}
	s.mu.Lock()
	s.cache = out
	s.mu.Unlock()
	syncRegistry(out)
	return out, nil
}

func (s *MCPServersService) invalidate(ctx context.Context) {
	s.mu.Lock()
	s.cache = nil
	s.mu.Unlock()
	// Reload immediately (rather than lazily on next read) so the registry — and therefore the
	// next tool call — reflects this write right away.
	_, _ = s.refreshCache(ctx)
	// Also re-discover tools in the background, so a newly-added (or newly-enabled/disabled)
	// server's effect on the catalog shows up without the admin needing a separate manual
	// "Refresh Tools" click. Async and best-effort: the HTTP response for this write shouldn't
	// wait on a tools/list round-trip to a possibly slow/unreachable server.
	go func() {
		_ = appai.DefaultToolCatalog().Refresh(context.Background())
	}()
}

// ListAll returns every server (admin table view), including the builtin row, always freshly
// read from the database so admin edits made elsewhere are reflected immediately.
func (s *MCPServersService) ListAll(ctx context.Context) ([]MCPServer, error) {
	return s.refreshCache(ctx)
}

// GetByID returns one server row (nil if missing), for the Test-connection endpoint.
func (s *MCPServersService) GetByID(ctx context.Context, id int64) (*MCPServer, error) {
	row, err := s.repo.GetByID(ctx, id)
	if err != nil || row == nil {
		return nil, err
	}
	m := toMCPServer(row)
	return &m, nil
}

// EnsureRegistryLoaded pushes the current server list into the registry without forcing a
// fresh DB read if already cached — called once at startup before the first tool discovery
// attempt (see cmd/server/main.go), so the registry knows about additional servers before
// ToolCatalog.RefreshWithRetry runs.
func (s *MCPServersService) EnsureRegistryLoaded(ctx context.Context) {
	s.mu.RLock()
	cached := s.cache
	s.mu.RUnlock()
	if cached != nil {
		syncRegistry(cached)
		return
	}
	_, _ = s.refreshCache(ctx)
}

// Create inserts a new additional MCP server (never builtin — that row is seeded once by
// database.SeedBuiltinMCPServerIfMissing and never created through this API).
func (s *MCPServersService) Create(ctx context.Context, in MCPServerInput) (*MCPServer, error) {
	name, endpointURL, err := validateMCPServerInput(in)
	if err != nil {
		return nil, err
	}
	conflict, err := s.repo.NameExistsExcluding(ctx, name, 0)
	if err != nil {
		return nil, err
	}
	if conflict {
		return nil, fmt.Errorf("conflict:an MCP server named %q already exists", name)
	}
	authToken := ""
	if in.AuthToken != nil {
		authToken = strings.TrimSpace(*in.AuthToken)
	}
	row, err := s.repo.Create(ctx, name, endpointURL, authToken, in.Enabled, in.SortOrder)
	if err != nil {
		return nil, err
	}
	s.invalidate(ctx)
	m := toMCPServer(row)
	return &m, nil
}

// Update replaces an existing MCP server row. For the builtin row, the submitted
// name/endpoint_url/auth_token are ignored — only enabled/sort_order apply — since its real
// connection is Electron-managed (MCP_SERVER_URL/MCP_AUTH_TOKEN), not these DB columns.
func (s *MCPServersService) Update(ctx context.Context, id int64, in MCPServerInput) (*MCPServer, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}

	name, endpointURL, authToken := existing.Name, existing.EndpointURL, existing.AuthToken
	if !existing.IsBuiltin {
		var verr error
		name, endpointURL, verr = validateMCPServerInput(in)
		if verr != nil {
			return nil, verr
		}
		conflict, cerr := s.repo.NameExistsExcluding(ctx, name, id)
		if cerr != nil {
			return nil, cerr
		}
		if conflict {
			return nil, fmt.Errorf("conflict:an MCP server named %q already exists", name)
		}
		if in.AuthToken != nil {
			authToken = strings.TrimSpace(*in.AuthToken)
		}
	}

	row, err := s.repo.Update(ctx, id, name, endpointURL, authToken, in.Enabled, in.SortOrder)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	s.invalidate(ctx)
	m := toMCPServer(row)
	return &m, nil
}

// Delete removes an MCP server row. The builtin row can never be deleted — only disabled —
// since there's no reinstall/re-add flow for it.
func (s *MCPServersService) Delete(ctx context.Context, id int64) error {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if existing != nil && existing.IsBuiltin {
		return fmt.Errorf("conflict:the built-in Digital Museum server cannot be deleted — disable it instead")
	}
	_, err = s.repo.Delete(ctx, id)
	if err != nil {
		return err
	}
	s.invalidate(ctx)
	return nil
}

// validateMCPServerInput validates the name/endpoint of Create/Update input for a non-builtin
// server. Auth token resolution ("leave unchanged" vs "set"/"clear") is Create/Update-specific —
// see MCPServerInput's doc comment — so it's handled by the caller, not here.
func validateMCPServerInput(in MCPServerInput) (name, endpointURL string, err error) {
	name = strings.TrimSpace(in.Name)
	if name == "" {
		return "", "", fmt.Errorf("name is required")
	}
	if strings.Contains(name, ":") {
		return "", "", fmt.Errorf("name must not contain ':' — it's used as this server's tool-name prefix")
	}
	endpointURL = strings.TrimSpace(in.EndpointURL)
	u, perr := url.Parse(endpointURL)
	if perr != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", "", fmt.Errorf("endpoint_url must be a valid http(s) URL")
	}
	return name, endpointURL, nil
}
