package service

import (
	"strings"
	"testing"

	appai "github.com/daveontour/aimuseum/internal/ai"
)

func TestBuildJevModelCriteria(t *testing.T) {
	models := []AIModel{
		{Key: "localai", DisplayName: "Local AI", Enabled: true},
		{Key: "gemini", DisplayName: "Gemini", ModelSlug: "google/gemini-2.5-flash", Enabled: true},
		{Key: "claude", DisplayName: "Claude", ModelSlug: "anthropic/claude-sonnet-4.5", Enabled: true},
	}
	catalog := []appai.OpenRouterCatalogModel{
		{
			ID:            "google/gemini-2.5-flash",
			Name:          "Gemini 2.5 Flash",
			Description:   "Fast general model",
			ContextLength: 1000000,
			Pricing: appai.OpenRouterPricing{
				Prompt:     "0.0000003",
				Completion: "0.0000025",
			},
		},
	}
	got := buildJevModelCriteria(models, catalog)
	if !strings.Contains(got["localai"], "no per-token API cost") {
		t.Fatalf("local criterion: %q", got["localai"])
	}
	gemini := got["gemini"]
	for _, want := range []string{"Gemini", "google/gemini-2.5-flash", "Fast general model", "1000000", "$0.3 per 1M tokens", "$2.5 per 1M tokens"} {
		if !strings.Contains(gemini, want) {
			t.Fatalf("gemini criterion missing %q: %q", want, gemini)
		}
	}
	if strings.Contains(got["claude"], "per 1M tokens") {
		t.Fatalf("claude should omit price when the catalog has no match: %q", got["claude"])
	}
	if !strings.Contains(got["claude"], "anthropic/claude-sonnet-4.5") {
		t.Fatalf("claude criterion: %q", got["claude"])
	}
}

func TestParseJevDecision(t *testing.T) {
	allowed := map[string]struct{}{"gemini": {}, "claude": {}}
	raw := []byte(`{
		"model": "typesafe/jev-1.13-20260917",
		"answers": {
			"model": {"type": "choice", "choice": "gemini", "confidence": 0.81, "probabilities": {"gemini": 0.9, "claude": 0.1}},
			"needs_reference_documents": {"type": "noul", "noul": 0.2},
			"needs_user_profile": {"type": "noul", "noul": 0.9}
		}
	}`)
	dec, err := parseJevDecision(raw, allowed)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Decision != "gemini" {
		t.Fatalf("decision=%q", dec.Decision)
	}
	if dec.Confidence != 0.81 {
		t.Fatalf("confidence=%v", dec.Confidence)
	}
	if dec.NeedsReferenceDocuments {
		t.Fatal("expected needs_reference_documents false")
	}
	if !dec.NeedsUserProfile {
		t.Fatal("expected needs_user_profile true")
	}
	if dec.ClassifierProvider != jevClassifierProvider {
		t.Fatalf("provider=%q", dec.ClassifierProvider)
	}

	t.Run("missing context flags default true", func(t *testing.T) {
		body := []byte(`{"answers":{"model":{"choice":"claude","confidence":0.4}}}`)
		dec, err := parseJevDecision(body, allowed)
		if err != nil {
			t.Fatal(err)
		}
		if !dec.NeedsReferenceDocuments || !dec.NeedsUserProfile {
			t.Fatalf("expected default true context flags, got ref=%v profile=%v", dec.NeedsReferenceDocuments, dec.NeedsUserProfile)
		}
	})

	t.Run("unknown choice", func(t *testing.T) {
		body := []byte(`{"answers":{"model":{"choice":"openai","confidence":0.9}}}`)
		if _, err := parseJevDecision(body, allowed); err == nil {
			t.Fatal("expected error for unknown choice")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		if _, err := parseJevDecision([]byte(`not json`), allowed); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestHostedProviderTryOrderLegacy(t *testing.T) {
	s, ctx := newTestChatServiceWithModels(t, "gemini", "claude", "deepseek", "openai")
	tests := []struct {
		lastManual string
		want       []string
	}{
		{"claude", []string{"claude", "gemini", "deepseek", "openai"}},
		{"gemini", []string{"gemini", "claude", "deepseek", "openai"}},
		{"", []string{"gemini", "claude", "deepseek", "openai"}},
		{"invalid", []string{"gemini", "claude", "deepseek", "openai"}},
		{"deepseek", []string{"deepseek", "gemini", "claude", "openai"}},
	}
	for _, tc := range tests {
		got := s.HostedProviderTryOrder(ctx, tc.lastManual)
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
