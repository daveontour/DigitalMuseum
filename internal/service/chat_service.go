package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strings"

	appai "github.com/daveontour/digitalmuseum/internal/ai"
	"github.com/daveontour/digitalmuseum/internal/keystore"
	"github.com/daveontour/digitalmuseum/internal/model"
	"github.com/daveontour/digitalmuseum/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

// voiceEntry holds one entry from voice_instructions.json.
type voiceEntry struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
}

// ChatService orchestrates AI generation, tool calling, and conversation persistence.
type ChatService struct {
	chatRepo        *repository.ChatRepo
	subjectRepo     *repository.SubjectConfigRepo
	cpRepo          *repository.CompleteProfileRepo
	pool            *pgxpool.Pool
	geminiProvider  appai.ChatProvider
	claudeProvider  appai.ChatProvider
	pythonStaticDir string
	tavilyKey       string
	pepper          string
	sessionStore    *keystore.SessionMasterStore
	privateStore    *PrivateStoreService
}

// NewChatService creates a ChatService.
func NewChatService(
	chatRepo *repository.ChatRepo,
	subjectRepo *repository.SubjectConfigRepo,
	cpRepo *repository.CompleteProfileRepo,
	pool *pgxpool.Pool,
	geminiProvider appai.ChatProvider,
	claudeProvider appai.ChatProvider,
	pythonStaticDir string,
	tavilyKey string,
	pepper string,
	sessionStore *keystore.SessionMasterStore,
	privateStore *PrivateStoreService,
) *ChatService {
	return &ChatService{
		chatRepo:        chatRepo,
		subjectRepo:     subjectRepo,
		cpRepo:          cpRepo,
		pool:            pool,
		geminiProvider:  geminiProvider,
		claudeProvider:  claudeProvider,
		pythonStaticDir: pythonStaticDir,
		tavilyKey:       tavilyKey,
		pepper:          pepper,
		sessionStore:    sessionStore,
		privateStore:    privateStore,
	}
}

func (s *ChatService) perRequestGetRAM(r *http.Request) appai.RAMMasterGetter {
	return func() (string, bool) {
		if s.sessionStore == nil || r == nil {
			return "", false
		}
		return s.sessionStore.Get(r)
	}
}

func (s *ChatService) loadToolAccessPolicy(ctx context.Context, masterPassword string) appai.ToolAccessPolicy {
	if s.privateStore == nil || strings.TrimSpace(masterPassword) == "" {
		return nil
	}
	rec, err := s.privateStore.GetByKey(ctx, appai.LLMToolsAccessStoreKey, masterPassword)
	if err != nil || rec == nil || strings.TrimSpace(rec.Value) == "" {
		return nil
	}
	p, err := appai.ParseToolAccessPolicyJSON(rec.Value)
	if err != nil {
		return nil
	}
	return p
}

// buildChatTools returns a policy-wrapped executor and filtered tool schemas for the current session tier.
func (s *ChatService) buildChatTools(ctx context.Context, r *http.Request, subjectName string) (appai.ToolExecutor, *[]map[string]any) {
	getRAM := s.perRequestGetRAM(r)
	tier := appai.UnlockTierFromSession(s.sessionStore, r)
	pw, ok := getRAM()
	var policy appai.ToolAccessPolicy
	if ok && pw != "" {
		policy = s.loadToolAccessPolicy(ctx, pw)
	}
	filtered := appai.FilterToolDefinitionsForTier(policy, tier)
	base := appai.NewToolExecutor(s.pool, subjectName, s.tavilyKey, s.pepper, getRAM)
	wrapped := appai.WrapToolExecutorWithPolicy(base, policy, tier)
	return wrapped, &filtered
}

// GeminiAvailable reports whether the Gemini provider is configured.
func (s *ChatService) GeminiAvailable() bool {
	return s.geminiProvider != nil && s.geminiProvider.IsAvailable()
}

// ClaudeAvailable reports whether the Claude provider is configured.
func (s *ChatService) ClaudeAvailable() bool {
	return s.claudeProvider != nil && s.claudeProvider.IsAvailable()
}

// GenerateResponse runs a full chat generation cycle.
func (s *ChatService) GenerateResponse(ctx context.Context, r *http.Request, req model.ChatRequest) (*model.ChatResponse, error) {
	// Choose provider
	provider := s.geminiProvider
	providerName := "gemini"
	if req.Provider == "claude" && s.claudeProvider != nil && s.claudeProvider.IsAvailable() {
		provider = s.claudeProvider
		providerName = "claude"
	}
	if provider == nil || !provider.IsAvailable() {
		return nil, fmt.Errorf("provider '%s' is not available — check API key", providerName)
	}

	voice := "expert"
	if req.Voice != nil && *req.Voice != "" {
		voice = *req.Voice
	}
	temperature := 0.0
	if req.Temperature != nil {
		temperature = *req.Temperature
	}
	mood := "neutral"
	if req.Mood != nil && *req.Mood != "" {
		mood = *req.Mood
	}
	whosAsking := req.WhosAsking
	if whosAsking == "" {
		whosAsking = "visitor"
	}

	repeatQuestion := req.RepeatQuestion

	// Load subject configuration
	cfg, _ := s.subjectRepo.GetFirst(ctx)
	subjectName := "Unknown"
	subjectGender := "Male"
	var psychProfile, writingStyle *string
	var sysInstructions, coreInstructions string
	if cfg != nil {
		subjectName = cfg.SubjectName
		subjectGender = cfg.Gender
		psychProfile = cfg.PsychologicalProfileAI
		writingStyle = cfg.WritingStyleAI
		sysInstructions = cfg.SystemInstructions
		coreInstructions = cfg.CoreSystemInstructions
	}

	// Pronoun substitution
	he, him, his := genderPronouns(subjectGender)
	replacer := strings.NewReplacer(
		"{SUBJECT_NAME}", subjectName,
		"{he}", he, "{him}", him, "{his}", his,
	)
	sysInstructions = replacer.Replace(sysInstructions)
	coreInstructions = replacer.Replace(coreInstructions)

	// Load voice instructions
	voiceMap := s.loadVoiceInstructions(ctx)
	entry, ok := voiceMap[voice]
	if !ok {
		entry = voiceMap["expert"]
		voice = "expert"
	}
	voiceText := replacer.Replace(entry.Instructions)

	// Build system prompt
	whosAskingText := fmt.Sprintf("The person asking is a visitor (not the subject %s). They are asking questions about the subject's life and history.", subjectName)
	if whosAsking == "its-me" {
		whosAskingText = fmt.Sprintf("The person asking is %s themselves. They are asking questions about their own history and life.", subjectName)
	}
	systemPrompt := coreInstructions +
		"\n\n**Your Personae:**\n" + voiceText +
		"\n\n**Additional Information:**\n" + sysInstructions +
		"\n\n**Who is asking:** " + whosAskingText

	if repeatQuestion {
		systemPrompt += "\n\n**IMPORTANT Repeat Question:** Repeat the question in the same language and tone as the original question at the begining of the response"
	}
	// Load conversation history
	var history []appai.ConvTurn
	if req.ConversationID != nil {
		turns, err := s.chatRepo.GetTurns(ctx, *req.ConversationID, 30)
		if err == nil {
			for _, t := range turns {
				history = append(history, appai.ConvTurn{
					UserInput:    t.UserInput,
					ResponseText: t.ResponseText,
				})
			}
		}
	}

	// Build tool executor and generation request
	executor, toolDecls := s.buildChatTools(ctx, r, subjectName)
	genReq := appai.GenerateRequest{
		UserInput:     req.Prompt,
		Temperature:   temperature,
		Voice:         voice,
		Mood:          mood,
		CompanionMode: req.CompanionMode,
		WhosAsking:    whosAsking,
		SubjectName:   subjectName,
		SubjectGender: subjectGender,
	}
	if voice == "owner" {
		genReq.PsychProfile = psychProfile
		genReq.WritingStyle = writingStyle
	}

	result, err := provider.GenerateResponse(ctx, genReq, systemPrompt, history, executor, toolDecls)
	if err != nil {
		return nil, err
	}

	// Save turn if conversation ID provided
	if req.ConversationID != nil {
		_ = s.chatRepo.SaveTurn(ctx, *req.ConversationID, req.Prompt, result.PlainText, voice, temperature)
	}

	// Enrich metadata and return
	var embeddedJSON map[string]any
	if err := json.Unmarshal([]byte(result.MetadataJSON), &embeddedJSON); err == nil {
		embeddedJSON["temperature"] = temperature
		embeddedJSON["prompt"] = req.Prompt
		embeddedJSON["voice"] = voice
		embeddedJSON["response_text"] = result.PlainText
		// Flatten: if embedded_json contains an array of parsed blocks, merge the first into top level and remove the nested key
		if arr, ok := embeddedJSON["embedded_json"].([]any); ok && len(arr) > 0 {
			if first, ok := arr[0].(map[string]any); ok {
				for k, v := range first {
					embeddedJSON[k] = v
				}
			}
			delete(embeddedJSON, "embedded_json")
		}
	}
	return &model.ChatResponse{
		Response:     result.PlainText,
		Voice:        voice,
		EmbeddedJSON: embeddedJSON,
	}, nil
}

// Generate A Random Question
func (s *ChatService) GenerateRandomQuestion(ctx context.Context, r *http.Request, req model.ChatRequest) (*model.ChatResponse, error) {
	// Choose provider
	provider := s.geminiProvider
	providerName := "gemini"
	if req.Provider == "claude" && s.claudeProvider != nil && s.claudeProvider.IsAvailable() {
		provider = s.claudeProvider
		providerName = "claude"
	}
	if provider == nil || !provider.IsAvailable() {
		return nil, fmt.Errorf("provider '%s' is not available — check API key", providerName)
	}

	voice := "expert"
	if req.Voice != nil && *req.Voice != "" {
		voice = *req.Voice
	}
	temperature := 0.5
	mood := "neutral"
	if req.Mood != nil && *req.Mood != "" {
		mood = *req.Mood
	}

	// Load subject configuration
	cfg, _ := s.subjectRepo.GetFirst(ctx)
	subjectName := "Unknown"
	subjectGender := "Male"
	// var sysInstructions, coreInstructions string
	if cfg != nil {
		subjectName = cfg.SubjectName
		subjectGender = cfg.Gender
		// sysInstructions = cfg.SystemInstructions
		// coreInstructions = cfg.CoreSystemInstructions
	}

	// Pronoun substitution
	he, him, his := genderPronouns(subjectGender)
	replacer := strings.NewReplacer(
		"{SUBJECT_NAME}", subjectName,
		"{he}", he, "{him}", him, "{his}", his,
	)
	// sysInstructions = replacer.Replace(sysInstructions)
	// coreInstructions = replacer.Replace(coreInstructions)

	// Load voice instructions
	voiceMap := s.loadVoiceInstructions(ctx)
	entry, ok := voiceMap[voice]
	if !ok {
		entry = voiceMap["expert"]
		voice = "expert"
	}
	voiceText := replacer.Replace(entry.Instructions)

	whosAsking := req.WhosAsking
	if whosAsking == "" {
		whosAsking = "visitor"
	}
	whosAskingText := fmt.Sprintf("The person asking is a visitor (not the subject %s). They are asking questions about the subject's life and history.", subjectName)
	if whosAsking == "its-me" {
		whosAskingText = fmt.Sprintf("The person asking is %s themselves. They are asking questions about their own history and life.", subjectName)
	}

	// Load system instructions
	coreInstructions, err := os.ReadFile(fmt.Sprintf("%s/data/system_instructions_question.txt", s.pythonStaticDir))
	if err != nil {
		return nil, fmt.Errorf("load system instructions: %w", err)
	}

	// Build system prompt
	systemPrompt := string(coreInstructions) +
		"\n\n**Your Personae:**\n" + voiceText +
		"\n\n**Who is asking:** " + whosAskingText

	// Load conversation history
	var history []appai.ConvTurn
	//Dont' want history when generating a random question

	// if req.ConversationID != nil {
	// 	turns, err := s.chatRepo.GetTurns(ctx, *req.ConversationID, 30)
	// 	if err == nil {
	// 		for _, t := range turns {
	// 			history = append(history, appai.ConvTurn{
	// 				UserInput:    t.UserInput,
	// 				ResponseText: t.ResponseText,
	// 			})
	// 		}
	// 	}
	// }

	//Select a random topic from the following list:
	topics := []string{
		"biography",
		"people " + he + "'s known",
		"travels",
		"work",
		"hobbies",
		"relationships",
		"psychology",
		"interest",
		"family",
		"friends",
		"childhood",
		"sports",
		"creative and artistic endeavours",
		"philosophy",
	}
	randomTopic := topics[rand.Intn(len(topics))]

	prompt := "Generate a random question about " + subjectName + "'s life." +
		" It could be about any aspect of " + randomTopic + "." +
		" The objective is that by answering the question it would provide insight into " + him + " or " +
		" reveal hidden or understated aspects of " + him + " or amusing facts." +
		" Do not answer the question, just generate it."

	// Build tool executor and generation request
	executor, toolDecls := s.buildChatTools(ctx, r, subjectName)
	genReq := appai.GenerateRequest{
		UserInput:     prompt,
		Temperature:   temperature,
		Voice:         voice,
		Mood:          mood,
		CompanionMode: false,
		WhosAsking:    whosAsking,
		SubjectName:   subjectName,
		SubjectGender: subjectGender,
	}

	// if voice == "owner" {
	// 	genReq.PsychProfile = psychProfile
	// 	genReq.WritingStyle = writingStyle
	// }

	result, err := provider.GenerateResponse(ctx, genReq, systemPrompt, history, executor, toolDecls)
	if err != nil {
		return nil, err
	}

	// Enrich metadata and return
	var embeddedJSON map[string]any
	if err := json.Unmarshal([]byte(result.MetadataJSON), &embeddedJSON); err == nil {
		embeddedJSON["temperature"] = temperature
		embeddedJSON["prompt"] = prompt
		embeddedJSON["voice"] = voice
		embeddedJSON["response_text"] = result.PlainText
		// Flatten: if embedded_json contains an array of parsed blocks, merge the first into top level and remove the nested key
		if arr, ok := embeddedJSON["embedded_json"].([]any); ok && len(arr) > 0 {
			if first, ok := arr[0].(map[string]any); ok {
				for k, v := range first {
					embeddedJSON[k] = v
				}
			}
			delete(embeddedJSON, "embedded_json")
		}
	}
	return &model.ChatResponse{
		Response:     result.PlainText,
		Voice:        voice,
		EmbeddedJSON: embeddedJSON,
	}, nil
}

// loadVoiceInstructions reads voice_instructions.json and merges DB custom voices.
func (s *ChatService) loadVoiceInstructions(ctx context.Context) map[string]voiceEntry {
	result := map[string]voiceEntry{
		"expert": {Name: "Expert", Instructions: "You are a professional expert."},
	}

	path := fmt.Sprintf("%s/data/voice_instructions.json", s.pythonStaticDir)
	data, err := os.ReadFile(path)
	if err == nil {
		var raw map[string]any
		if json.Unmarshal(data, &raw) == nil {
			for key, val := range raw {
				if vm, ok := val.(map[string]any); ok {
					entry := voiceEntry{
						Name:         anyStr(vm["name"]),
						Description:  anyStr(vm["description"]),
						Instructions: anyStr(vm["instructions"]),
					}
					result[key] = entry
				}
			}
		}
	}

	// Merge custom voices from DB (built-in keys are never overwritten)
	rows, err := s.pool.Query(ctx, `SELECT key, name, description, instructions FROM custom_voices`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var key, name, instructions string
			var desc *string
			if err := rows.Scan(&key, &name, &desc, &instructions); err == nil {
				if _, exists := result[key]; !exists {
					entry := voiceEntry{Name: name, Instructions: instructions}
					if desc != nil {
						entry.Description = *desc
					}
					result[key] = entry
				}
			}
		}
	}
	return result
}

// ── Conversation CRUD ─────────────────────────────────────────────────────────

func (s *ChatService) CreateConversation(ctx context.Context, title, voice string) (*model.ChatConversation, error) {
	return s.chatRepo.CreateConversation(ctx, title, voice)
}

func (s *ChatService) GetConversation(ctx context.Context, id int64) (*model.ChatConversation, error) {
	return s.chatRepo.GetConversation(ctx, id)
}

func (s *ChatService) ListConversations(ctx context.Context, limit *int) ([]*model.ChatConversation, error) {
	return s.chatRepo.ListConversations(ctx, limit)
}

func (s *ChatService) UpdateConversation(ctx context.Context, id int64, title, voice *string) (*model.ChatConversation, error) {
	return s.chatRepo.UpdateConversation(ctx, id, title, voice)
}

func (s *ChatService) DeleteConversation(ctx context.Context, id int64) error {
	return s.chatRepo.DeleteConversation(ctx, id)
}

func (s *ChatService) GetTurns(ctx context.Context, conversationID int64, limit int) ([]*model.ChatTurn, error) {
	return s.chatRepo.GetTurns(ctx, conversationID, limit)
}

func (s *ChatService) TurnCount(ctx context.Context, conversationID int64) (int64, error) {
	return s.chatRepo.TurnCount(ctx, conversationID)
}

// GenerateCompleteProfile builds a multi-step relationship profile for a contact
// from messages and emails, using the specified AI provider (gemini or claude) to summarize,
// and saves it to complete_profiles. Mirrors the Python base_chat_service.get_complete_profile_by_name.
func (s *ChatService) GenerateCompleteProfile(ctx context.Context, name string, provider string, getRAM appai.RAMMasterGetter, tier appai.UnlockTier) error {
	if getRAM == nil {
		getRAM = func() (string, bool) { return "", false }
	}
	pw, _ := getRAM()
	policy := s.loadToolAccessPolicy(ctx, pw)
	base := appai.NewToolExecutor(s.pool, "", s.tavilyKey, s.pepper, getRAM)
	executor := appai.WrapToolExecutorWithPolicy(base, policy, tier)
	msgsRaw, err := executor(ctx, "get_imessages_by_chat_session", map[string]any{"chat_session": name})
	if err != nil {
		return fmt.Errorf("get messages: %w", err)
	}
	emailsRaw, err := executor(ctx, "get_emails_by_contact", map[string]any{"name": name})
	if err != nil {
		return fmt.Errorf("get emails: %w", err)
	}

	// Tools return []map[string]any, not []any; convert so we can append email entries
	var msgs []any
	switch v := msgsRaw["messages"].(type) {
	case []map[string]any:
		for _, m := range v {
			msgs = append(msgs, m)
		}
	case []any:
		msgs = v
	}
	if msgs == nil {
		msgs = []any{}
	}
	var emails []any
	switch v := emailsRaw["emails"].(type) {
	case []map[string]any:
		for _, e := range v {
			emails = append(emails, e)
		}
	case []any:
		emails = v
	}
	if emails == nil {
		emails = []any{}
	}

	// Convert emails to message format and append (match Python)
	for _, e := range emails {
		em, ok := e.(map[string]any)
		if !ok {
			continue
		}
		plainText, _ := em["plain_text"].(string)
		from, _ := em["from_address"].(string)
		to, _ := em["to_addresses"].(string)
		subj, _ := em["subject"].(string)
		date := em["date"]
		id := em["id"]
		if plainText != "" && from != "" && to != "" && subj != "" && date != nil && id != nil {
			msgs = append(msgs, map[string]any{
				"id":           id,
				"message_date": date,
				"sender_name":  from,
				"sender_id":    from,
				"type":         "email",
				"text":         plainText,
				"service":      "email",
			})
		}
	}

	// Chunk by ~800KB (Python uses asizeof ~800000)
	const chunkBytes = 800_000
	var chunks [][]any
	var current []any
	var currentSize int
	for _, m := range msgs {
		b, _ := json.Marshal(m)
		sz := len(b) + 50
		if currentSize+sz > chunkBytes && len(current) > 0 {
			chunks = append(chunks, current)
			current = nil
			currentSize = 0
		}
		current = append(current, m)
		currentSize += sz
	}
	if len(current) > 0 {
		chunks = append(chunks, current)
	}

	// Resolve provider: prefer requested, default gemini, fallback claude if gemini unavailable
	if provider == "" {
		provider = "gemini"
	}
	if provider == "claude" && (s.claudeProvider == nil || !s.claudeProvider.IsAvailable()) {
		provider = "gemini" // fallback
	}
	if provider == "gemini" && (s.geminiProvider == nil || !s.geminiProvider.IsAvailable()) {
		provider = "claude" // fallback
	}

	type simpleGen interface {
		SimpleGenerate(context.Context, string) (string, error)
	}
	var ai simpleGen
	switch provider {
	case "claude":
		if cp, ok := s.claudeProvider.(*appai.ClaudeProvider); ok && cp != nil {
			ai = cp
		}
	case "gemini":
		if gp, ok := s.geminiProvider.(*appai.GeminiProvider); ok && gp != nil {
			ai = gp
		}
	}
	if ai == nil {
		return fmt.Errorf("no AI provider available for complete profile (gemini or claude API key required)")
	}

	var interimSummary string
	total := len(chunks)
	for i, chunk := range chunks {
		chunkMap := map[string]any{"messages": chunk}
		data, _ := json.Marshal(chunkMap)
		prompt := fmt.Sprintf(`You are a helpful assistant that summarizes communication patterns, relationships and psychological profiles in multiple steps.
You will be given a list of messages and an interim summary. Summarize based on the interim summary.
Build on the interim summary—do not replace it. Return the next cumulative interim summary.

There will be %d chunks total. This is chunk %d.

Interim summary so far:
%s

Data to process:
%s`, total, i+1, interimSummary, string(data))

		out, err := ai.SimpleGenerate(ctx, prompt)
		if err != nil {
			return fmt.Errorf("summarize chunk %d/%d: %w", i+1, total, err)
		}
		interimSummary = out
	}

	if err := s.cpRepo.Upsert(ctx, name, interimSummary); err != nil {
		return fmt.Errorf("save profile: %w", err)
	}
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func genderPronouns(gender string) (he, him, his string) {
	if gender == "Female" {
		return "she", "her", "her"
	}
	return "he", "him", "his"
}

func anyStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
