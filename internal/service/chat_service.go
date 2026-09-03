package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	appai "github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
)

// chatHistoryTurnLimit is how many prior user/assistant exchanges to send as context to the LLM.
const chatHistoryTurnLimit = 10

// explicitContentInstruction is appended to the chat system prompt when the client sends allowExplicitContent.
const explicitContentInstruction = "\n\n**Content policy (user preference):** The user has opted in to allowing explicit content for this chat. You may include explicit language, sexual themes, or other adult material in your responses when it fits the user's request. Do not refuse or sanitize answers solely for being mature or adult in nature."

func appendExplicitContentPolicy(systemPrompt string, allow bool) string {
	if !allow {
		return systemPrompt
	}
	return systemPrompt + explicitContentInstruction
}

// snarkinessInstruction is appended when the client sends enableSnarkiness.
const snarkinessInstruction = "\n\n**Tone (user preference):** Snarkiness is enabled. You may use dry wit, playful sarcasm, and lightly teasing humour when it fits the persona and moment. Stay clever rather than cruel; do not become insulting or demeaning toward the person asking."

func appendSnarkinessPolicy(systemPrompt string, enable bool) string {
	if !enable {
		return systemPrompt
	}
	return systemPrompt + snarkinessInstruction
}

func readAuthSessionID(r *http.Request, override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	if r == nil {
		return ""
	}
	c, err := r.Cookie(AuthSessionCookieName)
	if err != nil || c == nil {
		return ""
	}
	return c.Value
}

// voiceEntry holds one entry from voice_instructions.json.
type voiceEntry struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
}

// ChatService orchestrates AI generation, tool calling, and conversation persistence.
type ChatService struct {
	chatRepo             *repository.ChatRepo
	subjectRepo          *repository.SubjectConfigRepo
	appInstrRepo         *repository.AppSystemInstructionsRepo
	cpRepo               *repository.CompleteProfileRepo
	docRepo              *repository.DocumentRepo
	pool                 *sql.DB
	userRepo             *repository.UserRepo
	aiModelsSvc          *AIModelsService
	defaultLocalAIURL           string
	defaultLocalAIEmbeddingURL  string
	defaultLocalAIKey           string
	defaultLocalAIModel  string
	defaultLocalAIEmbeddingModel string
	defaultLocalAINumCtx int
	pythonStaticDir      string
	pepper               string
	sessionStore         *keystore.SessionMasterStore
	privateStore         *PrivateStoreService
	billing              *repository.BillingRepo
	dashSvc              *DashboardService
	configRepo           *repository.ConfigRepo
	localAIProbeCache    sync.Map
	archiveInventoryCache sync.Map
}

// NewChatService creates a ChatService. OpenRouter/Tavily/RunPod API keys are configured
// only via Configuration → API Keys in the running app (users/session_visitor_llm tables) —
// there is no server-wide env fallback for them. LocalAI defaults still come from cfg/env
// since they describe the local Ollama daemon, not a hosted credential.
func NewChatService(
	chatRepo *repository.ChatRepo,
	subjectRepo *repository.SubjectConfigRepo,
	appInstrRepo *repository.AppSystemInstructionsRepo,
	cpRepo *repository.CompleteProfileRepo,
	docRepo *repository.DocumentRepo,
	pool *sql.DB,
	userRepo *repository.UserRepo,
	aiModelsSvc *AIModelsService,
	defaultLocalAIURL, defaultLocalAIEmbeddingURL, defaultLocalAIKey, defaultLocalAIModel, defaultLocalAIEmbeddingModel string,
	defaultLocalAINumCtx int,
	pythonStaticDir string,
	pepper string,
	sessionStore *keystore.SessionMasterStore,
	privateStore *PrivateStoreService,
	billing *repository.BillingRepo,
	dashSvc *DashboardService,
	configRepo *repository.ConfigRepo,
) *ChatService {
	return &ChatService{
		chatRepo:             chatRepo,
		subjectRepo:          subjectRepo,
		appInstrRepo:         appInstrRepo,
		cpRepo:               cpRepo,
		docRepo:              docRepo,
		pool:                 pool,
		userRepo:             userRepo,
		aiModelsSvc:          aiModelsSvc,
		defaultLocalAIURL:           defaultLocalAIURL,
		defaultLocalAIEmbeddingURL:  defaultLocalAIEmbeddingURL,
		defaultLocalAIKey:           defaultLocalAIKey,
		defaultLocalAIModel:  defaultLocalAIModel,
		defaultLocalAIEmbeddingModel: defaultLocalAIEmbeddingModel,
		defaultLocalAINumCtx: defaultLocalAINumCtx,
		pythonStaticDir:      pythonStaticDir,
		pepper:               pepper,
		sessionStore:         sessionStore,
		privateStore:         privateStore,
		billing:              billing,
		dashSvc:              dashSvc,
		configRepo:           configRepo,
	}
}

func (s *ChatService) loadAppSystemInstructions(ctx context.Context) (chat, core, question string, err error) {
	if s.appInstrRepo == nil {
		return "", "", "", fmt.Errorf("app system instructions repository not configured")
	}
	ins, err := s.appInstrRepo.Get(ctx)
	if err != nil {
		return "", "", "", err
	}
	if ins == nil {
		return "", "", "", nil
	}
	return ins.ChatInstructions, ins.CoreInstructions, ins.QuestionInstructions, nil
}

// effectiveOpenRouterConfig merges the archive owner's saved key (users row), then visitor
// session override (sessions.visitor_llm_overrides) when the request is a visitor session.
// authSessionID is used when r is nil (e.g. background jobs); otherwise the cookie on r is
// read. Tavily's key shares the same merge precedence and is resolved alongside it since
// it's stored on the same rows. There is no server-wide env fallback for either key — both
// are configurable only via Configuration → API Keys.
func (s *ChatService) effectiveOpenRouterConfig(ctx context.Context, r *http.Request, authSessionID string) (openRouterKey, tavilyKey string) {
	uid := appctx.UserIDFromCtx(ctx)
	useOwnerLLM := true
	if appctx.IsVisitorFromCtx(ctx) && s.userRepo != nil {
		sid := readAuthSessionID(r, authSessionID)
		if sid != "" {
			if pol, err := s.userRepo.GetVisitorSessionLLMPolicy(ctx, sid); err == nil && pol != nil {
				useOwnerLLM = pol.AllowOwnerKeys
			}
		}
	}
	if uid != 0 && useOwnerLLM && s.userRepo != nil {
		if stored, err := s.userRepo.GetUserLLMStored(ctx, uid); err == nil && stored != nil {
			openRouterKey = strings.TrimSpace(stored.OpenRouterAPIKey)
			tavilyKey = strings.TrimSpace(stored.TavilyAPIKey)
		}
	}
	if appctx.IsVisitorFromCtx(ctx) && s.userRepo != nil {
		sid := readAuthSessionID(r, authSessionID)
		if sid == "" {
			return
		}
		vis, err := s.userRepo.GetSessionVisitorLLM(ctx, sid)
		if err != nil || vis == nil {
			return
		}
		if strings.TrimSpace(vis.OpenRouterAPIKey) != "" {
			openRouterKey = strings.TrimSpace(vis.OpenRouterAPIKey)
		}
		if strings.TrimSpace(vis.TavilyAPIKey) != "" {
			tavilyKey = strings.TrimSpace(vis.TavilyAPIKey)
		}
	}
	return
}

// effectiveOpenRouterKey returns just the resolved OpenRouter API key (see effectiveOpenRouterConfig).
func (s *ChatService) effectiveOpenRouterKey(ctx context.Context, r *http.Request, authSessionID string) string {
	k, _ := s.effectiveOpenRouterConfig(ctx, r, authSessionID)
	return k
}

// EffectiveTavilyKey returns the resolved Tavily API key for this request's user/visitor
// session (owner-configured only — there is no server-wide fallback).
func (s *ChatService) EffectiveTavilyKey(ctx context.Context, r *http.Request) string {
	_, tavilyKey := s.effectiveOpenRouterConfig(ctx, r, "")
	return tavilyKey
}

// applyUsageKeySourceToLLMUsage sets usage.UsedServerKey from effective key resolution (chat / have-a-chat / complete profile).
// LocalAI always runs on the server; every other provider is always backed by a
// user/owner-configured OpenRouter key now that there is no server-wide default.
func (s *ChatService) applyUsageKeySourceToLLMUsage(ctx context.Context, r *http.Request, authSessionID string, usage *appai.LLMUsage) {
	if usage == nil {
		return
	}
	provider := strings.ToLower(strings.TrimSpace(usage.Provider))
	if provider == "" {
		return
	}
	b := provider == "localai"
	usage.UsedServerKey = &b
}

// effectiveProviderByKey resolves the admin-configured AI model named by key (any enabled row in
// the ai_models table) to a live OpenRouter-backed ChatProvider, using the effective OpenRouter key
// for this request's user/visitor session. Returns nil if key is unknown/disabled or no key resolves.
// "localai" is special-cased to the Local AI provider (it's an ai_models row for enable/reorder
// purposes only — see AIModelsService.LocalAIRowEnabled — never routed through OpenRouter).
func (s *ChatService) effectiveProviderByKey(ctx context.Context, r *http.Request, authSessionID, key string) appai.ChatProvider {
	if strings.ToLower(strings.TrimSpace(key)) == "localai" {
		return s.localAIProviderForChat(ctx)
	}
	if s.aiModelsSvc == nil {
		return nil
	}
	m, ok := s.aiModelsSvc.GetByKey(ctx, key)
	if !ok {
		return nil
	}
	apiKey := s.effectiveOpenRouterKey(ctx, r, authSessionID)
	return appai.NewOpenRouterProvider(apiKey, m.ModelSlug, m.Key)
}

// applyOpenRouterModelRouting sets genReq.OpenRouterModels when error failover is enabled
// for a hosted provider request (manual Chat selection or Auto routing to a hosted model).
func (s *ChatService) applyOpenRouterModelRouting(ctx context.Context, genReq *appai.GenerateRequest, providerKey string) {
	if genReq == nil {
		return
	}
	providerKey = strings.ToLower(strings.TrimSpace(providerKey))
	if providerKey == "" || providerKey == "localai" {
		return
	}
	cfg := s.loadHostedLLMProviderOrderConfig(ctx)
	if !cfg.FailoverEnabled {
		return
	}
	slugs := s.openRouterModelSlugsForHosted(ctx, providerKey, true)
	if len(slugs) > 1 {
		genReq.OpenRouterModels = slugs
	}
}

// OpenRouterCredits returns the OpenRouter account credit balance for this request's
// effective key (server default, or user/visitor override), for display in the chat
// status bar after each completion.
func (s *ChatService) OpenRouterCredits(ctx context.Context, r *http.Request) (*appai.OpenRouterCredits, error) {
	key := s.effectiveOpenRouterKey(ctx, r, "")
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("openrouter credits: no API key configured")
	}
	return appai.FetchOpenRouterCredits(ctx, key)
}

// ResolveSummarizer implements appai.SummarizerResolver for one-shot summarization tasks
// (email/message thread summarization, admin bulk-classification/writing-style/psych-profile
// generation). Prefers the well-known "gemini" model key for historical continuity, falling back
// to the default enabled AI model if "gemini" is missing or disabled.
func (s *ChatService) ResolveSummarizer(ctx context.Context) (appai.ChatProvider, string) {
	if p := s.effectiveProviderByKey(ctx, nil, "", "gemini"); p != nil && p.IsAvailable() {
		return p, "gemini"
	}
	if dk, ok := s.DefaultAIModelKey(ctx); ok {
		if p := s.effectiveProviderByKey(ctx, nil, "", dk); p != nil && p.IsAvailable() {
			return p, dk
		}
	}
	return nil, ""
}

// defaultProviderName returns the requested provider key, or the default AI model key when none
// was requested (replacing the old hardcoded "gemini" fallback).
func (s *ChatService) defaultProviderName(ctx context.Context, requested string) string {
	if strings.TrimSpace(requested) != "" {
		return requested
	}
	if s.aiModelsSvc == nil {
		return ""
	}
	dk, _ := s.aiModelsSvc.DefaultKey(ctx)
	return dk
}

// effectiveLocalAIChatModel returns the hot-reloadable chat model (runtime overrides startup default).
func (s *ChatService) effectiveLocalAIChatModel() string {
	if m := strings.TrimSpace(config.LocalAIRuntimeStore().ChatModel()); m != "" {
		return m
	}
	return s.defaultLocalAIModel
}

// LocalAIBaseURL returns the configured Ollama chat base URL.
func (s *ChatService) LocalAIBaseURL() string {
	return strings.TrimSpace(s.defaultLocalAIURL)
}

// LocalAIEmbeddingBaseURL returns the configured Ollama embedding base URL.
func (s *ChatService) LocalAIEmbeddingBaseURL() string {
	return strings.TrimSpace(s.defaultLocalAIEmbeddingURL)
}

// effectiveLocalAIProvider returns a LocalAIProvider using the server-level config.
// LocalAI does not support per-user API key overrides — it always uses the server default.
func (s *ChatService) effectiveLocalAIProvider() appai.ChatProvider {
	return appai.NewLocalAIProvider(s.defaultLocalAIURL, s.defaultLocalAIKey, s.effectiveLocalAIChatModel(), s.defaultLocalAINumCtx)
}

// LocalAIAvailable reports whether Local AI can be used for chat (enabled by user, not
// disabled on the AI Models tab, and infrastructure up).
func (s *ChatService) LocalAIAvailable(ctx context.Context) bool {
	if !s.LocalAIUseEnabled(ctx) {
		return false
	}
	if s.aiModelsSvc != nil && !s.aiModelsSvc.LocalAIRowEnabled(ctx) {
		return false
	}
	return s.LocalAIInfrastructureAvailable(ctx)
}

func (s *ChatService) perRequestGetRAM(r *http.Request) appai.RAMMasterGetter {
	return func() (string, bool) {
		if s.sessionStore == nil || r == nil {
			return "", false
		}
		return s.sessionStore.Get(r)
	}
}

func (s *ChatService) loadToolAccessPolicyDecrypted(ctx context.Context, masterPassword string) appai.ToolAccessPolicy {
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

func (s *ChatService) resolveToolAccessPolicy(ctx context.Context, unlockPassword string) appai.ToolAccessPolicy {
	if policy := s.loadToolAccessPolicyDecrypted(ctx, unlockPassword); policy != nil {
		return policy
	}
	if s.privateStore == nil {
		return nil
	}
	policy, err := s.privateStore.LoadLLMToolsAccessPolicyMirror(ctx)
	if err != nil || policy == nil {
		return nil
	}
	return policy
}

// buildChatTools returns a policy-wrapped executor and filtered tool schemas for the current session tier.
func (s *ChatService) buildChatTools(ctx context.Context, r *http.Request, subjectName string) (appai.ToolExecutor, *[]map[string]any) {
	getRAM := s.perRequestGetRAM(r)
	tier := appai.UnlockTierFromSession(s.sessionStore, r)
	pw, ok := getRAM()
	var policy appai.ToolAccessPolicy
	if ok && pw != "" {
		policy = s.resolveToolAccessPolicy(ctx, pw)
	} else if tier != appai.TierNone && s.privateStore != nil {
		policy, _ = s.privateStore.LoadLLMToolsAccessPolicyMirror(ctx)
	}
	filtered := appai.FilterToolDefinitionsForTier(policy, tier)
	_, tavily := s.effectiveOpenRouterConfig(ctx, r, "")
	base := appai.NewToolExecutor(s.pool, subjectName, tavily, s.pepper, getRAM)
	wrapped := appai.WrapToolExecutorWithPolicy(base, policy, tier)
	return wrapped, &filtered
}

// ChatContextStatus returns the number of LLM tools offered for this request (policy + unlock tier) and reference documents enabled for the AI (task tools and/or system prompt).
func (s *ChatService) ChatContextStatus(ctx context.Context, r *http.Request) (toolCount int, refDocCount int64, err error) {
	_, decls := s.buildChatTools(ctx, r, "")
	if decls != nil {
		toolCount = len(*decls)
	}
	if s.docRepo == nil {
		return toolCount, 0, nil
	}
	refDocCount, err = s.docRepo.CountAvailableForAI(ctx)
	if err != nil {
		return toolCount, 0, err
	}
	return toolCount, refDocCount, nil
}

// ModelAvailable reports whether the named AI model key is configured and available for this
// request's user (and visitor session overrides).
func (s *ChatService) ModelAvailable(ctx context.Context, r *http.Request, key string) bool {
	p := s.effectiveProviderByKey(ctx, r, "", key)
	return p != nil && p.IsAvailable()
}

// AvailableAIModels returns every enabled AI model (key, display name, live availability) for this
// request's user, ordered by sort_order — the data behind GET /chat/availability's "models" field.
// Includes the "localai" row at its table position when enabled.
func (s *ChatService) AvailableAIModels(ctx context.Context, r *http.Request) []map[string]any {
	if s.aiModelsSvc == nil {
		return []map[string]any{}
	}
	models, err := s.aiModelsSvc.ListEnabledInTableOrder(ctx)
	if err != nil {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(models))
	for _, m := range models {
		var available bool
		if strings.ToLower(m.Key) == "localai" {
			available = s.LocalAIAvailable(ctx) && s.LocalAIUseEnabled(ctx)
		} else {
			available = s.ModelAvailable(ctx, r, m.Key)
		}
		out = append(out, map[string]any{
			"key":          m.Key,
			"display_name": m.DisplayName,
			"enabled":      m.Enabled,
			"available":    available,
		})
	}
	return out
}

// DefaultAIModelKey returns the key of the default (first enabled by sort_order) AI model.
func (s *ChatService) DefaultAIModelKey(ctx context.Context) (string, bool) {
	if s.aiModelsSvc == nil {
		return "", false
	}
	return s.aiModelsSvc.DefaultKey(ctx)
}

// ServerRunpodEndpointID returns the server-configured RunPod endpoint ID from the environment.
// Returns "" if neither RUNPOD_IMAGE_CLASSIFY_ENDPOINT_ID nor RUNPOD_IMAGE_CLASSIFY_URL is set.
func (s *ChatService) ServerRunpodEndpointID() string {
	if id := strings.TrimSpace(os.Getenv("RUNPOD_IMAGE_CLASSIFY_ENDPOINT_ID")); id != "" {
		return id
	}
	// Try to extract the endpoint ID segment from a full URL.
	if raw := strings.TrimSpace(os.Getenv("RUNPOD_IMAGE_CLASSIFY_URL")); raw != "" {
		// URL form: https://api.runpod.ai/v2/{id}/runsync
		parts := strings.Split(strings.TrimRight(raw, "/"), "/")
		for i, p := range parts {
			if p == "v2" && i+1 < len(parts) {
				return parts[i+1]
			}
		}
	}
	return ""
}

// ServerRunpodWorkers returns the server-configured IMAGE_AI_CLASSIFICATION_WORKERS value, or 0.
func (s *ChatService) ServerRunpodWorkers() int {
	s2 := strings.TrimSpace(os.Getenv("IMAGE_AI_CLASSIFICATION_WORKERS"))
	if s2 == "" {
		return 0
	}
	n, err := strconv.Atoi(s2)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// ServerElevenLabsKeyConfigured reports whether ELEVENLABS_API_KEY is set server-side (.env).
func (s *ChatService) ServerElevenLabsKeyConfigured() bool {
	return strings.TrimSpace(os.Getenv("ELEVENLABS_API_KEY")) != ""
}

// GenerateResponse runs a full chat generation cycle.
func (s *ChatService) GenerateResponse(ctx context.Context, r *http.Request, req model.ChatRequest) (*model.ChatResponse, error) {
	// Choose provider explicitly so the requested provider is always honoured.
	var provider appai.ChatProvider
	providerName := req.Provider
	var autoRouteMeta map[string]any
	var autoExecCtx *AutoExecutionContext
	switch req.Provider {
	case "auto":
		toolsCount := 0
		_, decls := s.buildChatTools(ctx, r, "")
		if decls != nil {
			toolsCount = len(*decls)
		}
		refDocCount := 0
		if s.docRepo != nil {
			if n, err := s.docRepo.CountAvailableForAI(ctx); err == nil {
				refDocCount = int(n)
			}
		}
		hasSubjectProfile := s.subjectProfileContextAvailable(ctx)
		lastManual := ""
		if req.LastManualHostedProvider != nil {
			lastManual = strings.TrimSpace(*req.LastManualHostedProvider)
		}
		classifyPrompt := req.Prompt
		if req.InactivityNudge {
			classifyPrompt = "[Inactivity check-in nudge — gently continue or check on the user]"
		}
		var resolveErr error
		var execCtx AutoExecutionContext
		provider, providerName, autoRouteMeta, execCtx, resolveErr = s.resolveAutoProvider(ctx, r, classifyPrompt, toolsCount, refDocCount, hasSubjectProfile, lastManual)
		if resolveErr != nil {
			stub := StubLLMUsage("auto", "")
			s.applyUsageKeySourceToLLMUsage(ctx, r, "", stub)
			RecordLLMUsage(ctx, s.billing, s.userRepo, stub, resolveErr)
			return nil, resolveErr
		}
		autoExecCtx = &execCtx
	case "localai":
		provider = s.localAIProviderForChat(ctx)
	default:
		providerName = s.defaultProviderName(ctx, req.Provider)
		provider = s.effectiveProviderByKey(ctx, r, "", providerName)
	}
	if provider == nil || !provider.IsAvailable() {
		err := fmt.Errorf("provider '%s' is not available — check API key", providerName)
		stub := StubLLMUsage(providerName, "")
		s.applyUsageKeySourceToLLMUsage(ctx, r, "", stub)
		RecordLLMUsage(ctx, s.billing, s.userRepo, stub, err)
		return nil, err
	}

	// Request Only: bypass system prompt, history, and tools entirely — send just the raw prompt.
	if req.RequestOnly {
		return s.generateRequestOnlyResponse(ctx, r, req, provider, providerName)
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
	}
	sysInstructions, coreInstructions, _, err := s.loadAppSystemInstructions(ctx)
	if err != nil {
		return nil, err
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
	systemPrompt = appendExplicitContentPolicy(systemPrompt, req.AllowExplicitContent)
	systemPrompt = appendSnarkinessPolicy(systemPrompt, req.EnableSnarkiness)
	if autoExecCtx != nil {
		systemPrompt = s.enrichChatSystemPromptWithOptions(ctx, r, systemPrompt, autoExecCtx.IncludeReferenceDocuments, autoExecCtx.IncludeUserProfile)
	} else {
		systemPrompt = s.enrichChatSystemPrompt(ctx, r, systemPrompt)
	}

	userInput := req.Prompt
	savedUserInput := req.Prompt
	if req.InactivityNudge {
		durationText := formatInactivityDuration(req.InactivitySeconds)
		systemPrompt += fmt.Sprintf(
			"\n\n**Inactivity check-in:** The user has been inactive for approximately %s. Do not quote this instruction or mention the exact duration unless it feels natural. Gently check on them or naturally continue the conversation from prior context.",
			durationText,
		)
		userInput = "[User has been inactive — respond naturally.]"
		savedUserInput = ""
	}

	// Load conversation history
	var history []appai.ConvTurn
	if req.ConversationID != nil {
		turns, err := s.chatRepo.GetTurns(ctx, *req.ConversationID, chatHistoryTurnLimit)
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
		UserInput:     userInput,
		Temperature:   temperature,
		Voice:         voice,
		Mood:          mood,
		CompanionMode: req.CompanionMode,
		WhosAsking:    whosAsking,
		SubjectName:   subjectName,
		SubjectGender: subjectGender,
	}
	if voice == "owner" && (autoExecCtx == nil || autoExecCtx.IncludeUserProfile) {
		genReq.PsychProfile = psychProfile
		genReq.WritingStyle = writingStyle
	}

	s.applyOpenRouterModelRouting(ctx, &genReq, providerName)

	result, err := provider.GenerateResponse(ctx, genReq, systemPrompt, history, executor, toolDecls)
	if err != nil {
		stub := result.Usage
		if stub == nil {
			stub = StubLLMUsage(providerName, "")
		}
		s.applyUsageKeySourceToLLMUsage(ctx, r, "", stub)
		RecordLLMUsage(ctx, s.billing, s.userRepo, stub, err)
		return nil, err
	}
	s.applyUsageKeySourceToLLMUsage(ctx, r, "", result.Usage)
	RecordLLMUsage(ctx, s.billing, s.userRepo, result.Usage, nil)

	// Save turn if conversation ID provided
	if req.ConversationID != nil {
		_ = s.chatRepo.SaveTurn(ctx, *req.ConversationID, savedUserInput, result.PlainText, voice, temperature)
	}

	// Enrich metadata and return
	var embeddedJSON map[string]any
	if err := json.Unmarshal([]byte(result.MetadataJSON), &embeddedJSON); err == nil {
		embeddedJSON["temperature"] = temperature
		embeddedJSON["prompt"] = savedUserInput
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
		if autoRouteMeta != nil {
			embeddedJSON["auto_route"] = autoRouteMeta
			embeddedJSON["provider"] = providerName
		}
	}
	return &model.ChatResponse{
		Response:     result.PlainText,
		Voice:        voice,
		EmbeddedJSON: embeddedJSON,
	}, nil
}

// generateRequestOnlyResponse sends just the raw prompt to the provider — no system prompt,
// no conversation history, and no tools. Used when the client sets request_only on the chat request.
func (s *ChatService) generateRequestOnlyResponse(ctx context.Context, r *http.Request, req model.ChatRequest, provider appai.ChatProvider, providerName string) (*model.ChatResponse, error) {
	voice := "expert"
	if req.Voice != nil && *req.Voice != "" {
		voice = *req.Voice
	}
	genReq := appai.GenerateRequest{UserInput: req.Prompt}
	s.applyOpenRouterModelRouting(ctx, &genReq, providerName)
	noTools := []map[string]any{}
	result, err := provider.GenerateResponse(ctx, genReq, "", nil, nil, &noTools)
	if err != nil {
		stub := result.Usage
		if stub == nil {
			stub = StubLLMUsage(providerName, "")
		}
		s.applyUsageKeySourceToLLMUsage(ctx, r, "", stub)
		RecordLLMUsage(ctx, s.billing, s.userRepo, stub, err)
		return nil, err
	}
	s.applyUsageKeySourceToLLMUsage(ctx, r, "", result.Usage)
	RecordLLMUsage(ctx, s.billing, s.userRepo, result.Usage, nil)

	if req.ConversationID != nil {
		_ = s.chatRepo.SaveTurn(ctx, *req.ConversationID, req.Prompt, result.PlainText, voice, 0)
	}

	var embeddedJSON map[string]any
	if err := json.Unmarshal([]byte(result.MetadataJSON), &embeddedJSON); err == nil {
		embeddedJSON["temperature"] = 0
		embeddedJSON["prompt"] = req.Prompt
		embeddedJSON["voice"] = voice
		embeddedJSON["response_text"] = result.PlainText
		embeddedJSON["request_only"] = true
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

func formatInactivityDuration(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%d seconds", seconds)
	}
	mins := seconds / 60
	rem := seconds % 60
	if rem == 0 {
		if mins == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", mins)
	}
	if mins == 1 {
		return fmt.Sprintf("1 minute and %d seconds", rem)
	}
	return fmt.Sprintf("%d minutes and %d seconds", mins, rem)
}

// Generate A Random Question
func (s *ChatService) GenerateRandomQuestion(ctx context.Context, r *http.Request, req model.ChatRequest) (*model.ChatResponse, error) {
	// Choose provider explicitly so the requested provider is always honoured.
	var provider appai.ChatProvider
	providerName := req.Provider
	switch req.Provider {
	case "localai":
		provider = s.localAIProviderForChat(ctx)
	default:
		providerName = s.defaultProviderName(ctx, req.Provider)
		provider = s.effectiveProviderByKey(ctx, r, "", providerName)
	}
	if provider == nil || !provider.IsAvailable() {
		err := fmt.Errorf("provider '%s' is not available — check API key", providerName)
		stub := StubLLMUsage(providerName, "")
		s.applyUsageKeySourceToLLMUsage(ctx, r, "", stub)
		RecordLLMUsage(ctx, s.billing, s.userRepo, stub, err)
		return nil, err
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

	cfg, _ := s.subjectRepo.GetFirst(ctx)
	subjectName := "Unknown"
	subjectGender := "Male"
	if cfg != nil {
		subjectName = cfg.SubjectName
		subjectGender = cfg.Gender
	}

	he, him, his := genderPronouns(subjectGender)
	replacer := strings.NewReplacer(
		"{SUBJECT_NAME}", subjectName,
		"{he}", he, "{him}", him, "{his}", his,
	)

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

	_, _, questionCore, err := s.loadAppSystemInstructions(ctx)
	if err != nil {
		return nil, err
	}
	questionCore = replacer.Replace(questionCore)

	// Build system prompt
	systemPrompt := questionCore +
		"\n\n**Your Personae:**\n" + voiceText +
		"\n\n**Who is asking:** " + whosAskingText
	systemPrompt = appendExplicitContentPolicy(systemPrompt, req.AllowExplicitContent)
	systemPrompt = appendSnarkinessPolicy(systemPrompt, req.EnableSnarkiness)
	systemPrompt = s.enrichChatSystemPrompt(ctx, r, systemPrompt)

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

	s.applyOpenRouterModelRouting(ctx, &genReq, providerName)

	result, err := provider.GenerateResponse(ctx, genReq, systemPrompt, history, executor, toolDecls)
	if err != nil {
		stub := result.Usage
		if stub == nil {
			stub = StubLLMUsage(providerName, "")
		}
		s.applyUsageKeySourceToLLMUsage(ctx, r, "", stub)
		RecordLLMUsage(ctx, s.billing, s.userRepo, stub, err)
		return nil, err
	}
	s.applyUsageKeySourceToLLMUsage(ctx, r, "", result.Usage)
	RecordLLMUsage(ctx, s.billing, s.userRepo, result.Usage, nil)

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
		// Random-question responses must always expose these keys for the UI
		// (Answer button + question handoff flow in chat.js).
		embeddedJSON["randomQuestion"] = true
		embeddedJSON["randomQuestionText"] = strings.TrimSpace(result.PlainText)
	} else {
		embeddedJSON = map[string]any{
			"randomQuestion":     true,
			"randomQuestionText": strings.TrimSpace(result.PlainText),
			"response_text":      result.PlainText,
			"prompt":             prompt,
			"voice":              voice,
			"temperature":        temperature,
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
	rows, err := s.pool.QueryContext(ctx, `SELECT key, name, description, instructions FROM custom_voices`)
	if err == nil {
		defer func() { _ = rows.Close() }()
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

// ClearConversationHistory removes all stored turns for the conversation (LLM context resets).
func (s *ChatService) ClearConversationHistory(ctx context.Context, id int64) (turnsDeleted int64, err error) {
	return s.chatRepo.ClearConversationTurns(ctx, id)
}

func (s *ChatService) GetTurns(ctx context.Context, conversationID int64, limit int) ([]*model.ChatTurn, error) {
	return s.chatRepo.GetTurns(ctx, conversationID, limit)
}

func (s *ChatService) TurnCount(ctx context.Context, conversationID int64) (int64, error) {
	return s.chatRepo.TurnCount(ctx, conversationID)
}

func (s *ChatService) TurnCountsBatch(ctx context.Context, ids []int64) (map[int64]int64, error) {
	return s.chatRepo.TurnCountsBatch(ctx, ids)
}

// identityExtractionPrompt is sent to Claude/Gemini to extract structured identity fields from free text.
const identityExtractionPrompt = `You are extracting biographical information from a personal profile document.
Return ONLY valid JSON with exactly this structure. Use null for any field you cannot find. Do not add extra fields.
{
  "basic": {
    "full_name": null, "preferred_name": null, "date_of_birth": null, "gender": null,
    "nationality": null, "residence": null, "emails": null, "phones": null,
    "linkedin": null, "social": null, "handedness": null, "religion": null, "other": null
  },
  "health": { "conditions": null, "hospitalisations": null, "surgeries": null, "mental_health": null },
  "family": { "parents": null, "siblings": null, "extended": null, "early_life": null },
  "education": { "primary": null, "secondary": null, "university": null, "vocational": null },
  "career": { "summary": null, "timeline": null, "skills": null, "anecdotes": null },
  "relationships": { "romantic_history": null, "close_friends": null, "social_notes": null },
  "interests": { "sports": null, "arts": null, "music": null, "intellectual": null, "travel": null, "technology": null },
  "personal": { "communication_style": null, "values": null, "rules_for_life": null, "psychological_notes": null },
  "additional": { "notes": null }
}

Document to extract from:
`

// ExtractIdentityProfile parses free-text into structured identity fields for the profile wizard.
// Provider order: enabled AI models in sort_order, then Local AI last.
func (s *ChatService) ExtractIdentityProfile(ctx context.Context, r *http.Request, text string) (map[string]any, error) {
	prompt := identityExtractionPrompt + text

	var ai appai.ChatProvider
	if s.aiModelsSvc != nil {
		models, _ := s.aiModelsSvc.ListEnabled(ctx)
		for _, m := range models {
			p := s.effectiveProviderByKey(ctx, r, "", m.Key)
			if p != nil && p.IsAvailable() {
				ai = p
				break
			}
		}
	}
	if ai == nil {
		if lp := s.localAIProviderForChat(ctx); lp != nil && lp.IsAvailable() {
			ai = lp
		}
	}
	if ai == nil {
		return make(map[string]any), fmt.Errorf("no AI provider available for extraction")
	}

	raw, usage, err := ai.SimpleGenerate(ctx, prompt)
	if err != nil {
		return make(map[string]any), err
	}
	s.applyUsageKeySourceToLLMUsage(ctx, r, "", usage)
	RecordLLMUsage(ctx, s.billing, s.userRepo, usage, nil)

	// Strip markdown code fences if the model wrapped the JSON
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		if idx := strings.Index(raw, "\n"); idx != -1 {
			raw = raw[idx+1:]
		}
		if idx := strings.LastIndex(raw, "```"); idx != -1 {
			raw = raw[:idx]
		}
		raw = strings.TrimSpace(raw)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return make(map[string]any), fmt.Errorf("parse extraction response: %w", err)
	}
	return result, nil
}

// GenerateCompleteProfile builds a multi-step relationship profile for a contact
// from messages and emails, using the specified AI provider (gemini or claude) to summarize,
// and saves it to complete_profiles. Mirrors the Python base_chat_service.get_complete_profile_by_name.
func (s *ChatService) GenerateCompleteProfile(ctx context.Context, name string, provider string, getRAM appai.RAMMasterGetter, authSessionID string) error {
	if getRAM == nil {
		getRAM = func() (string, bool) { return "", false }
	}
	// Use the raw tool executor here, not WrapToolExecutorWithPolicy. The LLM Tools Access policy
	// applies to in-chat tool calls; when policy is unset it denies every tool, which left profile
	// generation with no messages/emails. Reading DB rows for an explicit profile job is not gated by that policy.
	_, tavily := s.effectiveOpenRouterConfig(ctx, nil, authSessionID)
	base := appai.NewToolExecutor(s.pool, "", tavily, s.pepper, getRAM)
	msgsRaw, err := appai.GetMessagesForContactProfile(ctx, s.pool, name)
	if err != nil {
		return fmt.Errorf("get messages: %w", err)
	}
	emailsRaw, err := base(ctx, "get_emails_by_contact", map[string]any{"name": name})
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
	const chunkBytes = 3 * 1024 * 1024 // 3MB
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

	if len(chunks) == 0 {
		return fmt.Errorf("no messages or emails found for %q — use an exact contact name from your archive (messages matched by sender, thread identifiers, and contact email/IDs; emails by from/to)", name)
	}

	// Resolve provider: prefer requested, then fall through the ordered enabled AI models, then Local AI.
	if provider == "" {
		provider = s.defaultProviderName(ctx, "")
	}

	var ai appai.ChatProvider
	tried := map[string]bool{}
	tryProviderKey := func(key string) bool {
		if key == "" || tried[key] {
			return false
		}
		tried[key] = true
		var p appai.ChatProvider
		if key == "localai" {
			p = s.localAIProviderForChat(ctx)
		} else {
			p = s.effectiveProviderByKey(ctx, nil, authSessionID, key)
		}
		if p == nil || !p.IsAvailable() {
			return false
		}
		ai = p
		provider = key
		return true
	}

	if !tryProviderKey(provider) {
		if s.aiModelsSvc != nil {
			models, _ := s.aiModelsSvc.ListEnabled(ctx)
			for _, m := range models {
				if tryProviderKey(m.Key) {
					break
				}
			}
		}
		if ai == nil {
			tryProviderKey("localai")
		}
	}
	if ai == nil {
		return fmt.Errorf("no AI provider available for complete profile")
	}

	var interimSummary string
	total := len(chunks)
	for i, chunk := range chunks {
		chunkMap := map[string]any{"messages": chunk}
		data, _ := json.Marshal(chunkMap)
		prompt := fmt.Sprintf(`
		You are an expert behavioral analyst and conversational profiler. Your objective is to maintain and continuously update a running summary of a long conversation, focusing strictly on communication patterns, relationships, and psychological profiles.

Because the conversation is long, you are processing it in chunks. You will receive the "Current Interim Summary" (what we have learned so far) and "New Data" (the latest chunk of messages).

Your task is to integrate the "New Data" into the "Current Interim Summary" to create a single, updated, cohesive profile.

**CRITICAL INSTRUCTIONS:**
1. **Synthesize, Do Not Append:** Do not simply add a new paragraph at the end. Seamlessly weave new insights into the existing categories. 
2. **Evolve the Analysis:** If the "New Data" shows a shift in behavior, a change in a relationship, or contradicts earlier psychological observations, explicitly note how the dynamic has evolved.
3. **Maintain Conciseness:** Consolidate redundant information. The output must remain highly dense and focused.
4. **Maintain Structure:** You must format your output using the exact Markdown headers provided below.

=== CHUNK PROGRESS ===
Processing chunk %d of %d.

=== CURRENT INTERIM SUMMARY ===
%s

=== NEW DATA TO PROCESS ===
%s

=== REQUIRED OUTPUT FORMAT ===
Return ONLY the updated summary using these exact headers:
### 1. Communication Patterns
[Update with new conversational tactics, power dynamics, tone, or responsiveness.]
### 2. Communication Style
[Update with new communication style, including tone, pace, and language use.]
### 3. Emotional Intelligence
[Update with new emotional intelligence, including empathy, self-awareness, and emotional regulation.]
### 4. Cognitive Style
[Update with new cognitive style, including thinking patterns, decision-making, and problem-solving.]
### 5. Behavioral Patterns
[Update with new behavioral patterns, including habits, routines, and patterns of behavior.]
### 6. Relationship Dynamics
[Update with new alliances, conflicts, dependencies, or shifts in rapport.]
### 7. Psychological Profiles
[Update individual profiles with new motivations, emotional states, or behavioral traits.]
### 8. Key Events
[Update with new key events, including significant moments, milestones, or turning points.]
### 9. Key Insights
[Update with new key insights, including patterns, themes, or patterns of behavior.]
		`, total, i+1, interimSummary, string(data))

		out, usage, err := ai.SimpleGenerate(ctx, prompt)
		if err != nil {
			stub := usage
			if stub == nil {
				stub = StubLLMUsage(provider, "")
			}
			s.applyUsageKeySourceToLLMUsage(ctx, nil, authSessionID, stub)
			RecordLLMUsage(ctx, s.billing, s.userRepo, stub, err)
			return fmt.Errorf("summarize chunk %d/%d: %w", i+1, total, err)
		}
		s.applyUsageKeySourceToLLMUsage(ctx, nil, authSessionID, usage)
		RecordLLMUsage(ctx, s.billing, s.userRepo, usage, nil)
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
