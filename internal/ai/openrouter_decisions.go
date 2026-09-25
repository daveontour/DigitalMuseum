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

const openRouterDecisionsURL = "https://openrouter.ai/api/alpha/decisions"

// JevModelID is the pinned TypeSafe Jev release used for Auto routing.
const JevModelID = "typesafe/jev-1.13"

// JevDecisionResult is a Decisions API response from Jev.
type JevDecisionResult struct {
	ID      string                     `json:"id"`
	Model   string                     `json:"model"`
	Answers map[string]json.RawMessage `json:"answers"`
	Usage   JevUsage                   `json:"usage"`
}

// JevUsage is token accounting from a Decisions API response.
type JevUsage struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	Cost         float64 `json:"cost"`
}

// CallJevDecisions posts state and typed questions to Jev through OpenRouter.
// rawResponse is the response body when one was received, including non-200 responses.
func CallJevDecisions(ctx context.Context, apiKey string, state map[string]any, questions map[string]any) (*JevDecisionResult, []byte, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, nil, fmt.Errorf("jev: no OpenRouter API key configured")
	}
	body := map[string]any{
		"model":     JevModelID,
		"state":     state,
		"questions": questions,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, openRouterDecisionsURL, bytes.NewReader(payload))
	if err != nil {
		return nil, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, data, fmt.Errorf("openRouter decisions API %d: %s", resp.StatusCode, string(data))
	}
	var result JevDecisionResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, data, fmt.Errorf("jev: decode response: %w", err)
	}
	return &result, data, nil
}
