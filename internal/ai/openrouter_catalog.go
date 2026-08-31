package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const openRouterModelsURL = "https://openrouter.ai/api/v1/models"

// OpenRouterArchitecture describes a catalog model's modality support.
type OpenRouterArchitecture struct {
	Modality         string   `json:"modality"`
	InputModalities  []string `json:"input_modalities"`
	OutputModalities []string `json:"output_modalities"`
	Tokenizer        string   `json:"tokenizer"`
	InstructType     *string  `json:"instruct_type"`
}

// OpenRouterPricing holds per-token/per-request USD prices as OpenRouter
// returns them: decimal strings, e.g. "0.000005" (per token).
type OpenRouterPricing struct {
	Prompt          string `json:"prompt"`
	Completion      string `json:"completion"`
	Request         string `json:"request"`
	Image           string `json:"image"`
	WebSearch       string `json:"web_search"`
	InputCacheRead  string `json:"input_cache_read"`
	InputCacheWrite string `json:"input_cache_write"`
}

// OpenRouterTopProvider holds the serving provider's effective limits.
type OpenRouterTopProvider struct {
	ContextLength       int64 `json:"context_length"`
	MaxCompletionTokens int64 `json:"max_completion_tokens"`
	IsModerated         bool  `json:"is_moderated"`
}

// OpenRouterCatalogModel is one entry from OpenRouter's public model catalog
// (GET /api/v1/models) — the full set of models routable through OpenRouter,
// distinct from the admin-curated ai_models table (see AIModelsService).
type OpenRouterCatalogModel struct {
	ID                  string                 `json:"id"`
	CanonicalSlug       string                 `json:"canonical_slug"`
	Name                string                 `json:"name"`
	Created             int64                  `json:"created"`
	Description         string                 `json:"description"`
	ContextLength       int64                  `json:"context_length"`
	Architecture        OpenRouterArchitecture `json:"architecture"`
	Pricing             OpenRouterPricing      `json:"pricing"`
	TopProvider         OpenRouterTopProvider  `json:"top_provider"`
	SupportedParameters []string               `json:"supported_parameters"`
}

// FetchOpenRouterModels queries OpenRouter's public model catalog. The
// endpoint does not require authentication.
func FetchOpenRouterModels(ctx context.Context) ([]OpenRouterCatalogModel, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, openRouterModelsURL, nil)
	if err != nil {
		return nil, err
	}
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
		return nil, fmt.Errorf("openRouter models API %d: %s", resp.StatusCode, string(data))
	}
	var wrapper struct {
		Data []OpenRouterCatalogModel `json:"data"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Data, nil
}
