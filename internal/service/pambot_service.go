package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	appai "github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/repository"
)

// PamBotService implements the dementia companion LLM interaction.
// It uses its own GeneratePamBotMessage function — not the shared GenerateResponse.
type PamBotService struct {
	repo                *repository.PamBotRepo
	subjectRepo         *repository.SubjectConfigRepo
	appInstrRepo        *repository.AppSystemInstructionsRepo
	pool                *sql.DB
	userRepo            *repository.UserRepo
	defaultGeminiKey    string
	defaultGeminiModel  string
	defaultAnthropicKey string
	defaultClaudeModel  string
	defaultTavilyKey    string
	pepper              string
	sessionStore        *keystore.SessionMasterStore
	billing             *repository.BillingRepo
}

// NewPamBotService creates a PamBotService.
func NewPamBotService(
	repo *repository.PamBotRepo,
	subjectRepo *repository.SubjectConfigRepo,
	appInstrRepo *repository.AppSystemInstructionsRepo,
	pool *sql.DB,
	userRepo *repository.UserRepo,
	defaultGeminiKey, defaultGeminiModel string,
	defaultAnthropicKey, defaultClaudeModel string,
	defaultTavilyKey string,
	pepper string,
	sessionStore *keystore.SessionMasterStore,
	billing *repository.BillingRepo,
) *PamBotService {
	return &PamBotService{
		repo:                repo,
		subjectRepo:         subjectRepo,
		appInstrRepo:        appInstrRepo,
		pool:                pool,
		userRepo:            userRepo,
		defaultGeminiKey:    defaultGeminiKey,
		defaultGeminiModel:  defaultGeminiModel,
		defaultAnthropicKey: defaultAnthropicKey,
		defaultClaudeModel:  defaultClaudeModel,
		defaultTavilyKey:    defaultTavilyKey,
		pepper:              pepper,
		sessionStore:        sessionStore,
		billing:             billing,
	}
}

// PamBotMessageResult is returned from GeneratePamBotMessage.
type PamBotMessageResult struct {
	Message    string `json:"message"`
	SubjectTag string `json:"subject_tag"`
	TurnNumber int    `json:"turn_number"`
	PhotoURL   string `json:"photo_url,omitempty"`
}

// PamBotSessionResult is returned from GetSessionInfo.
type PamBotSessionResult struct {
	SessionID        int     `json:"session_id"`
	InteractionCount int     `json:"interaction_count"`
	LatestAnalysis   *string `json:"latest_analysis,omitempty"`
}

var pamBotJSONRe = regexp.MustCompile(`(?s)<json>\s*(\{.*?\})\s*</json>`)

// GeneratePamBotMessage is the unique LLM function for the dementia companion.
// It selects a topic from the archive, speaks warmly and simply, tracks subjects
// to avoid repetition, and triggers a background summary every 5th interaction.
func (s *PamBotService) GeneratePamBotMessage(ctx context.Context, r *http.Request, action, typedText string) (*PamBotMessageResult, error) {
	// 1. Get or create session
	session, err := s.repo.GetOrCreateSession(ctx)
	if err != nil {
		return nil, fmt.Errorf("pambot session: %w", err)
	}

	// 2. Load recent subjects to avoid repetition
	subjects, _ := s.repo.GetRecentSubjects(ctx, 20)

	// 3. Load subject config (name, gender)
	subjectCfg, _ := s.subjectRepo.GetFirst(ctx)
	subjectName := "the archive owner"
	subjectGender := "Male"
	if subjectCfg != nil {
		if strings.TrimSpace(subjectCfg.SubjectName) != "" {
			subjectName = subjectCfg.SubjectName
		}
		if strings.TrimSpace(subjectCfg.Gender) != "" {
			subjectGender = subjectCfg.Gender
		}
	}

	// 4. Load Pam Bot persona instructions from DB (admin-editable)
	storedInstructions := ""
	if s.appInstrRepo != nil {
		if ins, err := s.appInstrRepo.Get(ctx); err == nil && ins != nil {
			storedInstructions = ins.PamBotInstructions
		}
	}

	// 5. Build system prompt
	turnNumber := session.InteractionCount + 1
	systemPrompt := s.buildPamBotSystemPrompt(storedInstructions, subjectName, subjectGender, subjects, session.LatestAnalysis, turnNumber, session.LastFacebookPostID, session.LastFacebookAlbumID)

	// 6. Convert action to natural-language user input
	userInput := pamBotActionToInput(action, typedText, turnNumber)

	// 7. Load recent turns as history (last 6)
	history, _ := s.repo.GetRecentTurns(ctx, session.ID, 6)

	// 7. Select provider — prefer Claude, fall back to Gemini
	provider := s.selectProvider(r)
	providerName := "claude"
	if provider == nil || !provider.IsAvailable() {
		provider = appai.NewGeminiProvider(s.defaultGeminiKey, s.defaultGeminiModel)
		providerName = "gemini"
	}
	if provider == nil || !provider.IsAvailable() {
		stub := StubLLMUsage(providerName, "")
		RecordLLMUsage(ctx, s.billing, s.userRepo, stub, fmt.Errorf("no AI provider available"))
		return nil, fmt.Errorf("no AI provider available — check API keys")
	}

	// 8. Build tool executor and Pam Bot-specific tool declarations
	getRAM := func() (string, bool) {
		if s.sessionStore == nil || r == nil {
			return "", false
		}
		return s.sessionStore.Get(r)
	}
	executor := appai.NewToolExecutor(s.pool, subjectName, s.defaultTavilyKey, s.pepper, getRAM)
	toolDecls := pamBotToolDeclarations()

	// 9. Call provider
	genReq := appai.GenerateRequest{
		UserInput:     userInput,
		Temperature:   0.85,
		SubjectName:   subjectName,
		SubjectGender: subjectGender,
	}
	result, err := provider.GenerateResponse(ctx, genReq, systemPrompt, history, executor, &toolDecls)
	if err != nil {
		stub := StubLLMUsage(providerName, "")
		RecordLLMUsage(ctx, s.billing, s.userRepo, stub, err)
		return nil, fmt.Errorf("pambot LLM call: %w", err)
	}

	// 10. Extract subject_tag, subject_category, photo_id, facebook ids from embedded <json> block
	subjectTag, subjectCategory, cleanMessage, photoID, facebookPostID, facebookAlbumID := extractPamBotJSON(result.PlainText)

	if fbErr := s.repo.UpdateSessionFacebookContext(ctx, session.ID, facebookPostID, facebookAlbumID); fbErr != nil {
		slog.Warn("pambot update facebook context failed", "err", fbErr)
	}

	// 11. Save turn
	if saveErr := s.repo.SaveTurn(ctx, session.ID, turnNumber, subjectTag, subjectCategory, cleanMessage, action); saveErr != nil {
		slog.Warn("pambot save turn failed", "err", saveErr)
	}

	// 12. Update subject tracking
	if subjectTag != "" {
		if uErr := s.repo.UpsertSubject(ctx, subjectTag, subjectCategory); uErr != nil {
			slog.Warn("pambot upsert subject failed", "err", uErr)
		}
	}

	// 13. Increment session interaction count
	if incErr := s.repo.IncrementInteractionCount(ctx, session.ID); incErr != nil {
		slog.Warn("pambot increment count failed", "err", incErr)
	}

	// 14. Every 5th interaction, generate summary in background
	if turnNumber%5 == 0 {
		uid := appctx.UserIDFromCtx(ctx)
		bgCtx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
		sessionID := session.ID
		go s.GeneratePamBotSummary(bgCtx, sessionID)
	}

	// 15. Record billing
	if result.Usage != nil {
		fromServer := s.defaultAnthropicKey != "" && providerName == "claude"
		if providerName == "gemini" {
			fromServer = s.defaultGeminiKey != ""
		}
		MarkUsageServerKey(result.Usage, fromServer)
	}
	RecordLLMUsage(ctx, s.billing, s.userRepo, result.Usage, nil)

	photoURL := ""
	if photoID > 0 {
		photoURL = fmt.Sprintf("/images/%d", photoID)
	}

	return &PamBotMessageResult{
		Message:    cleanMessage,
		SubjectTag: subjectTag,
		TurnNumber: turnNumber,
		PhotoURL:   photoURL,
	}, nil
}

// GeneratePamBotSummary runs in a background goroutine every 5th interaction.
// It generates a conversation summary and cognitive acuity analysis.
func (s *PamBotService) GeneratePamBotSummary(ctx context.Context, sessionID int) {
	turns, err := s.repo.GetRecentTurnsRaw(ctx, sessionID, 10)
	if err != nil || len(turns) == 0 {
		return
	}

	// Build summary prompt
	var sb strings.Builder
	sb.WriteString("You are analysing a conversation between a dementia companion AI and a person with memory difficulties.\n\n")
	sb.WriteString("CONVERSATION TURNS (chronological):\n")
	for _, t := range turns {
		sb.WriteString(fmt.Sprintf("Turn %d [topic: %s]: %s\n\n", t.TurnNumber, t.SubjectTag, t.BotMessage))
		if t.UserAction != "" {
			sb.WriteString(fmt.Sprintf("  User action: %s\n", t.UserAction))
		}
	}
	sb.WriteString(`
Please respond with a JSON object with exactly these two keys:
{
  "summary": "A brief summary (2-3 sentences) of the conversation topics covered",
  "analysis": "An assessment of the user's apparent engagement and any topics of high interest, plus recommendations for tailoring future interactions"
}

Respond with only the JSON object, no other text.`)

	type simpleGenerator interface {
		IsAvailable() bool
		SimpleGenerate(ctx context.Context, prompt string) (string, *appai.LLMUsage, error)
	}

	var sg simpleGenerator
	claude := appai.NewClaudeProvider(s.defaultAnthropicKey, s.defaultClaudeModel)
	if claude != nil && claude.IsAvailable() {
		sg = claude
	} else {
		gemini := appai.NewGeminiProvider(s.defaultGeminiKey, s.defaultGeminiModel)
		if gemini != nil && gemini.IsAvailable() {
			sg = gemini
		}
	}
	if sg == nil {
		return
	}

	text, usage, err := sg.SimpleGenerate(ctx, sb.String())
	if err != nil {
		slog.Warn("pambot summary generation failed", "err", err)
		RecordLLMUsage(ctx, s.billing, s.userRepo, StubLLMUsage("claude", ""), err)
		return
	}

	// Parse the JSON response
	text = strings.TrimSpace(text)
	var parsed struct {
		Summary  string `json:"summary"`
		Analysis string `json:"analysis"`
	}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		// Try to extract JSON if wrapped in code fences
		if m := regexp.MustCompile(`(?s)\{.*\}`).FindString(text); m != "" {
			_ = json.Unmarshal([]byte(m), &parsed)
		}
	}

	summary := parsed.Summary
	analysis := parsed.Analysis
	if summary == "" && analysis == "" {
		// Fallback: store raw text as analysis
		analysis = text
	}

	if saveErr := s.repo.UpdateSessionSummary(ctx, sessionID, summary, analysis); saveErr != nil {
		slog.Warn("pambot save summary failed", "err", saveErr)
	}

	RecordLLMUsage(ctx, s.billing, s.userRepo, usage, nil)
}

// GetSessionInfo returns summary info for the current user's latest session.
func (s *PamBotService) GetSessionInfo(ctx context.Context) (*PamBotSessionResult, error) {
	session, err := s.repo.GetOrCreateSession(ctx)
	if err != nil {
		return nil, err
	}
	return &PamBotSessionResult{
		SessionID:        session.ID,
		InteractionCount: session.InteractionCount,
		LatestAnalysis:   session.LatestAnalysis,
	}, nil
}

// selectProvider picks Claude first, falls back to Gemini.
func (s *PamBotService) selectProvider(_ *http.Request) appai.ChatProvider {
	p := appai.NewClaudeProvider(s.defaultAnthropicKey, s.defaultClaudeModel)
	if p != nil && p.IsAvailable() {
		return p
	}
	return appai.NewGeminiProvider(s.defaultGeminiKey, s.defaultGeminiModel)
}

// buildPamBotSystemPrompt constructs the specialised companion system prompt.
func (s *PamBotService) buildPamBotSystemPrompt(
	storedInstructions string,
	subjectName, subjectGender string,
	subjects []repository.PamBotSubject,
	lastAnalysis *string,
	turnNumber int,
	lastFacebookPostID, lastFacebookAlbumID *int64,
) string {
	var sb strings.Builder

	pronoun := "them"
	if subjectGender == "Female" {
		pronoun = "her"
	} else if subjectGender == "Male" {
		pronoun = "him"
	}
	_ = pronoun // used implicitly through subject name below

	// Use DB-stored persona instructions as the base, with a sensible fallback.
	base := strings.TrimSpace(storedInstructions)
	if base == "" {
		base = `You are a warm, patient, and gentle memory companion for someone who may be experiencing memory difficulties.
Use the archive tools to find real, personal memories. Speak simply and warmly in 2-4 short sentences.
Never correct or challenge — always validate with kindness. Focus on positive, familiar memories.`
	}
	sb.WriteString(fmt.Sprintf("You are speaking with %s.\n\n", subjectName))
	sb.WriteString(base)
	sb.WriteString("\n\n")

	sb.WriteString(`IMPORTANT FORMAT RULE:
At the very end of your response, include a metadata block in this exact format:
<json>{"subject_tag": "brief_unique_snake_case_tag", "subject_category": "family|friends|travel|work|hobbies|general", "photo_id": null, "facebook_post_id": null, "facebook_album_id": null}</json>
This block will be hidden from the user. Choose a tag that uniquely identifies the memory or topic you discussed.
When you have retrieved image IDs using get_album_images and one is relevant to the memory, set photo_id to that integer. Otherwise leave it null.
When you have retrieved post IDs using search_facebook_posts and one is relevant to the memory, set facebook_post_id to that integer. Otherwise leave it null.
When you have retrieved album IDs using search_facebook_albums and one is relevant to the memory, set facebook_album_id to that integer. Otherwise leave it null.

`)

	if lastFacebookPostID != nil || lastFacebookAlbumID != nil {
		sb.WriteString("CONTEXT FROM YOUR LAST RESPONSE (Facebook):\n")
		if lastFacebookPostID != nil {
			sb.WriteString(fmt.Sprintf("- facebook_post_id: %d\n", *lastFacebookPostID))
		}
		if lastFacebookAlbumID != nil {
			sb.WriteString(fmt.Sprintf("- facebook_album_id: %d\n", *lastFacebookAlbumID))
		}
		sb.WriteString("Use these when continuing the same story or album. Set both to null in <json> when you intentionally change topic.\n\n")
	}

	// Add subject avoidance list
	if len(subjects) > 0 {
		sb.WriteString("SUBJECTS RECENTLY DISCUSSED (please explore different topics — avoid these for now):\n")
		for i, sub := range subjects {
			if i >= 15 {
				break
			}
			sb.WriteString(fmt.Sprintf("- %s (%s)\n", sub.SubjectTag, sub.SubjectCategory))
		}
		sb.WriteString("\n")
	}

	// Add cognitive analysis context
	sb.WriteString("CONTEXT FROM PREVIOUS INTERACTIONS:\n")
	if lastAnalysis != nil && strings.TrimSpace(*lastAnalysis) != "" {
		sb.WriteString(*lastAnalysis)
	} else {
		sb.WriteString("This is an early session — start gently and warmly with something very familiar from the archive.")
	}
	sb.WriteString("\n\n")

	sb.WriteString(fmt.Sprintf("This is interaction number %d in this session.\n", turnNumber))

	return sb.String()
}

// pamBotActionToInput converts the user's button press into a natural-language prompt.
func pamBotActionToInput(action, typedText string, turnNumber int) string {
	switch action {
	case "start":
		return "Please begin our conversation with a warm, personal memory from my archive. Choose something joyful and familiar."
	case "more":
		return "Please tell me more about that — more details or another related memory from my archive."
	case "different":
		return "Thank you. Could you share something different — a different memory or topic from my archive?"
	case "typed":
		if strings.TrimSpace(typedText) != "" {
			return typedText
		}
		return "Please share another memory from my archive."
	default:
		if turnNumber == 1 {
			return "Please begin our conversation with a personal memory from my archive."
		}
		return "Please share another memory from my archive."
	}
}

// extractPamBotJSON parses the <json>...</json> block from the LLM response,
// returning subject_tag, subject_category, photo_id, facebook ids, and the cleaned message text.
func extractPamBotJSON(text string) (subjectTag, subjectCategory, cleanText string, photoID, facebookPostID, facebookAlbumID int64) {
	m := pamBotJSONRe.FindStringSubmatchIndex(text)
	if m == nil {
		return "", "", strings.TrimSpace(text), 0, 0, 0
	}

	jsonStr := text[m[2]:m[3]]
	cleanText = strings.TrimSpace(text[:m[0]] + text[m[1]:])

	var meta struct {
		SubjectTag      string `json:"subject_tag"`
		SubjectCategory string `json:"subject_category"`
		PhotoID         *int64 `json:"photo_id"`
		FacebookPostID  *int64 `json:"facebook_post_id"`
		FacebookAlbumID *int64 `json:"facebook_album_id"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &meta); err == nil {
		subjectTag = meta.SubjectTag
		subjectCategory = meta.SubjectCategory
		if meta.PhotoID != nil {
			photoID = *meta.PhotoID
		}
		if meta.FacebookPostID != nil {
			facebookPostID = *meta.FacebookPostID
		}
		if meta.FacebookAlbumID != nil {
			facebookAlbumID = *meta.FacebookAlbumID
		}
	}
	return
}

// pamBotToolDeclarations returns tools for the companion (PamBot-specific schemas in internal/ai).
func pamBotToolDeclarations() []map[string]any {
	return appai.PamBotToolDefinitions()
}
