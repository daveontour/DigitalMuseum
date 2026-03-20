package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	appai "github.com/daveontour/digitalmuseum/internal/ai"
	"github.com/daveontour/digitalmuseum/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AdminHandler handles admin, control-panel, and AI summarization endpoints.
type AdminHandler struct {
	pool              *pgxpool.Pool
	subjectConfigRepo *repository.SubjectConfigRepo
	gemini            *appai.GeminiProvider
}

// NewAdminHandler creates an AdminHandler.
func NewAdminHandler(pool *pgxpool.Pool, subjectConfigRepo *repository.SubjectConfigRepo) *AdminHandler {
	return &AdminHandler{pool: pool, subjectConfigRepo: subjectConfigRepo}
}

// WithGemini injects a GeminiProvider for AI summarization.
func (h *AdminHandler) WithGemini(g *appai.GeminiProvider) { h.gemini = g }

// RegisterRoutes mounts all admin and AI routes.
func (h *AdminHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/import-control-last-run", h.GetImportControlLastRun)
	r.Get("/api/control-defaults", h.GetControlDefaults)
	r.Delete("/admin/empty-media-tables", h.DeleteEmptyMediaTables)
	r.Post("/writing-style/summarize", h.SummarizeWritingStyle)
	r.Post("/psychological-profile/summarize", h.SummarizePsychologicalProfile)
}

// GetImportControlLastRun handles GET /api/import-control-last-run.
// Returns the most recent created_at for each import data type.
func (h *AdminHandler) GetImportControlLastRun(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	type runInfo struct {
		LastRun *string `json:"last_run"`
	}

	type dataQuery struct {
		key string
		sql string
	}
	queries := []dataQuery{
		{"whatsapp", `SELECT MAX(created_at)::text FROM messages WHERE service = 'WhatsApp'`},
		{"imessage", `SELECT MAX(created_at)::text FROM messages WHERE service ILIKE '%iMessage%' OR service ILIKE '%SMS%'`},
		{"instagram", `SELECT MAX(created_at)::text FROM messages WHERE service = 'Instagram'`},
		{"facebook_messenger", `SELECT MAX(created_at)::text FROM messages WHERE service = 'Facebook Messenger'`},
		{"emails", `SELECT MAX(created_at)::text FROM emails`},
		{"images", `SELECT MAX(created_at)::text FROM media_items`},
	}

	result := make(map[string]runInfo, len(queries))
	for _, q := range queries {
		var ts *string
		_ = h.pool.QueryRow(ctx, q.sql).Scan(&ts)
		result[q.key] = runInfo{LastRun: ts}
	}
	writeJSON(w, result)
}

// GetControlDefaults handles GET /api/control-defaults.
// Returns app_configuration values useful for pre-filling import control forms.
func (h *AdminHandler) GetControlDefaults(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.pool.Query(ctx,
		`SELECT key, value FROM app_configuration
		 WHERE key ILIKE '%PATH%' OR key ILIKE '%DIRECTORY%' OR key ILIKE '%IMPORT%'`)
	if err != nil {
		writeJSON(w, map[string]any{})
		return
	}
	defer rows.Close()

	result := map[string]any{}
	for rows.Next() {
		var key string
		var value *string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		result[strings.ToLower(key)] = value
	}
	writeJSON(w, result)
}

// DeleteEmptyMediaTables handles DELETE /admin/empty-media-tables.
// Removes media_blobs with no data and media_items with no blob reference.
func (h *AdminHandler) DeleteEmptyMediaTables(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	blobTag, err := h.pool.Exec(ctx,
		`DELETE FROM media_blobs WHERE image_data IS NULL AND thumbnail_data IS NULL`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error deleting empty blobs: %s", err))
		return
	}

	itemTag, err := h.pool.Exec(ctx,
		`DELETE FROM media_items WHERE media_blob_id IS NULL`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error deleting orphan items: %s", err))
		return
	}

	writeJSON(w, map[string]any{
		"message":       "Empty media tables cleaned",
		"blobs_deleted": blobTag.RowsAffected(),
		"items_deleted": itemTag.RowsAffected(),
	})
}

// SummarizeWritingStyle handles POST /writing-style/summarize.
// Samples emails, asks Gemini to describe the subject's writing style,
// and stores the result in subject_configuration.writing_style_ai.
func (h *AdminHandler) SummarizeWritingStyle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.gemini == nil {
		writeError(w, http.StatusServiceUnavailable, "AI summarization is not configured")
		return
	}

	sample, err := h.sampleEmailsForAI(ctx, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error loading email samples: %s", err))
		return
	}
	if sample == "" {
		writeError(w, http.StatusUnprocessableEntity, "no emails available for analysis")
		return
	}

	prompt := fmt.Sprintf(`Analyse the writing style of the person who sent these emails. Describe their:
- Vocabulary and language complexity
- Sentence structure and length preferences
- Tone (formal/informal, warm/professional)
- Common phrases or patterns
- How they open and close emails
- Overall communication style

Email samples:
%s`, sample)

	result, err := h.gemini.GenerateResponse(ctx, appai.GenerateRequest{UserInput: prompt}, "", nil, nil, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("AI error: %s", err))
		return
	}

	if err := h.subjectConfigRepo.UpdateWritingStyleAI(ctx, result.PlainText); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error saving writing style: %s", err))
		return
	}

	writeJSON(w, map[string]any{
		"summary": result.PlainText,
		"message": "Writing style updated",
	})
}

// SummarizePsychologicalProfile handles POST /psychological-profile/summarize.
// Samples emails, asks Gemini to generate a psychological profile,
// and stores the result in subject_configuration.psychological_profile_ai.
func (h *AdminHandler) SummarizePsychologicalProfile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.gemini == nil {
		writeError(w, http.StatusServiceUnavailable, "AI summarization is not configured")
		return
	}

	sample, err := h.sampleEmailsForAI(ctx, 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error loading email samples: %s", err))
		return
	}
	if sample == "" {
		writeError(w, http.StatusUnprocessableEntity, "no emails available for analysis")
		return
	}

	prompt := fmt.Sprintf(`Based on the emails below, provide a psychological profile of the sender. Include:
- Personality traits (Big Five: openness, conscientiousness, extraversion, agreeableness, neuroticism)
- Values and priorities
- Emotional patterns
- Social style and relationships
- Decision-making approach
- Potential strengths and challenges

Email samples:
%s`, sample)

	result, err := h.gemini.GenerateResponse(ctx, appai.GenerateRequest{UserInput: prompt}, "", nil, nil, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("AI error: %s", err))
		return
	}

	if err := h.subjectConfigRepo.UpdatePsychologicalProfileAI(ctx, result.PlainText); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error saving profile: %s", err))
		return
	}

	writeJSON(w, map[string]any{
		"profile": result.PlainText,
		"message": "Psychological profile updated",
	})
}

// sampleEmailsForAI returns a formatted block of recent email subjects + plain text
// suitable for AI analysis. Each email body is capped at 500 characters.
func (h *AdminHandler) sampleEmailsForAI(ctx context.Context, limit int) (string, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT subject, plain_text
		FROM emails
		WHERE plain_text IS NOT NULL AND user_deleted = FALSE
		ORDER BY date DESC
		LIMIT $1`, limit)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var sb strings.Builder
	i := 0
	for rows.Next() {
		var subject, text *string
		if err := rows.Scan(&subject, &text); err != nil {
			continue
		}
		i++
		sb.WriteString(fmt.Sprintf("--- Email %d ---\n", i))
		if subject != nil {
			sb.WriteString(fmt.Sprintf("Subject: %s\n", *subject))
		}
		if text != nil {
			body := *text
			if len(body) > 500 {
				body = body[:500] + "..."
			}
			sb.WriteString(body)
		}
		sb.WriteString("\n\n")
	}
	if i == 0 {
		return "", rows.Err()
	}
	return sb.String(), rows.Err()
}
