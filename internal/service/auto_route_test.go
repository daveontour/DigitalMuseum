package service

import (
	"strings"
	"testing"
)

func TestParseAutoClassifierResponse(t *testing.T) {
	t.Run("valid local", func(t *testing.T) {
		dec, err := parseAutoClassifierResponse(`{"route":"local","reason":"simple time query","confidence":0.95,"needs_reference_documents":false,"needs_user_profile":false}`)
		if err != nil {
			t.Fatal(err)
		}
		if dec.Decision != "local" {
			t.Fatalf("decision=%q want local", dec.Decision)
		}
		if dec.Reason != "simple time query" {
			t.Fatalf("reason=%q", dec.Reason)
		}
		if dec.Confidence != 0.95 {
			t.Fatalf("confidence=%v", dec.Confidence)
		}
		if dec.NeedsReferenceDocuments {
			t.Fatal("expected needs_reference_documents false")
		}
		if dec.NeedsUserProfile {
			t.Fatal("expected needs_user_profile false")
		}
	})

	t.Run("valid hosted with fences", func(t *testing.T) {
		raw := "```json\n{\"route\":\"hosted\",\"reason\":\"needs narrative synthesis\",\"confidence\":0.8}\n```"
		dec, err := parseAutoClassifierResponse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if dec.Decision != "hosted" {
			t.Fatalf("decision=%q want hosted", dec.Decision)
		}
	})

	t.Run("invalid route", func(t *testing.T) {
		_, err := parseAutoClassifierResponse(`{"route":"cloud","reason":"x"}`)
		if err == nil {
			t.Fatal("expected error for invalid route")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := parseAutoClassifierResponse(`not json`)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("missing context flags default true", func(t *testing.T) {
		dec, err := parseAutoClassifierResponse(`{"route":"local","reason":"lookup"}`)
		if err != nil {
			t.Fatal(err)
		}
		if !dec.NeedsReferenceDocuments || !dec.NeedsUserProfile {
			t.Fatalf("expected default true context flags, got ref=%v profile=%v", dec.NeedsReferenceDocuments, dec.NeedsUserProfile)
		}
	})

	t.Run("empty reason gets default", func(t *testing.T) {
		dec, err := parseAutoClassifierResponse(`{"route":"local","reason":""}`)
		if err != nil {
			t.Fatal(err)
		}
		if dec.Reason != "No reason provided" {
			t.Fatalf("reason=%q", dec.Reason)
		}
	})
}

func TestHostedProviderTryOrder(t *testing.T) {
	tests := []struct {
		lastManual string
		want       []string
	}{
		{"claude", []string{"claude", "gemini", "deepseek"}},
		{"gemini", []string{"gemini", "claude", "deepseek"}},
		{"", []string{"gemini", "claude", "deepseek"}},
		{"invalid", []string{"gemini", "claude", "deepseek"}},
		{"deepseek", []string{"deepseek", "gemini", "claude"}},
	}
	for _, tc := range tests {
		got := hostedProviderTryOrder(tc.lastManual)
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

func TestBuildAutoClassifierPrompt(t *testing.T) {
	p := buildAutoClassifierPrompt("What time is it?", 12, 3, true)
	if p == "" {
		t.Fatal("empty prompt")
	}
	if !strings.Contains(p, "12") || !strings.Contains(p, "3 reference") || !strings.Contains(p, "What time is it?") || !strings.Contains(p, `"route"`) || !strings.Contains(p, "needs_reference_documents") {
		t.Fatalf("prompt missing expected content: %q", p)
	}
}
