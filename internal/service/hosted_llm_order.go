package service

import (
	"context"
	"encoding/json"
	"strings"
)

// HostedLLMProviderOrderConfigKey is the app_configuration key for hosted provider try order.
const HostedLLMProviderOrderConfigKey = "hosted_llm_provider_order_v1"

// DefaultHostedLLMProviderOrder is the fallback try order for Gemini, Claude, and DeepSeek.
var DefaultHostedLLMProviderOrder = []string{"gemini", "claude", "deepseek", "openai"}

// DefaultClassifierProvider is the default AI provider for Auto routing classification.
const DefaultClassifierProvider = "localai"

// HostedLLMProviderOrderConfig is persisted under HostedLLMProviderOrderConfigKey.
type HostedLLMProviderOrderConfig struct {
	AutoOrder          []string
	FailoverOrder      []string
	ClassifierProvider string
}

type hostedLLMProviderOrderConfig struct {
	AutoOrder          []string `json:"auto_order"`
	FailoverOrder      []string `json:"failover_order"`
	ClassifierProvider string   `json:"classifier_provider"`
}

func isValidClassifierProviderName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "localai", "gemini", "claude", "deepseek", "openai":
		return true
	default:
		return false
	}
}

func normalizeClassifierProvider(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if isValidClassifierProviderName(name) {
		return name
	}
	return DefaultClassifierProvider
}

func normalizeHostedProviderOrder(order []string) []string {
	valid := map[string]bool{"gemini": true, "claude": true, "deepseek": true, "openai": true}
	var out []string
	seen := map[string]bool{}
	for _, p := range order {
		p = strings.ToLower(strings.TrimSpace(p))
		if !valid[p] || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, p := range DefaultHostedLLMProviderOrder {
		if !seen[p] {
			out = append(out, p)
		}
	}
	return out
}

// HostedProviderTryOrder builds try order for Auto routing: last manually selected provider first, then auto_order.
func HostedProviderTryOrder(lastManualHosted string, configuredOrder []string) []string {
	order := normalizeHostedProviderOrder(configuredOrder)
	last := strings.ToLower(strings.TrimSpace(lastManualHosted))
	if !isValidHostedProviderName(last) {
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

// FailoverProviderTryOrder returns configured failover order excluding the failed primary provider.
func FailoverProviderTryOrder(primary string, configuredOrder []string) []string {
	primary = strings.ToLower(strings.TrimSpace(primary))
	order := normalizeHostedProviderOrder(configuredOrder)
	var out []string
	for _, p := range order {
		if p != primary {
			out = append(out, p)
		}
	}
	return out
}

func parseHostedLLMProviderOrderJSON(raw string) HostedLLMProviderOrderConfig {
	out := HostedLLMProviderOrderConfig{
		AutoOrder:          append([]string(nil), DefaultHostedLLMProviderOrder...),
		FailoverOrder:      append([]string(nil), DefaultHostedLLMProviderOrder...),
		ClassifierProvider: DefaultClassifierProvider,
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out
	}
	var cfg hostedLLMProviderOrderConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return out
	}
	if len(cfg.AutoOrder) > 0 {
		out.AutoOrder = normalizeHostedProviderOrder(cfg.AutoOrder)
	}
	if len(cfg.FailoverOrder) > 0 {
		out.FailoverOrder = normalizeHostedProviderOrder(cfg.FailoverOrder)
	}
	out.ClassifierProvider = normalizeClassifierProvider(cfg.ClassifierProvider)
	return out
}

func (s *ChatService) loadHostedLLMProviderOrderConfig(ctx context.Context) HostedLLMProviderOrderConfig {
	out := HostedLLMProviderOrderConfig{
		AutoOrder:          append([]string(nil), DefaultHostedLLMProviderOrder...),
		FailoverOrder:      append([]string(nil), DefaultHostedLLMProviderOrder...),
		ClassifierProvider: DefaultClassifierProvider,
	}
	if s.configRepo == nil {
		return out
	}
	raw, err := s.configRepo.GetValueByKey(ctx, HostedLLMProviderOrderConfigKey)
	if err != nil || raw == nil {
		return out
	}
	return parseHostedLLMProviderOrderJSON(*raw)
}

func (s *ChatService) loadHostedLLMProviderOrder(ctx context.Context) (autoOrder, failoverOrder []string) {
	cfg := s.loadHostedLLMProviderOrderConfig(ctx)
	return cfg.AutoOrder, cfg.FailoverOrder
}
