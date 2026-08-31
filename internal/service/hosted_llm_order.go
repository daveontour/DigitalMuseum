package service

import (
	"context"
	"encoding/json"
	"strings"
)

// HostedLLMProviderOrderConfigKey is the app_configuration key for the Auto-routing
// classifier provider and the error-failover on/off toggle.
const HostedLLMProviderOrderConfigKey = "hosted_llm_provider_order_v1"

// DefaultClassifierProvider is the default AI provider for Auto routing classification.
const DefaultClassifierProvider = "localai"

// HostedLLMProviderOrderConfig is persisted under HostedLLMProviderOrderConfigKey. Hosted
// provider try order for both Auto routing and error failover always follows the AI Models
// tab's sort_order (see defaultHostedLLMProviderOrder) — only the classifier provider and
// the failover on/off toggle are independently configurable.
type HostedLLMProviderOrderConfig struct {
	ClassifierProvider string
	FailoverEnabled    bool
}

type hostedLLMProviderOrderConfig struct {
	ClassifierProvider string `json:"classifier_provider"`
	FailoverEnabled    *bool  `json:"failover_enabled"`
}

// defaultHostedLLMProviderOrder returns every enabled AI Models row's key ordered by
// sort_order — including "localai" at its own position, unlike ListEnabled — the single
// source of truth for both Auto routing and error failover try order (see the AI Models
// config tab, where Local AI can be enabled/disabled and reordered like any other row).
func (s *ChatService) defaultHostedLLMProviderOrder(ctx context.Context) []string {
	if s.aiModelsSvc == nil {
		return nil
	}
	models, _ := s.aiModelsSvc.ListAll(ctx)
	out := make([]string, 0, len(models))
	for _, m := range models {
		if m.Enabled {
			out = append(out, m.Key)
		}
	}
	return out
}

// isValidHostedProviderName reports whether name is a currently-enabled AI model key
// (including "localai").
func (s *ChatService) isValidHostedProviderName(ctx context.Context, name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || s.aiModelsSvc == nil {
		return false
	}
	if name == "localai" {
		return s.aiModelsSvc.LocalAIRowEnabled(ctx)
	}
	return s.aiModelsSvc.IsEnabledKey(ctx, name)
}

func (s *ChatService) isValidClassifierProviderName(ctx context.Context, name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "localai" {
		return true
	}
	return s.isValidHostedProviderName(ctx, name)
}

func (s *ChatService) normalizeClassifierProvider(ctx context.Context, name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if s.isValidClassifierProviderName(ctx, name) {
		return name
	}
	return DefaultClassifierProvider
}

// HostedProviderTryOrder builds try order for Auto routing: last manually selected provider
// first, then the AI Models tab's sort_order.
func (s *ChatService) HostedProviderTryOrder(ctx context.Context, lastManualHosted string) []string {
	order := s.defaultHostedLLMProviderOrder(ctx)
	last := strings.ToLower(strings.TrimSpace(lastManualHosted))
	if !s.isValidHostedProviderName(ctx, last) {
		return order
	}
	out := []string{last}
	for _, p := range order {
		if p != last {
			out = append(out, p)
		}
	}
	return out
}

// FailoverProviderTryOrder returns the AI Models tab's sort_order, excluding the failed
// primary provider.
func (s *ChatService) FailoverProviderTryOrder(ctx context.Context, primary string) []string {
	primary = strings.ToLower(strings.TrimSpace(primary))
	order := s.defaultHostedLLMProviderOrder(ctx)
	var out []string
	for _, p := range order {
		if p != primary {
			out = append(out, p)
		}
	}
	return out
}

func (s *ChatService) parseHostedLLMProviderOrderJSON(ctx context.Context, raw string) HostedLLMProviderOrderConfig {
	out := HostedLLMProviderOrderConfig{
		ClassifierProvider: DefaultClassifierProvider,
		FailoverEnabled:    true,
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out
	}
	var cfg hostedLLMProviderOrderConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return out
	}
	out.ClassifierProvider = s.normalizeClassifierProvider(ctx, cfg.ClassifierProvider)
	if cfg.FailoverEnabled != nil {
		out.FailoverEnabled = *cfg.FailoverEnabled
	}
	return out
}

func (s *ChatService) loadHostedLLMProviderOrderConfig(ctx context.Context) HostedLLMProviderOrderConfig {
	out := HostedLLMProviderOrderConfig{
		ClassifierProvider: DefaultClassifierProvider,
		FailoverEnabled:    true,
	}
	if s.configRepo == nil {
		return out
	}
	raw, err := s.configRepo.GetValueByKey(ctx, HostedLLMProviderOrderConfigKey)
	if err != nil || raw == nil {
		return out
	}
	return s.parseHostedLLMProviderOrderJSON(ctx, *raw)
}
