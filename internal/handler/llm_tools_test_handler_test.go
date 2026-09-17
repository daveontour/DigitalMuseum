package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appai "github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/keystore"
	_ "github.com/mattn/go-sqlite3"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// seedTestToolCatalog seeds the tool catalog (normally populated by discovering the real MCP
// server, see internal/ai/tool_catalog.go) with just what these tests need, so
// validateToolTestNames/knownChatToolNames recognize "get_current_time" without a live server.
func seedTestToolCatalog(t *testing.T) {
	t.Helper()
	appai.SeedCatalogForTests([]map[string]any{
		{
			"name":        "get_current_time",
			"description": "Get the current date and time in ISO format.",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}},
		},
	})
}

// newFakeMCPServer spins up a minimal MCP server (get_current_time only) over HTTP for tests
// that actually need to exercise a tool call end-to-end, and points DefaultMCPClient at it.
func newFakeMCPServer(t *testing.T) {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "fake-mcp-test", Version: "0.0.0"}, nil)
	server.AddTool(&mcp.Tool{
		Name:        "get_current_time",
		Description: "test",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}},
	}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, _ := appai.GetCurrentTime()
		b, _ := json.Marshal(result)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	appai.SetDefaultMCPClientForTests(appai.NewMCPClient(ts.URL+"/mcp", ""))
}

func TestValidateToolTestNames(t *testing.T) {
	seedTestToolCatalog(t)
	invalid := validateToolTestNames([]string{"get_current_time", "not_a_real_tool"})
	if len(invalid) != 1 || invalid[0] != "not_a_real_tool" {
		t.Fatalf("validateToolTestNames() = %v; want [not_a_real_tool]", invalid)
	}
	if got := validateToolTestNames([]string{"get_current_time"}); len(got) != 0 {
		t.Fatalf("expected no invalid names, got %v", got)
	}
}

func TestTruncateToolTestResult(t *testing.T) {
	small := map[string]any{"ok": true}
	out, truncated := truncateToolTestResult(small)
	if truncated || out["ok"] != true {
		t.Fatalf("small result should not truncate: %+v truncated=%v", out, truncated)
	}

	largeText := strings.Repeat("x", toolTestResultMaxBytes+100)
	large := map[string]any{"data": largeText}
	out, truncated = truncateToolTestResult(large)
	if !truncated {
		t.Fatal("expected truncation for large result")
	}
	if out["truncated"] != true {
		t.Fatalf("expected truncated flag, got %+v", out)
	}
}

func TestLLMToolsTestHandler_TestTools_requiresMasterUnlock(t *testing.T) {
	store := keystore.NewSessionMasterStore(false)
	h := NewLLMToolsTestHandler(nil, store, nil, nil, "", nil)

	body := bytes.NewBufferString(`{"tools":[{"name":"get_current_time","arguments":{}}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/settings/llm-tools/test", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.TestTools(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d; want 403", w.Code)
	}
}

func testSQLitePool(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Skipf("sqlite unavailable: %v", err)
	}
	return db
}

func masterUnlockedRequest(t *testing.T, store *keystore.SessionMasterStore, method, target string, body *bytes.Buffer) *http.Request {
	t.Helper()
	setupReq := httptest.NewRequest(http.MethodPost, "/", nil)
	setupW := httptest.NewRecorder()
	if err := store.Put(setupW, setupReq, "secret", true); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, target, body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, c := range setupW.Result().Cookies() {
		req.AddCookie(c)
	}
	ctx := context.WithValue(req.Context(), appctx.ContextKeyUserID, int64(1))
	return req.WithContext(ctx)
}

func TestLLMToolsTestHandler_TestTools_rejectsUnknownTool(t *testing.T) {
	seedTestToolCatalog(t)
	store := keystore.NewSessionMasterStore(false)
	pool := testSQLitePool(t)
	defer func() { _ = pool.Close() }()
	h := NewLLMToolsTestHandler(pool, store, nil, nil, "", nil)

	body := bytes.NewBufferString(`{"tools":[{"name":"totally_fake_tool","arguments":{}}]}`)
	req := masterUnlockedRequest(t, store, http.MethodPost, "/api/settings/llm-tools/test", body)
	w := httptest.NewRecorder()

	h.TestTools(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s; want 400", w.Code, w.Body.String())
	}
}

func TestLLMToolsTestHandler_TestTools_getCurrentTime(t *testing.T) {
	seedTestToolCatalog(t)
	newFakeMCPServer(t)
	store := keystore.NewSessionMasterStore(false)
	pool := testSQLitePool(t)
	defer func() { _ = pool.Close() }()
	h := NewLLMToolsTestHandler(pool, store, nil, nil, "", nil)

	body := bytes.NewBufferString(`{"tools":[{"name":"get_current_time","arguments":{}}]}`)
	req := masterUnlockedRequest(t, store, http.MethodPost, "/api/settings/llm-tools/test", body)
	w := httptest.NewRecorder()

	h.TestTools(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s; want 200", w.Code, w.Body.String())
	}
	var resp struct {
		Results []struct {
			Name   string         `json:"name"`
			Result map[string]any `json:"result"`
			Error  any            `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("results len = %d; want 1", len(resp.Results))
	}
	if resp.Results[0].Name != "get_current_time" {
		t.Fatalf("name = %q", resp.Results[0].Name)
	}
	if resp.Results[0].Error != nil {
		t.Fatalf("error = %v", resp.Results[0].Error)
	}
	if resp.Results[0].Result["timezone"] != "UTC" {
		t.Fatalf("result = %+v", resp.Results[0].Result)
	}
}
