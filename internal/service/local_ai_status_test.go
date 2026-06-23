package service

import (
	"testing"

	appai "github.com/daveontour/aimuseum/internal/ai"
)

func TestLocalAIStatusReportFromProbe(t *testing.T) {
	s := &ChatService{
		defaultLocalAIURL:            "http://localhost:11434",
		defaultLocalAIModel:          "gemma4",
		defaultLocalAIEmbeddingModel: "embeddinggemma",
	}

	probe := appai.OllamaProbeResult{
		BaseURLConfigured:       true,
		BaseURL:                 "http://localhost:11434",
		ServerReachable:         true,
		ChatModel:               "gemma4",
		ChatModelAvailable:      true,
		EmbeddingModel:          "embeddinggemma",
		EmbeddingModelAvailable: true,
		EmbeddingBaseURL:        "http://localhost:11435",
		EmbeddingServerReachable: true,
	}

	report := s.buildLocalAIStatusReport(t.Context(), probe)
	if !report.InfrastructureAvailable {
		t.Fatal("expected infrastructure available")
	}
	if !report.EmbeddingServerReachable {
		t.Fatal("expected embedding server reachable in report")
	}
	if report.UseEnabledForChat != nil {
		t.Fatal("unauthenticated should omit use_enabled")
	}
}

func TestLocalAIStatusInfraFalseWhenUnreachable(t *testing.T) {
	s := &ChatService{}
	probe := appai.OllamaProbeResult{
		BaseURLConfigured: true,
		BaseURL:           "http://localhost:11434",
		ServerReachable:   false,
		ServerError:       "connection refused",
		ChatModel:         "gemma4",
	}
	report := s.buildLocalAIStatusReport(t.Context(), probe)
	if report.InfrastructureAvailable {
		t.Fatal("expected infra false when unreachable")
	}
}
