package service

import (
	"context"
	"strings"

	appai "github.com/daveontour/aimuseum/internal/ai"
)

// LocalAIUseEnabledConfigKey is the app_configuration key for per-archive Local AI chat use.
const LocalAIUseEnabledConfigKey = "local_ai_use_enabled_v1"

func parseLocalAIUseEnabled(raw *string) bool {
	if raw == nil {
		return true
	}
	v := strings.TrimSpace(strings.ToLower(*raw))
	switch v {
	case "false", "0", "no", "off":
		return false
	default:
		return true
	}
}

// LocalAIUseEnabled reports whether this archive allows Local AI for chat and related LLM features.
// Default is true when unset.
func (s *ChatService) LocalAIUseEnabled(ctx context.Context) bool {
	if s.configRepo == nil {
		return true
	}
	raw, err := s.configRepo.GetValueByKey(ctx, LocalAIUseEnabledConfigKey)
	if err != nil || raw == nil {
		return true
	}
	return parseLocalAIUseEnabled(raw)
}

// LocalAIInfrastructureAvailable reports whether Ollama is configured, reachable, and has the chat model.
func (s *ChatService) LocalAIInfrastructureAvailable(ctx context.Context) bool {
	probe := s.probeOllamaCached(ctx)
	return probe.BaseURLConfigured && probe.ServerReachable && probe.ChatModelAvailable
}

// localAIProviderForChat returns the Local AI provider when infrastructure is up and use is enabled.
func (s *ChatService) localAIProviderForChat(ctx context.Context) appai.ChatProvider {
	if !s.LocalAIUseEnabled(ctx) {
		return nil
	}
	p := s.effectiveLocalAIProvider()
	if p == nil || !p.IsAvailable() {
		return nil
	}
	return p
}
