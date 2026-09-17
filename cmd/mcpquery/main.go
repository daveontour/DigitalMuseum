// Command mcpquery is a small dev CLI for talking directly to the MCP tools server
// (cmd/mcpserver) — list what tools it currently exposes, or call one with arbitrary
// arguments. It uses the exact same client (internal/ai.MCPClient) the main app uses, so
// what you see here is what the app would see.
//
// Usage:
//
//	mcpquery list
//	mcpquery call <tool_name> [-args '{"key":"value"}'] [-uid N] [-master-password PW]
//	            [-tavily-key KEY] [-visitor-restricted] [-visitor-can-relationships]
//	            [-visitor-can-sensitive]
//
// Env:
//
//	MCP_SERVER_URL   MCP endpoint, e.g. http://127.0.0.1:8082/mcp (default shown below)
//	MCP_AUTH_TOKEN   bearer token the target MCP server requires
//
// Example — run cmd/mcpserver standalone, then query it:
//
//	SQLITE_PATH=archive.sqlite MCP_AUTH_TOKEN=devtoken HOST_PORT=8082 ./bin/digitalmuseum-mcp.exe &
//	MCP_SERVER_URL=http://127.0.0.1:8082/mcp MCP_AUTH_TOKEN=devtoken ./bin/mcpquery list
//	MCP_SERVER_URL=http://127.0.0.1:8082/mcp MCP_AUTH_TOKEN=devtoken \
//	    ./bin/mcpquery call search_chat_messages_globally -args '{"keyword":"hello"}' -uid 2
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/daveontour/aimuseum/internal/ai"
	_ "github.com/mattn/go-sqlite3" // internal/ai transitively links sqlite-vec's cgo bindings, which need this
)

const defaultEndpoint = "http://127.0.0.1:8082/mcp"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	endpoint := strings.TrimSpace(os.Getenv("MCP_SERVER_URL"))
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	token := os.Getenv("MCP_AUTH_TOKEN")
	client := ai.NewMCPClient(endpoint, token)
	ctx := context.Background()

	switch os.Args[1] {
	case "list":
		runList(ctx, client)
	case "call":
		runCall(ctx, client, os.Args[2:])
	case "-h", "-help", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `mcpquery — query the MCP tools server directly (dev tool)

Usage:
  mcpquery list
  mcpquery call <tool_name> [-args '{"key":"value"}'] [-uid N] [-master-password PW]
              [-tavily-key KEY] [-visitor-restricted] [-visitor-can-relationships]
              [-visitor-can-sensitive]

Env:
  MCP_SERVER_URL   MCP endpoint (default `+defaultEndpoint+`)
  MCP_AUTH_TOKEN   bearer token the target MCP server requires`)
}

func runList(ctx context.Context, client *ai.MCPClient) {
	defs, err := client.ListToolDefinitions(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Printf("%d tool(s):\n\n", len(defs))
	for _, d := range defs {
		name, _ := d["name"].(string)
		desc, _ := d["description"].(string)
		fmt.Printf("- %s\n    %s\n", name, desc)
		if params, err := json.MarshalIndent(d["parameters"], "    ", "  "); err == nil {
			fmt.Printf("    %s\n", params)
		}
	}
}

func runCall(ctx context.Context, client *ai.MCPClient, rest []string) {
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "call requires a tool name, e.g. mcpquery call get_current_time")
		os.Exit(2)
	}
	toolName := rest[0]

	fs := flag.NewFlagSet("call", flag.ExitOnError)
	argsJSON := fs.String("args", "{}", "tool arguments as a JSON object (what the LLM would send)")
	uid := fs.Int64("uid", 0, "attach as "+ai.ArgUID+" (simulates the authenticated user id; 0 = unauthenticated/legacy)")
	masterPassword := fs.String("master-password", "", "attach as "+ai.ArgMasterPassword+" (needed by get_reference_document / get_sensitive_reference_document)")
	tavilyKey := fs.String("tavily-key", "", "attach as "+ai.ArgTavilyKey+" (needed by search_tavily)")
	visitorRestricted := fs.Bool("visitor-restricted", false, "attach "+ai.ArgVisitorRestricted+"=true (simulates a restricted visitor-key session)")
	visitorCanRelationships := fs.Bool("visitor-can-relationships", false, "attach "+ai.ArgVisitorCanRelationships+"=true")
	visitorCanSensitive := fs.Bool("visitor-can-sensitive", false, "attach "+ai.ArgVisitorCanSensitivePriv+"=true")
	if err := fs.Parse(rest[1:]); err != nil {
		os.Exit(2)
	}

	var args map[string]any
	if err := json.Unmarshal([]byte(*argsJSON), &args); err != nil {
		fmt.Fprintln(os.Stderr, "invalid -args JSON:", err)
		os.Exit(2)
	}
	if args == nil {
		args = map[string]any{}
	}
	args[ai.ArgUID] = *uid
	args[ai.ArgVisitorRestricted] = *visitorRestricted
	args[ai.ArgVisitorCanRelationships] = *visitorCanRelationships
	args[ai.ArgVisitorCanSensitivePriv] = *visitorCanSensitive
	if *masterPassword != "" {
		args[ai.ArgMasterPassword] = *masterPassword
	}
	if *tavilyKey != "" {
		args[ai.ArgTavilyKey] = *tavilyKey
	}

	result, err := client.CallTool(ctx, toolName, args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "error encoding result:", err)
		os.Exit(1)
	}
	fmt.Println(string(out))
}
