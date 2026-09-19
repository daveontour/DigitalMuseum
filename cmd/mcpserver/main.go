// Command mcpserver is the MCP tools server: every Digital Museum AI tool (see
// internal/ai/tools.go) is exposed here over the MCP Streamable HTTP transport, on
// 127.0.0.1 only, gated by a shared-secret bearer token. It is spawned and managed by
// Electron (electron/main.js) as a peer process to the main Go server and the Ollama
// daemons. The main app defines no tool schemas of its own — it discovers this server's
// tool list via tools/list (internal/ai/tool_catalog.go). See CLAUDE.md for the full design.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/appctx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	sqlite_vec.Auto()

	sqlitePath := strings.TrimSpace(os.Getenv("SQLITE_PATH"))
	if sqlitePath == "" {
		fmt.Fprintln(os.Stderr, "mcpserver: SQLITE_PATH is required")
		os.Exit(1)
	}
	pepper := os.Getenv("KEYRING_PEPPER")
	authToken := strings.TrimSpace(os.Getenv("MCP_AUTH_TOKEN"))
	port := strings.TrimSpace(os.Getenv("HOST_PORT"))
	if port == "" {
		port = "8082"
	}

	// _journal_mode=WAL: the main server keeps its own long-lived connection to this same
	// file (internal/database/db.go, SetMaxOpenConns(1), no WAL pragma). Splitting tool
	// execution into a second process means two processes now touch the file concurrently;
	// WAL mode (a property of the file itself, not of either connection pool) is what makes
	// that safe instead of risking "database is locked" errors under the default rollback
	// journal.
	dsn := "file:" + sqlitePath + "?_foreign_keys=1&_busy_timeout=5000&_journal_mode=WAL"
	pool, err := sql.Open("sqlite3", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mcpserver: open sqlite: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = pool.Close() }()
	pool.SetMaxOpenConns(4)
	if err := pool.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "mcpserver: ping sqlite: %v\n", err)
		os.Exit(1)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "digitalmuseum-tools", Version: "0.2.0"}, nil)
	registerTools(server, pool, pepper)

	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/mcp", requireAuth(authToken, mcpHandler))

	// 127.0.0.1 only, never 0.0.0.0 — this server decrypts sensitive documents given a
	// caller-supplied master password argument, so it must be unreachable from the network.
	addr := "127.0.0.1:" + port
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	slog.Info("mcp tools server listening", "addr", addr)
	if err := httpServer.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "mcpserver: listen: %v\n", err)
		os.Exit(1)
	}
}

// requireAuth rejects any request without the shared-secret bearer token Electron generated
// at launch and injected into both this process and the main server's environment. Without
// this, any other local process could call tools — including the sensitive-document tool,
// which accepts a master password as a call argument.
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

// registerTools wires every tool. Query/decrypt logic lives in internal/ai/tools.go and is
// called directly through its exported wrappers — this file only adapts MCP's wire format to
// it and reconstructs the caller's user_id / visitor-access context that a single in-process
// call used to get from Go's request context automatically (see withCallerContext).
func registerTools(server *mcp.Server, pool *sql.DB, pepper string) {
	server.AddTool(&mcp.Tool{
		Name:        "get_current_time",
		Description: "Get the current date and time in ISO format. Useful when user asks about the current time or date.",
		InputSchema: objSchema(nil, nil),
	}, wrapTool(func(context.Context, map[string]any) (map[string]any, error) {
		return ai.GetCurrentTime()
	}))

	server.AddTool(&mcp.Tool{
		Name: "get_imessages_by_chat_session",
		Description: "Get all messages for WhatsApp, SMS, iMessage and Facebook messages for a specific chat. Use this " +
			"when the user asks about messages, conversations, or chats with a specific person or group.",
		InputSchema: objSchema(map[string]any{
			"chat_session": strProp("The chat session name (person or group name) to retrieve messages for"),
		}, []string{"chat_session"}),
	}, wrapTool(func(ctx context.Context, args map[string]any) (map[string]any, error) {
		chatSession, _ := args["chat_session"].(string)
		return ai.GetMessagesByChatSession(ctx, pool, chatSession)
	}))

	server.AddTool(&mcp.Tool{
		Name: "get_messages_around_in_chat",
		Description: "Return up to 20 messages before and after a specific message inside a chat session. Messages are " +
			"ordered chronologically. Use list_available_chat_sessions or get_imessages_by_chat_session to obtain " +
			"chat_session and message id values.",
		InputSchema: objSchema(map[string]any{
			"chat_session": strProp("Chat session name (same matching as get_imessages_by_chat_session)"),
			"message_id":   intProp("Database id of the anchor message (messages.id)"),
		}, []string{"chat_session", "message_id"}),
	}, wrapTool(func(ctx context.Context, args map[string]any) (map[string]any, error) {
		chatSession, _ := args["chat_session"].(string)
		return ai.GetMessagesAroundInChat(ctx, pool, chatSession, toInt64(args["message_id"]))
	}))

	server.AddTool(&mcp.Tool{
		Name: "list_available_chat_sessions",
		Description: "List distinct chat session names in the messages archive (WhatsApp, SMS, iMessage, Facebook " +
			"chats). Use before get_imessages_by_chat_session when you need valid session names or the user is unsure " +
			"of the exact spelling.",
		InputSchema: objSchema(nil, nil),
	}, wrapTool(func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		return ai.ListAvailableChatSessions(ctx, pool)
	}))

	server.AddTool(&mcp.Tool{
		Name: "search_chat_messages_globally",
		Description: "Search message body and subject across all chat sessions (WhatsApp, SMS, iMessage, Facebook) " +
			"with a case-insensitive keyword. Returns up to 200 matches; each has chat_session_id, message_id, and content.",
		InputSchema: objSchema(map[string]any{
			"keyword": strProp("Text to find within message text or subject (partial match)"),
		}, []string{"keyword"}),
	}, wrapTool(func(ctx context.Context, args map[string]any) (map[string]any, error) {
		keyword, _ := args["keyword"].(string)
		return ai.SearchChatMessagesGlobally(ctx, pool, keyword)
	}))

	server.AddTool(&mcp.Tool{
		Name: "search_chat_messages_in_session",
		Description: "Search message body and subject within a single chat session (same chat_session matching as " +
			"get_imessages_by_chat_session). Returns chat_session_id, message_id, and content per match (up to 200).",
		InputSchema: objSchema(map[string]any{
			"chat_session": strProp("Chat session name to restrict the search"),
			"keyword":      strProp("Text to find within message text or subject (partial match)"),
		}, []string{"chat_session", "keyword"}),
	}, wrapTool(func(ctx context.Context, args map[string]any) (map[string]any, error) {
		chatSession, _ := args["chat_session"].(string)
		keyword, _ := args["keyword"].(string)
		return ai.SearchChatMessagesInSession(ctx, pool, chatSession, keyword)
	}))

	server.AddTool(&mcp.Tool{
		Name: "search_messages_by_similarity",
		Description: "Vector similarity search over archived messages using message_embeddings. Accepts free text, " +
			"embeds it, and returns the top 20 closest messages.",
		InputSchema: objSchema(map[string]any{
			"text": strProp("Natural language query text to embed and search against messages"),
		}, []string{"text"}),
	}, wrapTool(func(ctx context.Context, args map[string]any) (map[string]any, error) {
		text, _ := args["text"].(string)
		return ai.SearchMessagesBySimilarity(ctx, pool, text)
	}))

	server.AddTool(&mcp.Tool{
		Name: "search_emails_by_similarity",
		Description: "Vector similarity search over archived emails using sqlite-vec email_embeddings. Accepts free " +
			"text, embeds it, and returns the top 20 closest emails.",
		InputSchema: objSchema(map[string]any{
			"text": strProp("Natural language query text to embed and search against emails"),
		}, []string{"text"}),
	}, wrapTool(func(ctx context.Context, args map[string]any) (map[string]any, error) {
		text, _ := args["text"].(string)
		return ai.SearchEmailsBySimilarity(ctx, pool, text)
	}))

	server.AddTool(&mcp.Tool{
		Name:        "get_emails_by_contact",
		Description: "Get plain text of emails where the sender or receiver matches the specified name or email address.",
		InputSchema: objSchema(map[string]any{
			"name": strProp("The name or email address to search for in sender or receiver fields"),
		}, []string{"name"}),
	}, wrapTool(func(ctx context.Context, args map[string]any) (map[string]any, error) {
		name, _ := args["name"].(string)
		return ai.GetEmailsByContact(ctx, pool, name)
	}))

	server.AddTool(&mcp.Tool{
		Name:        "get_all_messages_by_contact",
		Description: "Get all messages and emails for a specific contact. Use this when background information is needed on that person.",
		InputSchema: objSchema(map[string]any{
			"name": strProp("The name or email address to search"),
		}, []string{"name"}),
	}, wrapTool(func(ctx context.Context, args map[string]any) (map[string]any, error) {
		name, _ := args["name"].(string)
		return ai.GetAllMessagesByContact(ctx, pool, name)
	}))

	server.AddTool(&mcp.Tool{
		Name: "find_photos_of_person",
		Description: "Find photos containing a specific named person, using face recognition results linked to a Contact. " +
			"Accepts a name (matched against contacts.name and alternative_names) and returns the matching photos' " +
			"media_item_id, title, and date. Only returns results for people whose detected face(s) have been linked " +
			"to a Contact by the archive owner — a person not yet named this way will return no results.",
		InputSchema: objSchema(map[string]any{
			"name": strProp("Name of the person to search photos for (matches a Contact)"),
		}, []string{"name"}),
	}, wrapTool(func(ctx context.Context, args map[string]any) (map[string]any, error) {
		name, _ := args["name"].(string)
		return ai.FindPhotosOfPerson(ctx, pool, name)
	}))

	server.AddTool(&mcp.Tool{
		Name:        "get_subject_writing_examples",
		Description: "Get the subject's writing examples from their messages.",
		InputSchema: objSchema(nil, nil),
	}, wrapTool(func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		return ai.GetSubjectWritingExamples(ctx, pool)
	}))

	server.AddTool(&mcp.Tool{
		Name:        "search_tavily",
		Description: "Perform a web search for real-time information and current events using Tavily.",
		InputSchema: objSchema(map[string]any{
			"query": strProp("The search query"),
		}, []string{"query"}),
	}, wrapTool(func(_ context.Context, args map[string]any) (map[string]any, error) {
		query, _ := args["query"].(string)
		tavilyKey, _ := args[ai.ArgTavilyKey].(string) // never logged
		return ai.SearchTavily(tavilyKey, query)
	}))

	server.AddTool(&mcp.Tool{
		Name:        "search_facebook_albums",
		Description: "Search Facebook photo albums by a partial keyword match against album name or description.",
		InputSchema: objSchema(map[string]any{
			"keyword": strProp("Partial keyword to search for in album names and descriptions"),
		}, []string{"keyword"}),
	}, wrapTool(func(ctx context.Context, args map[string]any) (map[string]any, error) {
		keyword, _ := args["keyword"].(string)
		return ai.SearchFacebookAlbums(ctx, pool, keyword)
	}))

	server.AddTool(&mcp.Tool{
		Name: "get_album_images",
		Description: "Get the first 5 images in a Facebook album by album_id. Returns id, title, year, and month for " +
			"each image so you can choose the most relevant one to display alongside the memory. Call this after " +
			"search_facebook_albums when you want to show a photo.",
		InputSchema: objSchema(map[string]any{
			"album_id": intProp("The numeric album ID returned by search_facebook_albums"),
		}, []string{"album_id"}),
	}, wrapTool(func(ctx context.Context, args map[string]any) (map[string]any, error) {
		return ai.GetAlbumImages(ctx, pool, toInt64(args["album_id"]))
	}))

	server.AddTool(&mcp.Tool{
		Name:        "search_facebook_posts",
		Description: "Search Facebook posts where the post description partially matches the input.",
		InputSchema: objSchema(map[string]any{
			"description": strProp("Partial text to search for within Facebook post descriptions"),
		}, []string{"description"}),
	}, wrapTool(func(ctx context.Context, args map[string]any) (map[string]any, error) {
		description, _ := args["description"].(string)
		return ai.SearchFacebookPosts(ctx, pool, description)
	}))

	server.AddTool(&mcp.Tool{
		Name:        "get_all_facebook_posts",
		Description: "Retrieve the complete set of all Facebook posts (most recent first, up to 500). Use search_facebook_posts instead when the user gave a topic or keyword to narrow down.",
		InputSchema: objSchema(nil, nil),
	}, wrapTool(func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		return ai.GetAllFacebookPosts(ctx, pool)
	}))

	server.AddTool(&mcp.Tool{
		Name:        "get_unique_tags_count",
		Description: "Get the unique tags used in the media items library and artefacts collection, along with counts.",
		InputSchema: objSchema(nil, nil),
	}, wrapTool(func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		return ai.GetUniqueTagsCount(ctx, pool)
	}))

	server.AddTool(&mcp.Tool{
		Name:        "get_user_interests",
		Description: "Get the user's list of interests.",
		InputSchema: objSchema(nil, nil),
	}, wrapTool(func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		return ai.GetUserInterests(ctx, pool)
	}))

	server.AddTool(&mcp.Tool{
		Name:        "get_reference_document",
		Description: "Retrieve the full content of one or more reference documents by their IDs.",
		InputSchema: objSchema(map[string]any{
			"document_ids": idArrayProp("List of reference document IDs to retrieve"),
		}, []string{"document_ids"}),
	}, wrapTool(func(ctx context.Context, args map[string]any) (map[string]any, error) {
		ids := toInt64Slice(args["document_ids"])
		masterPassword, _ := args[ai.ArgMasterPassword].(string) // never logged
		return ai.GetReferenceDocumentsWithPassword(ctx, pool, ids, pepper, masterPassword)
	}))

	server.AddTool(&mcp.Tool{
		Name: "get_available_reference_documents",
		Description: "Get the title, description, id and tags of reference documents that are available for task and " +
			"not marked 'include in system prompt' (those are injected automatically and omitted here).",
		InputSchema: objSchema(nil, nil),
	}, wrapTool(func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		return ai.GetAvailableReferenceDocuments(ctx, pool)
	}))

	server.AddTool(&mcp.Tool{
		Name: "get_sensitive_reference_document",
		Description: "Retrieve full content of sensitive or private reference documents by ID (encrypted records need " +
			"the master key unlocked in this browser session). The archive owner sees every sensitive record; " +
			"visitor-key sessions only see IDs allowlisted for that key and only when sensitive/private archive " +
			"access is enabled for that key.",
		InputSchema: objSchema(map[string]any{
			"document_ids": idArrayProp("List of sensitive reference document IDs to retrieve"),
		}, []string{"document_ids"}),
	}, wrapTool(func(ctx context.Context, args map[string]any) (map[string]any, error) {
		ids := toInt64Slice(args["document_ids"])
		masterPassword, _ := args[ai.ArgMasterPassword].(string) // never logged
		return ai.GetSensitiveReferenceDocumentsWithPassword(ctx, pool, ids, pepper, masterPassword)
	}))

	server.AddTool(&mcp.Tool{
		Name: "get_available_sensitive_reference_documents",
		Description: "List sensitive/private reference documents (id, title, description, tags) the current session " +
			"may use with get_sensitive_reference_document, excluding rows marked 'include in system prompt' (those " +
			"are injected automatically). The archive owner gets the full set; visitor keys need sensitive/private " +
			"access and only see allowlisted documents.",
		InputSchema: objSchema(nil, nil),
	}, wrapTool(func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		return ai.GetAvailableSensitiveReferenceDocuments(ctx, pool)
	}))

	server.AddTool(&mcp.Tool{
		Name: "list_interviews",
		Description: "List all interviews that have been conducted with the subject, including their style, purpose, " +
			"state, turn count, and whether a writeup has been generated.",
		InputSchema: objSchema(map[string]any{
			"state": strProp("Optional filter: 'active', 'paused', or 'finished'. Omit for all."),
		}, []string{}),
	}, wrapTool(func(ctx context.Context, args map[string]any) (map[string]any, error) {
		stateFilter, _ := args["state"].(string)
		return ai.ListInterviews(ctx, pool, stateFilter)
	}))

	server.AddTool(&mcp.Tool{
		Name:        "get_interview",
		Description: "Get the full details of a specific interview including the questions, answers, and the final writeup if available.",
		InputSchema: objSchema(map[string]any{
			"interview_id": intProp("The ID of the interview to retrieve"),
		}, []string{"interview_id"}),
	}, wrapTool(func(ctx context.Context, args map[string]any) (map[string]any, error) {
		return ai.GetInterview(ctx, pool, toInt64(args["interview_id"]))
	}))

	server.AddTool(&mcp.Tool{
		Name: "list_complete_profiles",
		Description: "List all relationship complete profiles stored for this archive: each entry is a contact name " +
			"plus whether generation is still in progress. Use get_complete_profile to read the full text for a name. " +
			"Visitor keys need relationships (not only sensitive/private) access.",
		InputSchema: objSchema(nil, nil),
	}, wrapTool(func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		return ai.ListCompleteProfiles(ctx, pool)
	}))

	server.AddTool(&mcp.Tool{
		Name: "get_complete_profile",
		Description: "Retrieve the generated relationship/psychological profile text for a specific person by exact " +
			"name (as shown in list_complete_profiles). If pending is true, the profile is not ready yet. Visitor " +
			"keys need relationships archive access.",
		InputSchema: objSchema(map[string]any{
			"name": strProp("Contact name matching a complete profile row"),
		}, []string{"name"}),
	}, wrapTool(func(ctx context.Context, args map[string]any) (map[string]any, error) {
		name, _ := args["name"].(string)
		return ai.GetCompleteProfile(ctx, pool, name)
	}))
}

// --- JSON Schema helpers, to keep the registrations above readable ---------------------------

func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}
func intProp(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}
func idArrayProp(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": desc}
}

func objSchema(properties map[string]any, required []string) map[string]any {
	if properties == nil {
		properties = map[string]any{}
	}
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": properties, "required": required}
}

// wrapTool adapts a (ctx, args map[string]any) tool function — matching the shape of the main
// app's ToolExecutor — to the MCP SDK's low-level ToolHandler: decodes raw wire arguments,
// reconstructs the caller's context from the hidden fields NewMCPToolExecutor attaches on every
// call (see withCallerContext), and encodes the result back as MCP text content.
func wrapTool(fn func(ctx context.Context, args map[string]any) (map[string]any, error)) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := map[string]any{}
		if len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return errorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
			}
		}
		ctx = withCallerContext(ctx, args)
		result, err := fn(ctx, args)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		return jsonResult(result), nil
	}
}

// withCallerContext reconstructs appctx.UserID / appctx.VisitorAccess from the hidden args
// NewMCPToolExecutor (internal/ai/tools.go) attaches to every call, so every tool implementation
// (unchanged from before the migration) sees the same ctx-derived scoping it always did.
func withCallerContext(ctx context.Context, args map[string]any) context.Context {
	ctx = context.WithValue(ctx, appctx.ContextKeyUserID, toInt64(args[ai.ArgUID]))
	va := appctx.VisitorAccess{
		Restricted:          boolArg(args[ai.ArgVisitorRestricted]),
		CanMessagesChat:     boolArg(args[ai.ArgVisitorCanMessagesChat]),
		CanEmails:           boolArg(args[ai.ArgVisitorCanEmails]),
		CanContacts:         boolArg(args[ai.ArgVisitorCanContacts]),
		CanRelationships:    boolArg(args[ai.ArgVisitorCanRelationships]),
		CanSensitivePrivate: boolArg(args[ai.ArgVisitorCanSensitivePriv]),
	}
	return context.WithValue(ctx, appctx.ContextKeyVisitorAccess, va)
}

func jsonResult(v map[string]any) *mcp.CallToolResult {
	b, err := json.Marshal(v)
	if err != nil {
		return errorResult(err.Error())
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}
}

func errorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int64:
		return x
	case json.Number:
		n, _ := x.Int64()
		return n
	default:
		return 0
	}
}

func boolArg(v any) bool {
	b, _ := v.(bool)
	return b
}

func toInt64Slice(v any) []int64 {
	arr, _ := v.([]any)
	out := make([]int64, 0, len(arr))
	for _, e := range arr {
		out = append(out, toInt64(e))
	}
	return out
}
