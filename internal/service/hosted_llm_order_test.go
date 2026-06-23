package service

import "testing"

func TestNormalizeHostedProviderOrder(t *testing.T) {
	got := normalizeHostedProviderOrder([]string{"deepseek", "claude", "invalid", "deepseek"})
	want := []string{"deepseek", "claude", "gemini", "openai"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestHostedProviderTryOrder(t *testing.T) {
	tests := []struct {
		lastManual string
		configured []string
		want       []string
	}{
		{"claude", []string{"gemini", "claude", "deepseek"}, []string{"claude", "gemini", "deepseek", "openai"}},
		{"", []string{"deepseek", "gemini", "claude"}, []string{"deepseek", "gemini", "claude", "openai"}},
		{"invalid", []string{"claude", "gemini", "deepseek"}, []string{"claude", "gemini", "deepseek", "openai"}},
	}
	for _, tc := range tests {
		got := HostedProviderTryOrder(tc.lastManual, tc.configured)
		if len(got) != len(tc.want) {
			t.Fatalf("lastManual=%q: got %v want %v", tc.lastManual, got, tc.want)
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Fatalf("lastManual=%q: got %v want %v", tc.lastManual, got, tc.want)
			}
		}
	}
}

func TestFailoverProviderTryOrder(t *testing.T) {
	got := FailoverProviderTryOrder("gemini", []string{"gemini", "claude", "deepseek"})
	want := []string{"claude", "deepseek", "openai"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestParseHostedLLMProviderOrderJSON(t *testing.T) {
	cfg := parseHostedLLMProviderOrderJSON(`{"auto_order":["claude","gemini","deepseek"],"failover_order":["deepseek","claude","gemini"],"classifier_provider":"gemini"}`)
	if cfg.AutoOrder[0] != "claude" || cfg.FailoverOrder[0] != "deepseek" {
		t.Fatalf("auto=%v failover=%v", cfg.AutoOrder, cfg.FailoverOrder)
	}
	if cfg.ClassifierProvider != "gemini" {
		t.Fatalf("classifier=%q want gemini", cfg.ClassifierProvider)
	}
	if !cfg.FailoverEnabled {
		t.Fatalf("failover_enabled default true, got false")
	}
	cfgDisabled := parseHostedLLMProviderOrderJSON(`{"failover_enabled":false}`)
	if cfgDisabled.FailoverEnabled {
		t.Fatalf("failover_enabled=false not parsed")
	}
	cfgDefault := parseHostedLLMProviderOrderJSON("")
	if len(cfgDefault.FailoverOrder) != 4 || cfgDefault.FailoverOrder[0] != "gemini" {
		t.Fatalf("default failover=%v", cfgDefault.FailoverOrder)
	}
	if cfgDefault.ClassifierProvider != DefaultClassifierProvider {
		t.Fatalf("default classifier=%q", cfgDefault.ClassifierProvider)
	}
}

func TestNormalizeClassifierProvider(t *testing.T) {
	if got := normalizeClassifierProvider("claude"); got != "claude" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeClassifierProvider("invalid"); got != DefaultClassifierProvider {
		t.Fatalf("got %q", got)
	}
}
