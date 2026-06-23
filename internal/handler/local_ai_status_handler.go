package handler

import (
	"net/http"

	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/service"
)

// LocalAIStatusFromConfig handles GET /api/local-ai/status when no archive DB is open.
func LocalAIStatusFromConfig(ai config.AIConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chatModel := config.LocalAIRuntimeStore().ChatModel()
		if chatModel == "" {
			chatModel = ai.LocalAIModelName
		}
		writeJSON(w, service.InfrastructureLocalAIStatus(
			r.Context(),
			ai.LocalAIBaseURL,
			ai.LocalAIEmbeddingBaseURL,
			chatModel,
			ai.LocalAIEmbeddingModel,
		))
	}
}
