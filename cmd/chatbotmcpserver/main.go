// Command chatbotmcpserver is the ChatBot feature's dedicated MCP tools server: a small set of
// general-purpose utility tools (time, arithmetic, unit conversion, text helpers) available only
// to the ChatBot chat modality, never to the persona chat. It is a separate process and binary
// from cmd/mcpserver (the archive-data tools server) so the two tool sets stay hard-isolated —
// see internal/ai/mcp_registry.go's scope model (ScopeArchive vs ScopeChatbot vs ScopeShared)
// and CLAUDE.md's "MCP Tools Server & AI Tool Discovery" section.
//
// Deliberately dependency-light: no database access, no import of internal/ai or go-sqlite3, so
// this binary needs no CGO at all (see the Makefile's build-chatbot-mcp-exe target and its
// CGO_ENABLED=0) — every tool here is pure computation over its own call arguments.
package main

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	authToken := strings.TrimSpace(os.Getenv("MCP_AUTH_TOKEN"))
	port := strings.TrimSpace(os.Getenv("HOST_PORT"))
	if port == "" {
		port = "8083"
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "digitalmuseum-chatbot-tools", Version: "0.1.0"}, nil)
	registerTools(server)

	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/mcp", requireAuth(authToken, mcpHandler))

	// 127.0.0.1 only, same rationale as cmd/mcpserver — these tools carry no secrets, but there's
	// no reason to expose the port beyond loopback either.
	addr := "127.0.0.1:" + port
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	slog.Info("chatbot mcp tools server listening", "addr", addr)
	if err := httpServer.ListenAndServe(); err != nil {
		os.Exit(1)
	}
}

// requireAuth rejects any request without the shared-secret bearer token Electron generated at
// launch and injected into every managed child process — mirrors cmd/mcpserver's requireAuth.
func requireAuth(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		if token == "" || r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
