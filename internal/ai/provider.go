// Package ai provides chat provider implementations for Gemini and Claude.
package ai

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
)

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

// GenerateResult is the output of a generation call.
type GenerateResult struct {
	PlainText    string // response text with embedded JSON blocks stripped
	MetadataJSON string // JSON string with tokens, function calls, etc.
	Voice        string
}

// ToolExecutor executes a named AI tool and returns a JSON-serialisable result.
type ToolExecutor func(ctx context.Context, name string, args map[string]any) (map[string]any, error)

// ChatProvider is the interface implemented by both Gemini and Claude.
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
			"description": "Get the title, description, id and tags of all reference documents that are available for task.",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}},
		},
	}
}

var embeddedJSONRe = regexp.MustCompile("(?s)```json\\s*(.*?)\\s*```")

// stripEmbeddedJSON removes ```json ... ``` blocks from the response text.
func stripEmbeddedJSON(text string) string {
	return strings.TrimSpace(embeddedJSONRe.ReplaceAllString(text, ""))
}

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
