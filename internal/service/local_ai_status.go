package service

import (
	"context"
	"strings"
	"time"

	appai "github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/appctx"
)

const localAIProbeCacheTTL = 20 * time.Second

type cachedOllamaProbe struct {
	result    appai.OllamaProbeResult
	expiresAt time.Time
}

// LocalAIStatusReport is returned by GET /api/local-ai/status.
type LocalAIStatusReport struct {
	BaseURLConfigured          bool   `json:"base_url_configured"`
	BaseURL                    string `json:"base_url,omitempty"`
	ServerReachable            bool   `json:"server_reachable"`
	ServerError                string `json:"server_error,omitempty"`
	ChatModel                  string `json:"chat_model"`
	ChatModelAvailable         bool   `json:"chat_model_available"`
	EmbeddingModel             string `json:"embedding_model"`
	EmbeddingModelAvailable    bool   `json:"embedding_model_available"`
	EmbeddingBaseURL           string `json:"embedding_base_url,omitempty"`
	EmbeddingServerReachable   bool   `json:"embedding_server_reachable"`
	EmbeddingServerError       string `json:"embedding_server_error,omitempty"`
	InfrastructureAvailable    bool   `json:"infrastructure_available"`
	UseEnabledForChat          *bool  `json:"use_enabled_for_chat,omitempty"`
	ChatAvailable              *bool  `json:"chat_available,omitempty"`
	Authenticated              bool   `json:"authenticated"`
}

func (s *ChatService) probeOllamaCached(ctx context.Context) appai.OllamaProbeResult {
	baseURL := strings.TrimSpace(s.defaultLocalAIURL)
	chatModel := s.effectiveLocalAIChatModel()
	cacheKey := baseURL + "|" + s.defaultLocalAIEmbeddingURL + "|" + chatModel + "|" + s.defaultLocalAIEmbeddingModel
	now := time.Now()
	if v, ok := s.localAIProbeCache.Load(cacheKey); ok {
		if entry, ok := v.(cachedOllamaProbe); ok && now.Before(entry.expiresAt) {
			return entry.result
		}
	}
	result := appai.ProbeOllamaDual(ctx, baseURL, s.defaultLocalAIEmbeddingURL, chatModel, s.defaultLocalAIEmbeddingModel)
	s.localAIProbeCache.Store(cacheKey, cachedOllamaProbe{result: result, expiresAt: now.Add(localAIProbeCacheTTL)})
	return result
}

// LocalAIStatus probes Ollama and returns status for the setup UI.
// When the request is unauthenticated, use_enabled_for_chat and chat_available are omitted.
func (s *ChatService) LocalAIStatus(ctx context.Context) LocalAIStatusReport {
	return s.buildLocalAIStatusReport(ctx, s.probeOllamaCached(ctx))
}

func (s *ChatService) buildLocalAIStatusReport(ctx context.Context, probe appai.OllamaProbeResult) LocalAIStatusReport {
	infra := probe.BaseURLConfigured && probe.ServerReachable && probe.ChatModelAvailable

	out := LocalAIStatusReport{
		BaseURLConfigured:       probe.BaseURLConfigured,
		BaseURL:                 probe.BaseURL,
		ServerReachable:         probe.ServerReachable,
		ServerError:             probe.ServerError,
		ChatModel:               probe.ChatModel,
		ChatModelAvailable:      probe.ChatModelAvailable,
		EmbeddingModel:          probe.EmbeddingModel,
		EmbeddingModelAvailable: probe.EmbeddingModelAvailable,
		EmbeddingBaseURL:        probe.EmbeddingBaseURL,
		EmbeddingServerReachable: probe.EmbeddingServerReachable,
		EmbeddingServerError:    probe.EmbeddingServerError,
		InfrastructureAvailable: infra,
	}

	uid := appctx.UserIDFromCtx(ctx)
	if uid != 0 {
		useEnabled := s.LocalAIUseEnabled(ctx)
		chatAvail := useEnabled && infra
		out.Authenticated = true
		out.UseEnabledForChat = &useEnabled
		out.ChatAvailable = &chatAvail
	}
	return out
}

// InfrastructureLocalAIStatus returns probe results without per-archive use_enabled fields.
func InfrastructureLocalAIStatus(ctx context.Context, baseURL, embeddingBaseURL, chatModel, embeddingModel string) LocalAIStatusReport {
	probe := appai.ProbeOllamaDual(ctx, baseURL, embeddingBaseURL, chatModel, embeddingModel)
	var s ChatService
	return s.buildLocalAIStatusReport(ctx, probe)
}

// InvalidateLocalAIProbeCache clears cached Ollama probe results (e.g. after model pull in tests).
func (s *ChatService) InvalidateLocalAIProbeCache() {
	s.localAIProbeCache.Range(func(key, _ any) bool {
		s.localAIProbeCache.Delete(key)
		return true
	})
}
