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

// DeepSeek's Anthropic-compatible Messages API.
// See https://api-docs.deepseek.com/guides/anthropic_api
const deepSeekMessagesURL = "https://api.deepseek.com/anthropic/v1/messages"

const deepSeekAnthropicVersion = "2023-06-01"

// DeepSeekProvider calls DeepSeek's Anthropic Messages-compatible REST API with tool support.
type DeepSeekProvider struct {
	apiKey    string
	modelName string
}

// NewDeepSeekProvider returns a provider for DeepSeek's Anthropic-compatible endpoint.
// Returns nil if apiKey is empty.
func NewDeepSeekProvider(apiKey, modelName string) *DeepSeekProvider {
	if apiKey == "" {
		return nil
	}
	if modelName == "" {
		modelName = "deepseek-chat"
	}
	return &DeepSeekProvider{apiKey: apiKey, modelName: modelName}
}

func (p *DeepSeekProvider) IsAvailable() bool { return p != nil }

// SimpleGenerate sends a prompt without tools (Messages API, no system block).
func (p *DeepSeekProvider) SimpleGenerate(ctx context.Context, prompt string) (string, *LLMUsage, error) {
	if p == nil || p.apiKey == "" {
		return "", nil, fmt.Errorf("deepseek: not configured")
	}
	body := map[string]any{
		"model":       p.modelName,
		"max_tokens":  8096,
		"temperature": 0.2,
		"messages": []map[string]any{
			{"role": "user", "content": prompt},
		},
	}
	resp, err := p.post(ctx, body)
	if err != nil {
		return "", nil, err
	}
	usage := deepSeekUsageFromResponse(resp, p.modelName)
	contentRaw, _ := resp["content"].([]any)
	var textParts []string
	for _, item := range contentRaw {
		if block, ok := item.(map[string]any); ok && block["type"] == "text" {
			if t, ok := block["text"].(string); ok && t != "" {
				textParts = append(textParts, t)
			}
		}
	}
	return strings.TrimSpace(strings.Join(textParts, "")), usage, nil
}

func deepSeekUsageFromResponse(resp map[string]any, modelName string) *LLMUsage {
	in, out := 0, 0
	if u, ok := resp["usage"].(map[string]any); ok {
		if v, ok := u["input_tokens"].(float64); ok {
			in = int(v)
		}
		if v, ok := u["output_tokens"].(float64); ok {
			out = int(v)
		}
	}
	if in == 0 && out == 0 {
		return nil
	}
	return &LLMUsage{Provider: "deepseek", Model: modelName, InputTokens: in, OutputTokens: out}
}

// GenerateResponse runs a tool-calling loop against the DeepSeek Anthropic-compatible API.
func (p *DeepSeekProvider) GenerateResponse(
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
	tools := make([]map[string]any, 0, len(defs))
	for _, td := range defs {
		tools = append(tools, map[string]any{
			"name":         td["name"],
			"description":  td["description"],
			"input_schema": td["parameters"],
		})
	}

	messages := buildDeepSeekMessages(req, history)
	funcCallsMade := []map[string]any{}
	inputTokens := 0
	outputTokens := 0
	temperature := math.Min(req.Temperature, 1.0)

	for iter := 0; iter < maxToolCallIterations; iter++ {
		body := map[string]any{
			"model":       p.modelName,
			"max_tokens":  8096,
			"temperature": temperature,
			"system": []map[string]any{
				{"type": "text", "text": systemPrompt},
			},
			"messages": messages,
		}
		if len(tools) > 0 {
			body["tools"] = tools
		}

		resp, err := p.post(ctx, body)
		if err != nil {
			return GenerateResult{}, fmt.Errorf("deepseek: %w", err)
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
				"provider":         "deepseek",
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
				Usage: &LLMUsage{
					Provider: "deepseek", Model: p.modelName,
					InputTokens: inputTokens, OutputTokens: outputTokens,
				},
			}, nil
		}

		messages = append(messages, map[string]any{
			"role":    "assistant",
			"content": contentRaw,
		})

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
			toolResults = append(toolResults, map[string]any{
				"type":        "tool_result",
				"tool_use_id": toolID,
				"content":     string(resultJSON),
			})
		}

		messages = append(messages, map[string]any{
			"role":    "user",
			"content": toolResults,
		})
	}

	return GenerateResult{}, fmt.Errorf("deepseek: exceeded max tool-calling iterations")
}

// buildDeepSeekMessages mirrors the Anthropic-style user/assistant layout used for Claude chat.
func buildDeepSeekMessages(req GenerateRequest, history []ConvTurn) []map[string]any {
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

func (p *DeepSeekProvider) post(ctx context.Context, body map[string]any) (map[string]any, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, deepSeekMessagesURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", deepSeekAnthropicVersion)
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
		return nil, fmt.Errorf("DeepSeek API %d: %s", resp.StatusCode, string(data))
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}
