package ai

import "testing"

func TestOllamaModelBaseName(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"gemma4:latest", "gemma4"},
		{"Gemma4", "gemma4"},
		{"embeddinggemma", "embeddinggemma"},
		{"  nomic-embed-text:v1.5 ", "nomic-embed-text"},
	}
	for _, tc := range tests {
		if got := OllamaModelBaseName(tc.in); got != tc.want {
			t.Fatalf("OllamaModelBaseName(%q) = %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestModelPresent(t *testing.T) {
	tags := []string{"gemma4:latest", "embeddinggemma:latest", "llama3.2:1b"}
	tests := []struct {
		configured string
		want       bool
	}{
		{"gemma4", true},
		{"gemma4:latest", true},
		{"embeddinggemma", true},
		{"local-model", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := ModelPresent(tags, tc.configured); got != tc.want {
			t.Fatalf("ModelPresent(..., %q) = %v want %v", tc.configured, got, tc.want)
		}
	}
}

func TestResolveEmbeddingModelName(t *testing.T) {
	if got := ResolveEmbeddingModelName("", "gemma4"); got != "gemma4" {
		t.Fatalf("fallback got %q", got)
	}
	if got := ResolveEmbeddingModelName("embeddinggemma", "gemma4"); got != "embeddinggemma" {
		t.Fatalf("explicit got %q", got)
	}
}

func TestProbeOllamaTagsEmptyBaseURL(t *testing.T) {
	r := ProbeOllamaTags(t.Context(), "", "gemma4", "embeddinggemma")
	if r.BaseURLConfigured || r.ServerReachable {
		t.Fatalf("expected unconfigured: %+v", r)
	}
}

func TestProbeOllamaDualEmptyURLs(t *testing.T) {
	r := ProbeOllamaDual(t.Context(), "", "", "gemma4", "embeddinggemma")
	if r.BaseURLConfigured || r.EmbeddingBaseURLConfigured {
		t.Fatalf("expected both unconfigured: %+v", r)
	}
	if r.EmbeddingModel != "embeddinggemma" {
		t.Fatalf("embedding model name %q", r.EmbeddingModel)
	}
}
