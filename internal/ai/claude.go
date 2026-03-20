package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
)

const claudeMessagesURL = "https://api.anthropic.com/v1/messages"
const claudeAPIVersion = "2023-06-01"

// ClaudeProvider calls the Anthropic Claude REST API with tool-calling support.
type ClaudeProvider struct {
	apiKey    string
	modelName string
}

// NewClaudeProvider creates a ClaudeProvider. Returns nil if apiKey is empty.
func NewClaudeProvider(apiKey, modelName string) *ClaudeProvider {
	if apiKey == "" {
		return nil
	}
	if modelName == "" {
		modelName = "claude-sonnet-4-6"
	}
	return &ClaudeProvider{apiKey: apiKey, modelName: modelName}
}

func (p *ClaudeProvider) IsAvailable() bool { return p != nil }

// SimpleGenerate sends a prompt to Claude without tools. Returns the response text.
// Used for summarization and other tasks that don't need tool-calling.
func (p *ClaudeProvider) SimpleGenerate(ctx context.Context, prompt string) (string, error) {
	if p == nil || p.apiKey == "" {
		return "", fmt.Errorf("claude: not configured")
	}
	body := map[string]any{
		"model":       p.modelName,
		"max_tokens":  8096,
		"temperature": 0.2,
		"messages": []map[string]any{
			{"role": "user", "content": prompt},
		},
	}
	resp, err := claudePost(ctx, p.apiKey, body)
	if err != nil {
		return "", err
	}
	contentRaw, _ := resp["content"].([]any)
	var textParts []string
	for _, item := range contentRaw {
		if block, ok := item.(map[string]any); ok && block["type"] == "text" {
			if t, ok := block["text"].(string); ok && t != "" {
				textParts = append(textParts, t)
			}
		}
	}
	return strings.TrimSpace(strings.Join(textParts, "")), nil
}

// GenerateResponse calls Claude with a tool-calling loop.
func (p *ClaudeProvider) GenerateResponse(
	ctx context.Context,
	req GenerateRequest,
	systemPrompt string,
	history []ConvTurn,
	executor ToolExecutor,
	toolDecls *[]map[string]any,
) (GenerateResult, error) {
	if executor == nil {
		empty := []map[string]any{}
		toolDecls = &empty
	}
	exec := executor
	if exec == nil {
		exec = func(_ context.Context, name string, _ map[string]any) (map[string]any, error) {
			return map[string]any{"error": "tool execution not available: " + name}, nil
		}
	}

	var defs []map[string]any
	if toolDecls == nil {
		defs = toolDefinitions()
	} else {
		defs = *toolDecls
	}
	claudeTools := make([]map[string]any, 0, len(defs))
	for _, td := range defs {
		claudeTools = append(claudeTools, map[string]any{
			"name":         td["name"],
			"description":  td["description"],
			"input_schema": td["parameters"],
		})
	}
	if len(claudeTools) > 0 {
		claudeTools[len(claudeTools)-1]["cache_control"] = map[string]any{"type": "ephemeral", "ttl": "1h"}
	}

	messages := buildClaudeMessages(req, history)
	funcCallsMade := []map[string]any{}
	inputTokens := 0
	outputTokens := 0
	temperature := math.Min(req.Temperature, 1.0)

	for iter := 0; iter < 5; iter++ {
		body := map[string]any{
			"model":       p.modelName,
			"max_tokens":  8096,
			"temperature": temperature,
			"system": []map[string]any{
				{
					"type":          "text",
					"text":          systemPrompt,
					"cache_control": map[string]any{"type": "ephemeral", "ttl": "1h"},
				},
			},
			"messages": messages,
		}
		if len(claudeTools) > 0 {
			body["tools"] = claudeTools
		}

		resp, err := claudePost(ctx, p.apiKey, body)
		if err != nil {
			return GenerateResult{}, fmt.Errorf("claude: %w", err)
		}

		if u, ok := resp["usage"].(map[string]any); ok {
			if v, ok := u["input_tokens"].(float64); ok {
				inputTokens += int(v)
			}
			if v, ok := u["output_tokens"].(float64); ok {
				outputTokens += int(v)
			}
		}

		stopReason, _ := resp["stop_reason"].(string)
		contentRaw, _ := resp["content"].([]any)

		if stopReason != "tool_use" {
			var textParts []string
			for _, item := range contentRaw {
				if block, ok := item.(map[string]any); ok {
					if block["type"] == "text" {
						if t, ok := block["text"].(string); ok && t != "" {
							textParts = append(textParts, t)
						}
					}
				}
			}
			responseText := strings.TrimSpace(strings.Join(textParts, ""))
			if responseText == "" {
				responseText = "I apologize, but I couldn't generate a response."
			}
			plainText, embeddedElements := extractEmbeddedJSON(responseText)
			metadata := map[string]any{
				"referenced_files": []any{},
				"function_calls":   funcCallsMade,
				"input_tokens":     inputTokens,
				"output_tokens":    outputTokens,
				"total_tokens":     inputTokens + outputTokens,
				"provider":         "claude",
				"model":            p.modelName,
			}
			if len(embeddedElements) > 0 {
				metadata["embedded_json"] = embeddedElements
			}
			metaJSON, _ := json.Marshal(metadata)
			return GenerateResult{
				PlainText:    plainText,
				MetadataJSON: string(metaJSON),
				Voice:        req.Voice,
			}, nil
		}

		// Append assistant message with tool use blocks
		messages = append(messages, map[string]any{
			"role":    "assistant",
			"content": contentRaw,
		})

		// Execute tool calls and collect tool_result blocks
		var toolResults []map[string]any
		for _, item := range contentRaw {
			block, ok := item.(map[string]any)
			if !ok || block["type"] != "tool_use" {
				continue
			}
			toolID, _ := block["id"].(string)
			toolName, _ := block["name"].(string)
			toolInput, _ := block["input"].(map[string]any)
			if toolInput == nil {
				toolInput = map[string]any{}
			}

			funcCallsMade = append(funcCallsMade, map[string]any{
				"name":      toolName,
				"arguments": toolInput,
				"iteration": iter + 1,
			})

			result, err := exec(ctx, toolName, toolInput)
			if err != nil {
				result = map[string]any{"error": err.Error()}
			}
			resultJSON, _ := json.Marshal(result)
			toolResult := map[string]any{
				"type":        "tool_result",
				"tool_use_id": toolID,
				"content":     string(resultJSON),
			}
			if toolName == "get_reference_document" {
				toolResult["content"] = []map[string]any{
					{
						"type":          "text",
						"text":          string(resultJSON),
						"cache_control": map[string]any{"type": "ephemeral", "ttl": "1h"},
					},
				}
			}
			toolResults = append(toolResults, toolResult)
		}

		messages = append(messages, map[string]any{
			"role":    "user",
			"content": toolResults,
		})
	}

	return GenerateResult{}, fmt.Errorf("claude: exceeded max tool-calling iterations")
}

func buildClaudeMessages(req GenerateRequest, history []ConvTurn) []map[string]any {
	var messages []map[string]any
	recent := history
	if len(recent) > 20 {
		recent = recent[len(recent)-20:]
	}
	for _, turn := range recent {
		messages = append(messages,
			map[string]any{"role": "user", "content": turn.UserInput},
			map[string]any{"role": "assistant", "content": turn.ResponseText},
		)
	}

	var parts []string
	if req.Voice == "owner" {
		parts = append(parts, "IMPORTANT: Respond in the first person voice as the owner of the subject's life.")
		parts = append(parts, fmt.Sprintf("IMPORTANT: Your current mood is %s", req.Mood))
		if req.PsychProfile != nil && *req.PsychProfile != "" {
			parts = append(parts, fmt.Sprintf("IMPORTANT: Respond consistent with your psychological profile: %s", *req.PsychProfile))
		}
		if req.WritingStyle != nil && *req.WritingStyle != "" {
			parts = append(parts, fmt.Sprintf("IMPORTANT: Respond consistent with your writing style: %s", *req.WritingStyle))
		}
	}
	if req.CompanionMode {
		parts = append(parts, "IMPORTANT: You are in companion mode. Respond conversationally as a friend, not as an assistant.")
	}
	if len(parts) > 0 {
		parts = append(parts, "\nUser input:\n"+req.UserInput)
	} else {
		parts = []string{req.UserInput}
	}
	messages = append(messages, map[string]any{"role": "user", "content": strings.Join(parts, "\n")})
	return messages
}

func claudePost(ctx context.Context, apiKey string, body map[string]any) (map[string]any, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	fmt.Println("Request body:")
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, b, "", "  "); err == nil {
		fmt.Println(prettyJSON.String())
	} else {
		fmt.Println(string(b))
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, claudeMessagesURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("anthropic-version", claudeAPIVersion)
	httpReq.Header.Set("anthropic-beta", "extended-cache-ttl-2025-04-11")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Claude API %d: %s", resp.StatusCode, string(data))
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}
