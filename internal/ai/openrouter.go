package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const openRouterChatCompletionsURL = "https://openrouter.ai/api/v1/chat/completions"
const openRouterCreditsURL = "https://openrouter.ai/api/v1/credits"
const openRouterMaxModelsFallback = 3

// OpenRouterCredits reports an OpenRouter account's cumulative purchased credits and
// usage, as returned by GET /api/v1/credits for the given API key.
type OpenRouterCredits struct {
	TotalCredits float64 `json:"total_credits"`
	TotalUsage   float64 `json:"total_usage"`
}

// Remaining estimates the remaining credit balance (total purchased minus total used).
func (c OpenRouterCredits) Remaining() float64 {
	return c.TotalCredits - c.TotalUsage
}

// FetchOpenRouterCredits queries OpenRouter's credits endpoint for apiKey.
func FetchOpenRouterCredits(ctx context.Context, apiKey string) (*OpenRouterCredits, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("openrouter credits: no API key configured")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, openRouterCreditsURL, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openRouter credits API %d: %s", resp.StatusCode, string(data))
	}
	var wrapper struct {
		Data OpenRouterCredits `json:"data"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	return &wrapper.Data, nil
}

// OpenRouterProvider calls the OpenRouter Chat Completions API (an
// OpenAI-compatible superset that can route to any of OpenRouter's
// supported models) with tool-calling support. One OpenRouterProvider
// instance backs one admin-configured AI model preset; providerKey is
// stamped into LLMUsage.Provider so billing keeps a distinct bucket per
// model, exactly as the old per-vendor providers did.
type OpenRouterProvider struct {
	apiKey      string
	modelName   string // OpenRouter compound model slug, e.g. "anthropic/claude-sonnet-4.5"
	providerKey string // admin-configured model key, e.g. "claude" — stamped into LLMUsage.Provider
}

// NewOpenRouterProvider creates an OpenRouterProvider. Returns nil if apiKey
// or modelName is empty.
func NewOpenRouterProvider(apiKey, modelName, providerKey string) *OpenRouterProvider {
	if apiKey == "" || modelName == "" {
		return nil
	}
	if providerKey == "" {
		providerKey = "openrouter"
	}
	return &OpenRouterProvider{apiKey: apiKey, modelName: modelName, providerKey: providerKey}
}

func (p *OpenRouterProvider) IsAvailable() bool { return p != nil }

// SimpleGenerate sends a prompt without tools. Used for classification and summarization.
func (p *OpenRouterProvider) SimpleGenerate(ctx context.Context, prompt string) (string, *LLMUsage, error) {
	if p == nil || p.apiKey == "" {
		return "", nil, fmt.Errorf("openrouter(%s): not configured", p.providerLabelOrUnknown())
	}
	body := map[string]any{
		"model":       p.modelName,
		"temperature": 0.2,
		"messages": []map[string]any{
			{"role": "user", "content": prompt},
		},
	}
	resp, err := p.post(ctx, body)
	if err != nil {
		return "", nil, err
	}
	model := openRouterModelFromResponse(resp, p.modelName)
	usage := p.usageFromResponse(resp, model)
	return strings.TrimSpace(openRouterExtractText(resp)), usage, nil
}

func (p *OpenRouterProvider) providerLabelOrUnknown() string {
	if p == nil || p.providerKey == "" {
		return "unknown"
	}
	return p.providerKey
}

func openRouterModelFromResponse(resp map[string]any, fallback string) string {
	if m, ok := resp["model"].(string); ok {
		if trimmed := strings.TrimSpace(m); trimmed != "" {
			return trimmed
		}
	}
	return fallback
}

func openRouterApplyModelRouting(body map[string]any, defaultModel string, models []string) string {
	if len(models) > openRouterMaxModelsFallback {
		models = models[:openRouterMaxModelsFallback]
	}
	if len(models) > 1 {
		body["models"] = models
		body["model"] = models[0]
		return models[0]
	}
	if len(models) == 1 {
		body["model"] = models[0]
		return models[0]
	}
	body["model"] = defaultModel
	return defaultModel
}

func (p *OpenRouterProvider) usageFromResponse(resp map[string]any, model string) *LLMUsage {
	in, out := 0, 0
	if u, ok := resp["usage"].(map[string]any); ok {
		if v, ok := u["prompt_tokens"].(float64); ok {
			in = int(v)
		}
		if v, ok := u["completion_tokens"].(float64); ok {
			out = int(v)
		}
	}
	if in == 0 && out == 0 {
		return nil
	}
	if model == "" {
		model = p.modelName
	}
	return &LLMUsage{Provider: p.providerKey, Model: model, InputTokens: in, OutputTokens: out}
}

func openRouterExtractText(resp map[string]any) string {
	choices, _ := resp["choices"].([]any)
	if len(choices) == 0 {
		return ""
	}
	choice, _ := choices[0].(map[string]any)
	msg, _ := choice["message"].(map[string]any)
	if msg == nil {
		return ""
	}
	content, _ := msg["content"].(string)
	return content
}

func openRouterExtractToolCalls(resp map[string]any) []map[string]any {
	choices, _ := resp["choices"].([]any)
	if len(choices) == 0 {
		return nil
	}
	choice, _ := choices[0].(map[string]any)
	msg, _ := choice["message"].(map[string]any)
	if msg == nil {
		return nil
	}
	raw, _ := msg["tool_calls"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// GenerateResponse calls OpenRouter with a tool-calling loop.
func (p *OpenRouterProvider) GenerateResponse(
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
	var tools []map[string]any
	for _, td := range defs {
		tools = append(tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        td["name"],
				"description": td["description"],
				"parameters":  td["parameters"],
			},
		})
	}

	messages := buildOpenRouterMessages(req, history, systemPrompt)
	funcCallsMade := []map[string]any{}
	inputTokens := 0
	outputTokens := 0
	responseModel := p.modelName

	for iter := 0; iter < maxToolCallIterations; iter++ {
		body := map[string]any{
			"messages":    messages,
			"temperature": req.Temperature,
		}
		requestModel := openRouterApplyModelRouting(body, p.modelName, req.OpenRouterModels)
		if len(tools) > 0 {
			body["tools"] = tools
		}

		resp, err := p.post(ctx, body)
		if err != nil {
			return GenerateResult{}, fmt.Errorf("openrouter(%s): %w", p.providerKey, err)
		}

		responseModel = openRouterModelFromResponse(resp, requestModel)
		if u, ok := resp["usage"].(map[string]any); ok {
			if v, ok := u["prompt_tokens"].(float64); ok {
				inputTokens += int(v)
			}
			if v, ok := u["completion_tokens"].(float64); ok {
				outputTokens += int(v)
			}
		}

		toolCalls := openRouterExtractToolCalls(resp)
		if len(toolCalls) == 0 {
			responseText := strings.TrimSpace(openRouterExtractText(resp))
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
				"provider":         p.providerKey,
				"model":            responseModel,
			}
			if len(req.OpenRouterModels) > 1 {
				metadata["openrouter_models"] = req.OpenRouterModels
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
					Provider:     p.providerKey,
					Model:        responseModel,
					InputTokens:  inputTokens,
					OutputTokens: outputTokens,
				},
			}, nil
		}

		choices, _ := resp["choices"].([]any)
		var assistantMsg map[string]any
		if len(choices) > 0 {
			if choice, ok := choices[0].(map[string]any); ok {
				if msg, ok := choice["message"].(map[string]any); ok {
					assistantMsg = msg
				}
			}
		}
		if assistantMsg != nil {
			messages = append(messages, assistantMsg)
		}

		for _, tc := range toolCalls {
			fn, _ := tc["function"].(map[string]any)
			toolName, _ := fn["name"].(string)
			toolCallID, _ := tc["id"].(string)
			var toolInput map[string]any
			if argsRaw, ok := fn["arguments"].(string); ok && argsRaw != "" {
				_ = json.Unmarshal([]byte(argsRaw), &toolInput)
			}
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
			messages = append(messages, map[string]any{
				"role":         "tool",
				"tool_call_id": toolCallID,
				"content":      string(resultJSON),
			})
		}
	}

	return GenerateResult{}, fmt.Errorf("openrouter(%s): exceeded max tool-calling iterations", p.providerKey)
}

func buildOpenRouterMessages(req GenerateRequest, history []ConvTurn, systemPrompt string) []map[string]any {
	var messages []map[string]any
	if systemPrompt != "" {
		messages = append(messages, map[string]any{
			"role":    "system",
			"content": systemPrompt,
		})
	}

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

func (p *OpenRouterProvider) post(ctx context.Context, body map[string]any) (map[string]any, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, openRouterChatCompletionsURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openRouter API %d: %s", resp.StatusCode, string(data))
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}
