// Package ai provides chat provider implementations: OpenRouterProvider (a
// single OpenAI-compatible adapter backing every admin-configured hosted AI
// model preset — Claude, Gemini, DeepSeek, OpenAI models and any custom
// model an admin adds, all routed through OpenRouter) and LocalAI (Ollama).
package ai

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
)

// maxToolCallIterations caps assistant→tool→tool_result rounds per GenerateResponse
// (Claude and Gemini). Tool-heavy chat flows may chain many archive tools.
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
	// OpenRouterModels, when len > 1 (max 3), is sent as OpenRouter's "models" fallback array
	// (primary slug first). Empty means use the provider's configured model only.
	OpenRouterModels []string
	// Attachments, when non-empty, are woven into the final user message as multimodal content
	// parts (OpenRouterProvider only — see buildOpenRouterMessages). Only ever populated by
	// ChatBotService, whose vision gate guarantees the resolved provider supports image input
	// before setting this; every other caller (persona chat, Interview, Have-a-Chat) leaves it
	// nil, so their request bodies are unchanged.
	Attachments []Attachment
}

// Attachment is one image to send alongside a chat request as multimodal content.
type Attachment struct {
	MimeType   string
	DataBase64 string
	Filename   string
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

// ChatProvider is the interface implemented by OpenRouterProvider (which backs
// every admin-configured hosted AI model preset) and LocalAI.
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
	// SimpleGenerate sends a prompt without tools. Used for classification,
	// summarization, and identity-profile extraction.
	SimpleGenerate(ctx context.Context, prompt string) (string, *LLMUsage, error)
}

// SummarizerResolver resolves a ChatProvider for one-shot summarization tasks (email/message
// thread summarization, admin bulk-classification/writing-style/psych-profile generation) that
// don't go through the full chat request flow. Implemented by service.ChatService, which prefers
// the well-known "gemini" model key for historical continuity and falls back to the default
// enabled AI model if "gemini" is missing or disabled.
type SummarizerResolver interface {
	// ResolveSummarizer returns the provider to use and the model key it resolved to (for billing),
	// or (nil, "") if no AI model is available.
	ResolveSummarizer(ctx context.Context) (ChatProvider, string)
}

// GetToolDefinitions returns the tool schema for callers that pass explicit toolDecls — the
// tool set currently discovered from the MCP tools server (DefaultToolCatalog), not a static
// list. Every tool now lives in cmd/mcpserver; this app defines no tool schemas of its own.
func GetToolDefinitions() []map[string]any {
	return DefaultToolCatalog().Definitions()
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
