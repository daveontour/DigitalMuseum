package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/daveontour/aimuseum/internal/ai"
)

// EmbeddingService generates embedding vectors using the local AI server's
// /v1/embeddings endpoint (OpenAI-compatible). It requires a LocalAIProvider
// with a configured base URL and a separate embedding model name (typically
// different from the chat completion model, e.g. "nomic-embed-text").
type EmbeddingService struct {
	provider       *ai.LocalAIProvider
	embeddingModel string
}

// NewEmbeddingService creates an EmbeddingService.
// provider must be non-nil and available.
// embeddingModel is the model name passed to /v1/embeddings; if blank the
// provider's chat model name is used as a fallback.
func NewEmbeddingService(provider *ai.LocalAIProvider, embeddingModel string) *EmbeddingService {
	return &EmbeddingService{
		provider:       provider,
		embeddingModel: strings.TrimSpace(embeddingModel),
	}
}

// IsAvailable reports whether the service can produce embeddings.
func (s *EmbeddingService) IsAvailable() bool {
	return s != nil && s.provider != nil && s.provider.IsAvailable()
}

// EmbedText returns an embedding vector for the supplied text.
// Returns an error if the service is not available or the server call fails.
func (s *EmbeddingService) EmbedText(ctx context.Context, text string) ([]float32, error) {
	if !s.IsAvailable() {
		return nil, fmt.Errorf("embedding service: local AI not configured")
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("embedding service: text must not be empty")
	}
	return s.provider.Embed(ctx, text, s.embeddingModel)
}

// EmbedTextWithModel returns an embedding vector using an explicit model name.
// If model is blank, it falls back to the service default model behavior.
func (s *EmbeddingService) EmbedTextWithModel(ctx context.Context, text, model string) ([]float32, error) {
	if !s.IsAvailable() {
		return nil, fmt.Errorf("embedding service: local AI not configured")
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("embedding service: text must not be empty")
	}
	m := strings.TrimSpace(model)
	if m == "" {
		return s.provider.Embed(ctx, text, s.embeddingModel)
	}
	return s.provider.Embed(ctx, text, m)
}

// EmbedBatch returns embedding vectors for each string in texts.
// Each entry is embedded with a separate API call; the slice is returned in
// the same order as the input. If any call fails the whole batch fails.
func (s *EmbeddingService) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if !s.IsAvailable() {
		return nil, fmt.Errorf("embedding service: local AI not configured")
	}
	results := make([][]float32, len(texts))
	for i, t := range texts {
		vec, err := s.EmbedText(ctx, t)
		if err != nil {
			return nil, fmt.Errorf("embedding service: batch item %d: %w", i, err)
		}
		results[i] = vec
	}
	return results, nil
}
