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

const geminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/models"

// GeminiProvider calls the Gemini REST API with tool-calling support.
type GeminiProvider struct {
	apiKey    string
	modelName string
}

// NewGeminiProvider creates a GeminiProvider. Returns nil if apiKey is empty.
func NewGeminiProvider(apiKey, modelName string) *GeminiProvider {
	if apiKey == "" {
		return nil
	}
	if modelName == "" {
		modelName = "gemini-2.5-flash"
	}
	return &GeminiProvider{apiKey: apiKey, modelName: modelName}
}

func (p *GeminiProvider) IsAvailable() bool { return p != nil }

// SimpleGenerate sends a prompt to Gemini without tools. Returns the response text.
// Used for summarization and other tasks that don't need tool-calling.
func (p *GeminiProvider) SimpleGenerate(ctx context.Context, prompt string) (string, error) {
	if p == nil || p.apiKey == "" {
		return "", fmt.Errorf("gemini: not configured")
	}
	body := map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": prompt}}},
		},
		"generationConfig": map[string]any{"temperature": 0.2},
	}
	resp, err := geminiPost(ctx, p.apiKey, p.modelName, body)
	if err != nil {
		return "", err
	}
	parts := geminiExtractParts(resp)
	var textParts []string
	for _, part := range parts {
		if t, ok := part["text"].(string); ok && t != "" {
			textParts = append(textParts, t)
		}
	}
	return strings.TrimSpace(strings.Join(textParts, " ")), nil
}

// GenerateResponse calls Gemini with a tool-calling loop.
func (p *GeminiProvider) GenerateResponse(
	ctx context.Context,
	req GenerateRequest,
	systemPrompt string,
	history []ConvTurn,
	executor ToolExecutor,
	toolDecls *[]map[string]any,
) (GenerateResult, error) {
	// Callers without an executor must not advertise tools (avoids unusable tool calls).
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
	funcDecls := make([]map[string]any, 0, len(defs))
	for _, td := range defs {
		funcDecls = append(funcDecls, map[string]any{
			"name":        td["name"],
			"description": td["description"],
			"parameters":  td["parameters"],
		})
	}
	var tools []map[string]any
	if len(funcDecls) > 0 {
		tools = []map[string]any{{"functionDeclarations": funcDecls}}
	}

	contents := buildGeminiContents(req, history)
	funcCallsMade := []map[string]any{}
	inputTokens := 0
	outputTokens := 0

	for iter := 0; iter < 5; iter++ {
		body := map[string]any{
			"system_instruction": map[string]any{
				"parts": []map[string]any{{"text": systemPrompt}},
			},
			"contents": contents,
			"generationConfig": map[string]any{
				"temperature": req.Temperature,
			},
		}
		if len(tools) > 0 {
			body["tools"] = tools
		}

		resp, err := geminiPost(ctx, p.apiKey, p.modelName, body)
		if err != nil {
			return GenerateResult{}, fmt.Errorf("gemini: %w", err)
		}

		// Accumulate token usage
		if um, ok := resp["usageMetadata"].(map[string]any); ok {
			if v, ok := um["promptTokenCount"].(float64); ok {
				inputTokens += int(v)
			}
			if v, ok := um["candidatesTokenCount"].(float64); ok {
				outputTokens += int(v)
			}
		}

		parts := geminiExtractParts(resp)

		// Collect function calls
		var funcCalls []map[string]any
		for _, part := range parts {
			if fc, ok := part["functionCall"].(map[string]any); ok {
				funcCalls = append(funcCalls, fc)
			}
		}

		if len(funcCalls) == 0 {
			// Done — extract text
			var textParts []string
			for _, part := range parts {
				if t, ok := part["text"].(string); ok && t != "" {
					textParts = append(textParts, t)
				}
			}
			responseText := strings.TrimSpace(strings.Join(textParts, " "))
			if responseText == "" {
				responseText = "I apologize, but I couldn't generate a response."
			}
			plainText := stripEmbeddedJSON(responseText)
			metadata := map[string]any{
				"referenced_files": []any{},
				"function_calls":   funcCallsMade,
				"input_tokens":     inputTokens,
				"output_tokens":    outputTokens,
				"total_tokens":     inputTokens + outputTokens,
				"provider":         "gemini",
				"model":            p.modelName,
			}
			metaJSON, _ := json.Marshal(metadata)
			return GenerateResult{
				PlainText:    plainText,
				MetadataJSON: string(metaJSON),
				Voice:        req.Voice,
			}, nil
		}

		// Append model's message (containing function calls) to history
		contents = append(contents, map[string]any{"role": "model", "parts": parts})

		// Execute each function call
		var funcResponseParts []map[string]any
		for _, fc := range funcCalls {
			name, _ := fc["name"].(string)
			argsRaw, _ := fc["args"].(map[string]any)
			if argsRaw == nil {
				argsRaw = map[string]any{}
			}
			funcCallsMade = append(funcCallsMade, map[string]any{
				"name":      name,
				"arguments": argsRaw,
				"iteration": iter + 1,
			})
			result, err := exec(ctx, name, argsRaw)
			if err != nil {
				result = map[string]any{"error": err.Error()}
			}
			funcResponseParts = append(funcResponseParts, map[string]any{
				"functionResponse": map[string]any{
					"name":     name,
					"response": result,
				},
			})
		}

		// Append user message with function responses
		contents = append(contents, map[string]any{"role": "user", "parts": funcResponseParts})
	}

	return GenerateResult{}, fmt.Errorf("gemini: exceeded max tool-calling iterations")
}

func buildGeminiContents(req GenerateRequest, history []ConvTurn) []map[string]any {
	var contents []map[string]any

	recent := history
	if len(recent) > 20 {
		recent = recent[len(recent)-20:]
	}
	for _, turn := range recent {
		contents = append(contents,
			map[string]any{"role": "user", "parts": []map[string]any{{"text": turn.UserInput}}},
			map[string]any{"role": "model", "parts": []map[string]any{{"text": turn.ResponseText}}},
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
	parts = append(parts, "\nUser input:\n"+req.UserInput)
	userText := strings.Join(parts, "\n")

	contents = append(contents, map[string]any{
		"role":  "user",
		"parts": []map[string]any{{"text": userText}},
	})
	return contents
}

func geminiPost(ctx context.Context, apiKey, modelName string, body map[string]any) (map[string]any, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s:generateContent?key=%s", geminiBaseURL, modelName, apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
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
		return nil, fmt.Errorf("Gemini API %d: %s", resp.StatusCode, string(data))
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func geminiExtractParts(resp map[string]any) []map[string]any {
	candidates, _ := resp["candidates"].([]any)
	if len(candidates) == 0 {
		return nil
	}
	cand, _ := candidates[0].(map[string]any)
	content, _ := cand["content"].(map[string]any)
	partsRaw, _ := content["parts"].([]any)
	parts := make([]map[string]any, 0, len(partsRaw))
	for _, p := range partsRaw {
		if pm, ok := p.(map[string]any); ok {
			parts = append(parts, pm)
		}
	}
	return parts
}
