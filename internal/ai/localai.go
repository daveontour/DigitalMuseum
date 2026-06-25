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

// LocalAIProvider calls an Ollama server via the native /api/chat and /api/embed endpoints.
type LocalAIProvider struct {
	baseURL   string
	apiKey    string // retained for interface compatibility; not required by Ollama
	modelName string
	// numCtx is sent under "options".num_ctx on every chat request when > 0.
	// Zero leaves it unset so Ollama uses its server-side default (set via the
	// OLLAMA_NUM_CTX env var when Electron starts the daemon).
	numCtx int
}

// ollamaRequest is the body sent to POST /api/chat.
type ollamaRequest struct {
	Model    string           `json:"model"`
	Messages []map[string]any `json:"messages"`
	Tools    []map[string]any `json:"tools,omitempty"`
	Stream   bool             `json:"stream"`
	Format   any              `json:"format,omitempty"`
	Think    *bool            `json:"think,omitempty"`
	Options  map[string]any   `json:"options,omitempty"`
}

// ollamaResponse is returned by POST /api/chat with stream:false.
type ollamaResponse struct {
	Message struct {
		Role      string `json:"role"`
		Content   string `json:"content"`
		Thinking  string `json:"thinking"`
		ToolCalls []struct {
			Function struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	} `json:"message"`
	DoneReason      string `json:"done_reason"`
	Done            bool   `json:"done"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
}

// NewLocalAIProvider creates a LocalAIProvider. Returns nil if baseURL is empty.
// numCtx <= 0 means "do not send num_ctx in the request body" (the Ollama server
// will use its own default, typically set via the OLLAMA_NUM_CTX env var).
func NewLocalAIProvider(baseURL, apiKey, modelName string, numCtx int) *LocalAIProvider {
	if strings.TrimSpace(baseURL) == "" {
		return nil
	}
	if modelName == "" {
		modelName = "local-model"
	}
	if numCtx < 0 {
		numCtx = 0
	}
	return &LocalAIProvider{
		baseURL:   strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:    apiKey,
		modelName: modelName,
		numCtx:    numCtx,
	}
}

func (p *LocalAIProvider) IsAvailable() bool { return p != nil }

// SimpleGenerate sends a prompt without tools. Used for summarisation tasks.
func (p *LocalAIProvider) SimpleGenerate(ctx context.Context, prompt string) (string, *LLMUsage, error) {
	return p.simpleGenerate(ctx, prompt, 0)
}

// SimpleGenerateWithNumCtx is like SimpleGenerate but sets options.num_ctx for this request.
// numCtx must be positive; it overrides the provider default and is always sent.
func (p *LocalAIProvider) SimpleGenerateWithNumCtx(ctx context.Context, prompt string, numCtx int) (string, *LLMUsage, error) {
	if numCtx <= 0 {
		return p.simpleGenerate(ctx, prompt, 0)
	}
	return p.simpleGenerate(ctx, prompt, numCtx)
}

type simpleGenerateOptions struct {
	numCtx     int
	omitNumCtx bool
	format     map[string]any
	think      *bool
}

// SimpleGenerateClassifier sends a structured-output classification prompt to Ollama.
// preferredNumCtx is attempted first; on failure the call is retried without num_ctx so
// Ollama uses its server default (some model tags such as gemma4:e2b crash when num_ctx is low).
func (p *LocalAIProvider) SimpleGenerateClassifier(ctx context.Context, prompt string, preferredNumCtx int, formatSchema map[string]any) (string, *LLMUsage, error) {
	thinkFalse := false
	opts := simpleGenerateOptions{numCtx: preferredNumCtx, format: formatSchema, think: &thinkFalse}
	text, usage, err := p.simpleGenerateAdvanced(ctx, prompt, opts)
	if err == nil && strings.TrimSpace(text) != "" {
		return text, usage, nil
	}
	if preferredNumCtx > 0 {
		opts.numCtx = 0
		opts.omitNumCtx = true
		text2, usage2, err2 := p.simpleGenerateAdvanced(ctx, prompt, opts)
		if err2 == nil && strings.TrimSpace(text2) != "" {
			return text2, usage2, nil
		}
		if err != nil {
			return "", usage2, err2
		}
	}
	if formatSchema != nil {
		text3, usage3, err3 := p.simpleGenerateAdvanced(ctx, prompt, simpleGenerateOptions{omitNumCtx: true})
		if err3 == nil && strings.TrimSpace(text3) != "" {
			return text3, usage3, nil
		}
		if err != nil {
			return "", usage3, err3
		}
	}
	if err != nil {
		return "", usage, err
	}
	return text, usage, fmt.Errorf("localai: empty classifier response")
}

func (p *LocalAIProvider) simpleGenerate(ctx context.Context, prompt string, numCtxOverride int) (string, *LLMUsage, error) {
	return p.simpleGenerateAdvanced(ctx, prompt, simpleGenerateOptions{numCtx: numCtxOverride})
}

func (p *LocalAIProvider) simpleGenerateAdvanced(ctx context.Context, prompt string, opts simpleGenerateOptions) (string, *LLMUsage, error) {
	if p == nil || p.baseURL == "" {
		return "", nil, fmt.Errorf("localai: not configured")
	}
	options := map[string]any{"temperature": 0.2}
	if !opts.omitNumCtx {
		if opts.numCtx > 0 {
			options["num_ctx"] = opts.numCtx
		} else if p.numCtx > 0 {
			options["num_ctx"] = p.numCtx
		}
	}
	req := ollamaRequest{
		Model:  p.modelName,
		Stream: false,
		Messages: []map[string]any{
			{"role": "user", "content": prompt},
		},
		Options: options,
	}
	if len(opts.format) > 0 {
		req.Format = opts.format
	}
	if opts.think != nil {
		req.Think = opts.think
	}
	resp, err := ollamaPost(ctx, p.baseURL, req)
	if err != nil {
		return "", nil, err
	}
	var usage *LLMUsage
	if resp.PromptEvalCount > 0 || resp.EvalCount > 0 {
		usage = &LLMUsage{
			Provider:     "localai",
			Model:        p.modelName,
			InputTokens:  resp.PromptEvalCount,
			OutputTokens: resp.EvalCount,
		}
	}
	text := strings.TrimSpace(resp.Message.Content)
	if text == "" {
		text = strings.TrimSpace(resp.Message.Thinking)
	}
	return text, usage, nil
}

// GenerateResponse calls the Ollama native API with a tool-calling loop.
func (p *LocalAIProvider) GenerateResponse(
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

	// Ollama native tool definitions use the same shape as OpenAI.
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

	messages := buildLocalAIMessages(req, history, systemPrompt)
	funcCallsMade := []map[string]any{}
	inputTokens := 0
	outputTokens := 0

	for iter := 0; iter < maxToolCallIterations; iter++ {
		opts := map[string]any{"temperature": req.Temperature}
		if p.numCtx > 0 {
			opts["num_ctx"] = p.numCtx
		}
		ollamaReq := ollamaRequest{
			Model:    p.modelName,
			Messages: messages,
			Tools:    tools,
			Stream:   false,
			Options:  opts,
		}

		resp, err := ollamaPost(ctx, p.baseURL, ollamaReq)
		if err != nil {
			return GenerateResult{}, fmt.Errorf("localai: %w", err)
		}

		inputTokens += resp.PromptEvalCount
		outputTokens += resp.EvalCount

		if len(resp.Message.ToolCalls) == 0 {
			// Text response — loop complete.
			responseText := strings.TrimSpace(resp.Message.Content)
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
				"provider":         "localai",
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
					Provider:     "localai",
					Model:        p.modelName,
					InputTokens:  inputTokens,
					OutputTokens: outputTokens,
				},
			}, nil
		}

		// Echo the assistant message (including tool_calls) back into the conversation.
		messages = append(messages, map[string]any{
			"role":       "assistant",
			"content":    resp.Message.Content,
			"tool_calls": resp.Message.ToolCalls,
		})

		// Execute each tool call and append the results.
		for _, tc := range resp.Message.ToolCalls {
			toolName := tc.Function.Name
			toolInput := tc.Function.Arguments
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

			// Ollama native API does not use tool_call_id.
			messages = append(messages, map[string]any{
				"role":    "tool",
				"content": string(resultJSON),
			})

			// Prevent the same tool from being offered in subsequent iterations.
			tools = removeToolDefinitionByName(tools, toolName)
		}
	}

	return GenerateResult{}, fmt.Errorf("localai: exceeded max tool-calling iterations")
}

// removeToolDefinitionByName removes a tool definition by name from a list of tool definitions.
// This is used to prevent the same tool from being offered in subsequent iterations.
// I saw this type of behaviour in the wild with the gemma4 model.
func removeToolDefinitionByName(tools []map[string]any, name string) []map[string]any {
	if name == "" || len(tools) == 0 {
		return tools
	}
	filtered := tools[:0]
	for _, t := range tools {
		fn, _ := t["function"].(map[string]any)
		toolName, _ := fn["name"].(string)
		if toolName == name {
			continue
		}
		filtered = append(filtered, t)
	}
	return filtered
}

func buildLocalAIMessages(req GenerateRequest, history []ConvTurn, systemPrompt string) []map[string]any {
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

// Embed calls the Ollama native /api/embed endpoint and returns the embedding vector.
func (p *LocalAIProvider) Embed(ctx context.Context, text, embeddingModel string) ([]float32, error) {
	if p == nil || p.baseURL == "" {
		return nil, fmt.Errorf("localai: not configured")
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("localai: embed called with empty text")
	}
	model := embeddingModel
	if strings.TrimSpace(model) == "" {
		model = p.modelName
	}

	body := map[string]any{
		"model": model,
		"input": text,
	}
	resp, err := ollamaEmbedPost(ctx, p.baseURL, body)
	if err != nil {
		return nil, fmt.Errorf("localai embed: %w", err)
	}

	// Response: {"embeddings": [[float, ...]]}
	embsRaw, _ := resp["embeddings"].([]any)
	if len(embsRaw) == 0 {
		return nil, fmt.Errorf("localai embed: empty embeddings array in response")
	}
	firstRaw, _ := embsRaw[0].([]any)
	if len(firstRaw) == 0 {
		return nil, fmt.Errorf("localai embed: no embedding values in response")
	}

	vec := make([]float32, len(firstRaw))
	for i, v := range firstRaw {
		f, ok := v.(float64)
		if !ok {
			return nil, fmt.Errorf("localai embed: unexpected value type at index %d", i)
		}
		vec[i] = float32(f)
	}
	return vec, nil
}

// ollamaPost sends a request to POST /api/chat and decodes the response.
func ollamaPost(ctx context.Context, baseURL string, body ollamaRequest) (*ollamaResponse, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/chat", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
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
		return nil, fmt.Errorf("ollama API %d: %s", resp.StatusCode, string(data))
	}
	var result ollamaResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ollamaEmbedPost sends a request to POST /api/embed and returns the raw response map.
func ollamaEmbedPost(ctx context.Context, baseURL string, body map[string]any) (map[string]any, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/embed", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
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
		return nil, fmt.Errorf("ollama embed API %d: %s", resp.StatusCode, string(data))
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}
