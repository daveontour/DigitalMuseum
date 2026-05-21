// Package ai provides chat provider implementations for Gemini, Claude, DeepSeek, and LocalAI.
package ai

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
)

// maxToolCallIterations caps assistant→tool→tool_result rounds per GenerateResponse
// (Claude and Gemini). PamBot and similar flows may chain many archive tools.
const maxToolCallIterations = 15

// ConvTurn is a previous conversation turn used to build context.
type ConvTurn struct {
	UserInput    string
	ResponseText string
}

// GenerateRequest holds all parameters for a single generation call.
type GenerateRequest struct {
	UserInput     string
	Temperature   float64
	Voice         string
	Mood          string
	CompanionMode bool
	WhosAsking    string
	SubjectName   string
	SubjectGender string
	PsychProfile  *string
	WritingStyle  *string
}

// LLMUsage summarises token usage for one completed generation (tool loop totals).
type LLMUsage struct {
	Provider     string
	Model        string
	InputTokens  int
	OutputTokens int
	// UsedServerKey is set by callers when known: true if the API key came from server env/config, false if user or visitor session override.
	UsedServerKey *bool `json:"used_server_key,omitempty"`
}

// GenerateResult is the output of a generation call.
type GenerateResult struct {
	PlainText    string // response text with embedded JSON blocks stripped
	MetadataJSON string // JSON string with tokens, function calls, etc.
	Voice        string
	Usage        *LLMUsage // set when the provider reports usage; nil if unavailable
}

// ToolExecutor executes a named AI tool and returns a JSON-serialisable result.
type ToolExecutor func(ctx context.Context, name string, args map[string]any) (map[string]any, error)

// ChatProvider is the interface implemented by Gemini, Claude, DeepSeek (Anthropic-compatible), and LocalAI.
type ChatProvider interface {
	IsAvailable() bool
	// GenerateResponse runs a tool loop. toolDecls nil means expose all built-in tools (legacy).
	// Non-nil: use exactly *toolDecls (may be empty — no tools sent to the model).
	// If executor is nil, tools are not offered to the model (safe for summarisation-only callers).
	GenerateResponse(
		ctx context.Context,
		req GenerateRequest,
		systemPrompt string,
		history []ConvTurn,
		executor ToolExecutor,
		toolDecls *[]map[string]any,
	) (GenerateResult, error)
}

// GetToolDefinitions returns the shared tool schema for callers that pass explicit toolDecls.
func GetToolDefinitions() []map[string]any {
	return toolDefinitions()
}

// toolDefinitions returns the shared tool schema used by both providers.
func toolDefinitions() []map[string]any {
	return []map[string]any{
		{
			"name":        "get_current_time",
			"description": "Get the current date and time in ISO format. Useful when user asks about the current time or date.",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}},
		},
		{
			"name":        "get_imessages_by_chat_session",
			"description": "Get all messages for WhatsApp, SMS, iMessage and Facebook messages for a specific chat. Use this when the user asks about messages, conversations, or chats with a specific person or group.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"chat_session": map[string]any{"type": "string", "description": "The chat session name (person or group name) to retrieve messages for"},
				},
				"required": []string{"chat_session"},
			},
		},

		{
			"name":        "get_messages_around_in_chat",
			"description": "Return up to 20 messages before and after a specific message inside a chat session. Messages are ordered chronologically. Use list_available_chat_sessions or get_imessages_by_chat_session to obtain chat_session and message id values.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"chat_session": map[string]any{"type": "string", "description": "Chat session name (same matching as get_imessages_by_chat_session)"},
					"message_id":   map[string]any{"type": "integer", "description": "Database id of the anchor message (messages.id)"},
				},
				"required": []string{"chat_session", "message_id"},
			},
		},
		{
			"name":        "list_available_chat_sessions",
			"description": "List distinct chat session names in the messages archive (WhatsApp, SMS, iMessage, Facebook chats). Use before get_imessages_by_chat_session when you need valid session names or the user is unsure of the exact spelling.",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}},
		},

		{
			"name":        "search_chat_messages_globally",
			"description": "Search message body and subject across all chat sessions (WhatsApp, SMS, iMessage, Facebook) with a case-insensitive keyword. Returns up to 200 matches; each has chat_session_id (session string from the archive), message_id, and content.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"keyword": map[string]any{"type": "string", "description": "Text to find within message text or subject (partial match)"},
				},
				"required": []string{"keyword"},
			},
		},
		{
			"name":        "search_chat_messages_in_session",
			"description": "Search message body and subject within a single chat session (same chat_session matching as get_imessages_by_chat_session). Returns chat_session_id, message_id, and content per match (up to 200).",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"chat_session": map[string]any{"type": "string", "description": "Chat session name to restrict the search"},
					"keyword":      map[string]any{"type": "string", "description": "Text to find within message text or subject (partial match)"},
				},
				"required": []string{"chat_session", "keyword"},
			},
		},
		{
			"name":        "search_messages_by_similarity",
			"description": "Vector similarity search over archived messages using embedding_vector. Accepts free text, embeds it, and returns the top 20 closest messages.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{"type": "string", "description": "Natural language query text to embed and search against messages"},
				},
				"required": []string{"text"},
			},
		},
		{
			"name":        "search_emails_by_similarity",
			"description": "Vector similarity search over archived emails using embedding_vector. Accepts free text, embeds it, and returns the top 20 closest emails.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{"type": "string", "description": "Natural language query text to embed and search against emails"},
				},
				"required": []string{"text"},
			},
		},
		{
			"name":        "get_emails_by_contact",
			"description": "Get plain text of emails where the sender or receiver matches the specified name or email address.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string", "description": "The name or email address to search for in sender or receiver fields"},
				},
				"required": []string{"name"},
			},
		},
		{
			"name":        "get_all_messages_by_contact",
			"description": "Get all messages and emails for a specific contact. Use this when background information is needed on that person.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string", "description": "The name or email address to search"},
				},
				"required": []string{"name"},
			},
		},
		{
			"name":        "get_subject_writing_examples",
			"description": "Get the subject's writing examples from their messages.",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}},
		},
		{
			"name":        "search_tavily",
			"description": "Perform a web search for real-time information and current events using Tavily.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "The search query"},
				},
				"required": []string{"query"},
			},
		},
		{
			"name":        "search_facebook_albums",
			"description": "Search Facebook photo albums by a partial keyword match against album name or description.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"keyword": map[string]any{"type": "string", "description": "Partial keyword to search for in album names and descriptions"},
				},
				"required": []string{"keyword"},
			},
		},
		{
			"name":        "get_album_images",
			"description": "Get the first 5 images in a Facebook album by album_id. Returns id, title, year, and month for each image so you can choose the most relevant one to display alongside the memory. Call this after search_facebook_albums when you want to show a photo.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"album_id": map[string]any{"type": "integer", "description": "The numeric album ID returned by search_facebook_albums"},
				},
				"required": []string{"album_id"},
			},
		},
		// {
		// 	"name":        "get_all_facebook_posts",
		// 	"description": "Retrieve the complete set of all Facebook posts.",
		// 	"parameters":  map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}},
		// },
		{
			"name":        "search_facebook_posts",
			"description": "Search Facebook posts where the post description partially matches the input.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"description": map[string]any{"type": "string", "description": "Partial text to search for within Facebook post descriptions"},
				},
				"required": []string{"description"},
			},
		},
		{
			"name":        "get_unique_tags_count",
			"description": "Get the unique tags used in the media items library and artefacts collection, along with counts.",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}},
		},
		{
			"name":        "get_user_interests",
			"description": "Get the user's list of interests.",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}},
		},
		{
			"name":        "get_reference_document",
			"description": "Retrieve the full content of one or more reference documents by their IDs.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"document_ids": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "integer"},
						"description": "List of reference document IDs to retrieve",
					},
				},
				"required": []string{"document_ids"},
			},
		},
		{
			"name":        "get_available_reference_documents",
			"description": "Get the title, description, id and tags of reference documents that are available for task and not marked 'include in system prompt' (those are injected automatically and omitted here).",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}},
		},
		{
			"name":        "get_sensitive_reference_document",
			"description": "Retrieve full content of sensitive or private reference documents by ID (encrypted records need the master key unlocked in this browser session). The archive owner sees every sensitive record; visitor-key sessions only see IDs allowlisted for that key and only when sensitive/private archive access is enabled for that key.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"document_ids": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "integer"},
						"description": "List of sensitive reference document IDs to retrieve",
					},
				},
				"required": []string{"document_ids"},
			},
		},
		{
			"name":        "get_available_sensitive_reference_documents",
			"description": "List sensitive/private reference documents (id, title, description, tags) the current session may use with get_sensitive_reference_document, excluding rows marked 'include in system prompt' (those are injected automatically). The archive owner gets the full set; visitor keys need sensitive/private access and only see allowlisted documents.",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}},
		},
		{
			"name":        "list_interviews",
			"description": "List all interviews that have been conducted with the subject, including their style, purpose, state, turn count, and whether a writeup has been generated.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"state": map[string]any{"type": "string", "description": "Optional filter: 'active', 'paused', or 'finished'. Omit for all."},
				},
				"required": []string{},
			},
		},
		{
			"name":        "get_interview",
			"description": "Get the full details of a specific interview including the questions, answers, and the final writeup if available.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"interview_id": map[string]any{"type": "integer", "description": "The ID of the interview to retrieve"},
				},
				"required": []string{"interview_id"},
			},
		},
		{
			"name":        "list_complete_profiles",
			"description": "List all relationship complete profiles stored for this archive: each entry is a contact name plus whether generation is still in progress. Use get_complete_profile to read the full text for a name. Visitor keys need relationships (not only sensitive/private) access.",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}},
		},
		{
			"name":        "get_complete_profile",
			"description": "Retrieve the generated relationship/psychological profile text for a specific person by exact name (as shown in list_complete_profiles). If pending is true, the profile is not ready yet. Visitor keys need relationships archive access.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string", "description": "Contact name matching a complete profile row"},
				},
				"required": []string{"name"},
			},
		},
	}
}

var embeddedJSONRe = regexp.MustCompile("(?s)```json\\s*(.*?)\\s*```")

// extractEmbeddedJSON removes ```json ... ``` blocks from text and returns the stripped
// text plus any successfully parsed JSON elements from those blocks.
func extractEmbeddedJSON(text string) (stripped string, elements []any) {
	matches := embeddedJSONRe.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		raw := strings.TrimSpace(m[1])
		if raw == "" {
			continue
		}
		var v any
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			continue
		}
		elements = append(elements, v)
	}
	stripped = strings.TrimSpace(embeddedJSONRe.ReplaceAllString(text, ""))
	return stripped, elements
}
