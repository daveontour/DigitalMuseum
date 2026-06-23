package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const ollamaTagsProbeTimeout = 3 * time.Second

// OllamaProbeResult holds the outcome of probing Ollama server(s).
type OllamaProbeResult struct {
	BaseURLConfigured       bool
	BaseURL                 string
	ServerReachable         bool
	ServerError             string
	ChatModel               string
	ChatModelAvailable      bool
	EmbeddingModel          string
	EmbeddingModelAvailable bool
	// Embedding server (separate Ollama daemon when LOCALAI_EMBEDDING_BASE_URL is set).
	EmbeddingBaseURLConfigured bool
	EmbeddingBaseURL           string
	EmbeddingServerReachable   bool
	EmbeddingServerError       string
}

type ollamaTagsResponse struct {
	Models []struct {
		Name  string `json:"name"`
		Model string `json:"model"`
	} `json:"models"`
}

// NormalizeOllamaModelName strips whitespace and lowercases for comparison.
func NormalizeOllamaModelName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// OllamaModelBaseName returns the portion before the first colon (e.g. gemma4 from gemma4:latest).
func OllamaModelBaseName(name string) string {
	n := NormalizeOllamaModelName(name)
	if i := strings.Index(n, ":"); i >= 0 {
		return n[:i]
	}
	return n
}

// ModelPresent reports whether configuredName matches any tag in the Ollama model list.
func ModelPresent(tagNames []string, configuredName string) bool {
	want := OllamaModelBaseName(configuredName)
	if want == "" {
		return false
	}
	for _, tag := range tagNames {
		if OllamaModelBaseName(tag) == want {
			return true
		}
	}
	return false
}

// ResolveEmbeddingModelName returns the embedding model name, falling back to chatModel when unset.
func ResolveEmbeddingModelName(embeddingModel, chatModel string) string {
	if em := strings.TrimSpace(embeddingModel); em != "" {
		return em
	}
	return strings.TrimSpace(chatModel)
}

// ProbeOllamaTags GETs {baseURL}/api/tags and checks chat and embedding models on one server.
func ProbeOllamaTags(ctx context.Context, baseURL, chatModel, embeddingModel string) OllamaProbeResult {
	return ProbeOllamaDual(ctx, baseURL, baseURL, chatModel, embeddingModel)
}

// ProbeOllamaDual probes chat and embedding models on separate Ollama base URLs.
func ProbeOllamaDual(ctx context.Context, chatBaseURL, embedBaseURL, chatModel, embeddingModel string) OllamaProbeResult {
	embedName := ResolveEmbeddingModelName(embeddingModel, chatModel)
	chatProbe := probeOllamaServer(ctx, chatBaseURL, strings.TrimSpace(chatModel))
	embedProbe := probeOllamaServer(ctx, embedBaseURL, embedName)

	return OllamaProbeResult{
		BaseURLConfigured:          chatProbe.baseURLConfigured,
		BaseURL:                    chatProbe.baseURL,
		ServerReachable:            chatProbe.serverReachable,
		ServerError:                chatProbe.serverError,
		ChatModel:                  strings.TrimSpace(chatModel),
		ChatModelAvailable:         chatProbe.modelAvailable,
		EmbeddingModel:             embedName,
		EmbeddingModelAvailable:    embedProbe.modelAvailable,
		EmbeddingBaseURLConfigured: embedProbe.baseURLConfigured,
		EmbeddingBaseURL:           embedProbe.baseURL,
		EmbeddingServerReachable:   embedProbe.serverReachable,
		EmbeddingServerError:       embedProbe.serverError,
	}
}

type ollamaServerProbe struct {
	baseURLConfigured bool
	baseURL           string
	serverReachable   bool
	serverError       string
	modelAvailable    bool
}

func probeOllamaServer(ctx context.Context, baseURL, modelName string) ollamaServerProbe {
	out := ollamaServerProbe{}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return out
	}
	out.baseURLConfigured = true
	out.baseURL = baseURL

	reqCtx, cancel := context.WithTimeout(ctx, ollamaTagsProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+"/api/tags", nil)
	if err != nil {
		out.serverError = err.Error()
		return out
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		out.serverError = err.Error()
		return out
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		out.serverError = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		return out
	}
	var tags ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		out.serverError = fmt.Sprintf("decode tags: %v", err)
		return out
	}
	out.serverReachable = true

	var names []string
	for _, m := range tags.Models {
		if n := strings.TrimSpace(m.Name); n != "" {
			names = append(names, n)
		} else if n := strings.TrimSpace(m.Model); n != "" {
			names = append(names, n)
		}
	}
	out.modelAvailable = ModelPresent(names, modelName)
	return out
}
