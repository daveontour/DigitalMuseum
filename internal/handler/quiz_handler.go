package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/daveontour/aimuseum/internal/importer"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// QuizHandler serves the unauthenticated "Take the Quiz" login-page feature.
type QuizHandler struct {
	chatSvc    *service.ChatService
	visitorSvc *service.VisitorService
}

// NewQuizHandler creates a QuizHandler.
func NewQuizHandler(chatSvc *service.ChatService, visitorSvc *service.VisitorService) *QuizHandler {
	return &QuizHandler{chatSvc: chatSvc, visitorSvc: visitorSvc}
}

// RegisterRoutes mounts quiz endpoints. Exempt from auth middleware
// (see exemptPrefixes in internal/middleware/auth.go).
func (h *QuizHandler) RegisterRoutes(r chi.Router) {
	r.Post("/api/quiz/generate", h.Generate)
	r.Get("/api/quiz/generate/stream", h.Stream)
	r.Get("/api/quiz/generate/status", h.Status)
}

// quizJob tracks the single in-flight quiz generation (a global singleton, matching the
// pattern used by every other long-running background job in this app — see
// internal/importer/job.go). Only one quiz is generated at a time.
var quizJob = importer.NewImportJob("Quiz generation", map[string]any{
	"status": "idle", "status_line": nil, "error_message": nil,
	"provider": nil, "tool": nil,
	"subject_name": nil, "questions": nil,
})

// quizProviderDisplayNames maps a provider's config name (as used by the Hosted LLM
// Order settings) to a friendly display name for the quiz's live progress feed.
var quizProviderDisplayNames = map[string]string{
	"claude":   "Claude",
	"gemini":   "Gemini",
	"deepseek": "DeepSeek",
	"openai":   "ChatGPT",
	"localai":  "the local AI",
}

// quizToolStatusPhrases maps a quiz tool name (see quizAllowedTools in
// internal/service/quiz_service.go) to a friendly present-participle phrase for the
// quiz's live progress feed.
var quizToolStatusPhrases = map[string]string{
	"get_user_interests":                "checking their interests",
	"get_unique_tags_count":             "reviewing photo tags",
	"get_available_reference_documents": "browsing reference documents",
	"get_reference_document":            "reading a reference document",
	"search_facebook_albums":            "searching Facebook albums",
	"get_album_images":                  "looking through a photo album",
	"search_facebook_posts":             "searching Facebook posts",
}

// quizProgressStatusLine turns a raw (provider, tool) progress report into a
// human-readable status line for the quiz page.
func quizProgressStatusLine(provider, tool string) string {
	name := quizProviderDisplayNames[provider]
	if name == "" {
		name = provider
	}
	if tool == "" {
		return fmt.Sprintf("Asking %s to write your quiz…", name)
	}
	action := quizToolStatusPhrases[tool]
	if action == "" {
		action = "using a tool"
	}
	return fmt.Sprintf("%s is %s…", name, action)
}

// POST /api/quiz/generate
// Body: { "include_mature": bool, "roast_mode": bool }
// Starts a background job that generates a fresh 10-question multiple-choice quiz about
// the archive subject. Returns immediately with {"status":"started"}; subscribe to
// GET /api/quiz/generate/stream for live progress (current AI provider / tool) and the
// final result, or poll GET /api/quiz/generate/status as a fallback.
func (h *QuizHandler) Generate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IncludeMature bool `json:"include_mature"`
		RoastMode     bool `json:"roast_mode"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := quizJob.AssertNotRunning(); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	quizJob.Start()
	quizJob.UpdateState(map[string]any{
		"status":        "in_progress",
		"status_line":   "Getting ready…",
		"error_message": nil,
		"provider":      nil,
		"tool":          nil,
		"subject_name":  nil,
		"questions":     nil,
	})
	quizJob.Broadcast("progress", quizJob.GetState())

	go h.runQuizGeneration(req.IncludeMature, req.RoastMode)

	writeJSON(w, map[string]any{"status": "started"})
}

// runQuizGeneration runs GenerateQuiz in the background and streams progress/results to
// quizJob's SSE subscribers. Uses context.Background() rather than the triggering
// request's context, since that request already returned before this goroutine finishes.
func (h *QuizHandler) runQuizGeneration(includeMature, roastMode bool) {
	defer quizJob.Finish()

	onProgress := func(provider, tool string) {
		quizJob.UpdateState(map[string]any{
			"provider":    provider,
			"tool":        tool,
			"status_line": quizProgressStatusLine(provider, tool),
		})
		quizJob.Broadcast("progress", quizJob.GetState())
	}

	result, err := h.chatSvc.GenerateQuiz(context.Background(), h.visitorSvc, includeMature, roastMode, onProgress)
	if err != nil {
		quizJob.UpdateState(map[string]any{
			"status":        "error",
			"status_line":   nil,
			"error_message": "could not generate quiz: " + err.Error(),
		})
		quizJob.Broadcast("error", quizJob.GetState())
		return
	}

	quizJob.UpdateState(map[string]any{
		"status":       "completed",
		"status_line":  nil,
		"subject_name": result.SubjectName,
		"questions":    result.Questions,
	})
	quizJob.Broadcast("completed", quizJob.GetState())
}

// GET /api/quiz/generate/stream — SSE progress feed for the running (or last) quiz job.
func (h *QuizHandler) Stream(w http.ResponseWriter, r *http.Request) {
	quizJob.ServeSSE(w, r)
}

// GET /api/quiz/generate/status — snapshot of current quiz job state (SSE fallback).
func (h *QuizHandler) Status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, quizJob.Status())
}
