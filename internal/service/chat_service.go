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

	appai "github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/appctx"
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
	defaultGeminiKey     string
	defaultGeminiModel   string
	defaultAnthropicKey  string
	defaultClaudeModel   string
	defaultDeepSeekKey   string
	defaultDeepSeekModel string
	defaultTavilyKey     string
	defaultLocalAIURL    string
	defaultLocalAIKey    string
	defaultLocalAIModel  string
	defaultLocalAINumCtx int
	pythonStaticDir      string
	pepper               string
	sessionStore         *keystore.SessionMasterStore
	privateStore         *PrivateStoreService
	billing              *repository.BillingRepo
}

// NewChatService creates a ChatService. Server defaults come from cfg/env; authenticated
// users may override keys and models in the users table.
func NewChatService(
	chatRepo *repository.ChatRepo,
	subjectRepo *repository.SubjectConfigRepo,
	appInstrRepo *repository.AppSystemInstructionsRepo,
	cpRepo *repository.CompleteProfileRepo,
	docRepo *repository.DocumentRepo,
	pool *sql.DB,
	userRepo *repository.UserRepo,
	defaultGeminiKey, defaultGeminiModel, defaultAnthropicKey, defaultClaudeModel string,
	defaultDeepSeekKey, defaultDeepSeekModel, defaultTavilyKey string,
	defaultLocalAIURL, defaultLocalAIKey, defaultLocalAIModel string,
	defaultLocalAINumCtx int,
	pythonStaticDir string,
	pepper string,
	sessionStore *keystore.SessionMasterStore,
	privateStore *PrivateStoreService,
	billing *repository.BillingRepo,
) *ChatService {
	return &ChatService{
		chatRepo:             chatRepo,
		subjectRepo:          subjectRepo,
		appInstrRepo:         appInstrRepo,
		cpRepo:               cpRepo,
		docRepo:              docRepo,
		pool:                 pool,
		userRepo:             userRepo,
		defaultGeminiKey:     defaultGeminiKey,
		defaultGeminiModel:   defaultGeminiModel,
		defaultAnthropicKey:  defaultAnthropicKey,
		defaultClaudeModel:   defaultClaudeModel,
		defaultDeepSeekKey:   defaultDeepSeekKey,
		defaultDeepSeekModel: defaultDeepSeekModel,
		defaultTavilyKey:     defaultTavilyKey,
		defaultLocalAIURL:    defaultLocalAIURL,
		defaultLocalAIKey:    defaultLocalAIKey,
		defaultLocalAIModel:  defaultLocalAIModel,
		defaultLocalAINumCtx: defaultLocalAINumCtx,
		pythonStaticDir:      pythonStaticDir,
		pepper:               pepper,
		sessionStore:         sessionStore,
		privateStore:         privateStore,
		billing:              billing,
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

// effectiveAIConfig merges server defaults, the archive owner's saved overrides (users row), then
// visitor session overrides (sessions.visitor_llm_overrides) when the request is a visitor session.
// authSessionID is used when r is nil (e.g. background jobs); otherwise the cookie on r is read.
func (s *ChatService) effectiveAIConfig(ctx context.Context, r *http.Request, authSessionID string) (geminiKey, geminiModel, anthropicKey, claudeModel, tavilyKey, deepseekKey, deepseekModel string) {
	geminiKey = s.defaultGeminiKey
	geminiModel = s.defaultGeminiModel
	anthropicKey = s.defaultAnthropicKey
	claudeModel = s.defaultClaudeModel
	tavilyKey = s.defaultTavilyKey
	deepseekKey = s.defaultDeepSeekKey
	deepseekModel = s.defaultDeepSeekModel
	uid := appctx.UserIDFromCtx(ctx)
	useOwnerLLM := true
	useServerLLM := true
	if appctx.IsVisitorFromCtx(ctx) && s.userRepo != nil {
		sid := readAuthSessionID(r, authSessionID)
		if sid != "" {
			if pol, err := s.userRepo.GetVisitorSessionLLMPolicy(ctx, sid); err == nil && pol != nil {
				useOwnerLLM = pol.AllowOwnerKeys
				useServerLLM = pol.AllowServerKeys
			}
		}
	}
	if uid != 0 && s.userRepo != nil {
		if stored, err := s.userRepo.GetUserLLMStored(ctx, uid); err == nil && stored != nil {
			allow := stored.AllowServerLLMKeys
			if useOwnerLLM {
				if strings.TrimSpace(stored.GeminiAPIKey) != "" {
					geminiKey = strings.TrimSpace(stored.GeminiAPIKey)
				} else if !allow {
					geminiKey = ""
				}
				if strings.TrimSpace(stored.GeminiModel) != "" {
					geminiModel = strings.TrimSpace(stored.GeminiModel)
				}
				if strings.TrimSpace(stored.AnthropicAPIKey) != "" {
					anthropicKey = strings.TrimSpace(stored.AnthropicAPIKey)
				} else if !allow {
					anthropicKey = ""
				}
				if strings.TrimSpace(stored.ClaudeModel) != "" {
					claudeModel = strings.TrimSpace(stored.ClaudeModel)
				}
				if strings.TrimSpace(stored.TavilyAPIKey) != "" {
					tavilyKey = strings.TrimSpace(stored.TavilyAPIKey)
				} else if !allow {
					tavilyKey = ""
				}
				if strings.TrimSpace(stored.DeepSeekAPIKey) != "" {
					deepseekKey = strings.TrimSpace(stored.DeepSeekAPIKey)
				} else if !allow {
					deepseekKey = ""
				}
				if strings.TrimSpace(stored.DeepSeekModel) != "" {
					deepseekModel = strings.TrimSpace(stored.DeepSeekModel)
				}
			} else {
				geminiKey = s.defaultGeminiKey
				if !useServerLLM {
					geminiKey = ""
				} else if !allow {
					geminiKey = ""
				}
				anthropicKey = s.defaultAnthropicKey
				if !useServerLLM {
					anthropicKey = ""
				} else if !allow {
					anthropicKey = ""
				}
				tavilyKey = s.defaultTavilyKey
				if !useServerLLM {
					tavilyKey = ""
				} else if !allow {
					tavilyKey = ""
				}
				deepseekKey = s.defaultDeepSeekKey
				if !useServerLLM {
					deepseekKey = ""
				} else if !allow {
					deepseekKey = ""
				}
			}
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
		if strings.TrimSpace(vis.GeminiAPIKey) != "" {
			geminiKey = strings.TrimSpace(vis.GeminiAPIKey)
		}
		if strings.TrimSpace(vis.GeminiModel) != "" {
			geminiModel = strings.TrimSpace(vis.GeminiModel)
		}
		if strings.TrimSpace(vis.AnthropicAPIKey) != "" {
			anthropicKey = strings.TrimSpace(vis.AnthropicAPIKey)
		}
		if strings.TrimSpace(vis.ClaudeModel) != "" {
			claudeModel = strings.TrimSpace(vis.ClaudeModel)
		}
		if strings.TrimSpace(vis.TavilyAPIKey) != "" {
			tavilyKey = strings.TrimSpace(vis.TavilyAPIKey)
		}
		if strings.TrimSpace(vis.DeepSeekAPIKey) != "" {
			deepseekKey = strings.TrimSpace(vis.DeepSeekAPIKey)
		}
		if strings.TrimSpace(vis.DeepSeekModel) != "" {
			deepseekModel = strings.TrimSpace(vis.DeepSeekModel)
		}
	}
	return
}

// effectiveAIKeySource reports whether the effective Gemini, Claude, and DeepSeek API keys match server defaults (env).
// User or visitor overrides that differ from those defaults yield false for that provider.
func (s *ChatService) effectiveAIKeySource(ctx context.Context, r *http.Request, authSessionID string) (geminiFromServer, claudeFromServer, deepseekFromServer bool) {
	gk, _, ak, _, _, dsk, _ := s.effectiveAIConfig(ctx, r, authSessionID)
	gk = strings.TrimSpace(gk)
	ak = strings.TrimSpace(ak)
	dsk = strings.TrimSpace(dsk)
	dg := strings.TrimSpace(s.defaultGeminiKey)
	da := strings.TrimSpace(s.defaultAnthropicKey)
	dd := strings.TrimSpace(s.defaultDeepSeekKey)
	geminiFromServer = gk != "" && dg != "" && gk == dg
	claudeFromServer = ak != "" && da != "" && ak == da
	deepseekFromServer = dsk != "" && dd != "" && dsk == dd
	return
}

// applyUsageKeySourceToLLMUsage sets usage.UsedServerKey from effective key resolution (chat / have-a-chat / complete profile).
func (s *ChatService) applyUsageKeySourceToLLMUsage(ctx context.Context, r *http.Request, authSessionID string, usage *appai.LLMUsage) {
	if usage == nil {
		return
	}
	gS, cS, dS := s.effectiveAIKeySource(ctx, r, authSessionID)
	var v bool
	switch strings.ToLower(strings.TrimSpace(usage.Provider)) {
	case "gemini":
		v = gS
	case "claude":
		v = cS
	case "localai":
		v = true // LocalAI always runs on the server
	case "deepseek":
		v = dS
	default:
		return
	}
	b := v
	usage.UsedServerKey = &b
}

func (s *ChatService) effectiveGeminiProvider(ctx context.Context, r *http.Request, authSessionID string) appai.ChatProvider {
	k, m, _, _, _, _, _ := s.effectiveAIConfig(ctx, r, authSessionID)
	return appai.NewGeminiProvider(k, m)
}

func (s *ChatService) effectiveClaudeProvider(ctx context.Context, r *http.Request, authSessionID string) appai.ChatProvider {
	_, _, k, m, _, _, _ := s.effectiveAIConfig(ctx, r, authSessionID)
	return appai.NewClaudeProvider(k, m)
}

func (s *ChatService) effectiveDeepSeekProvider(ctx context.Context, r *http.Request, authSessionID string) appai.ChatProvider {
	_, _, _, _, _, k, m := s.effectiveAIConfig(ctx, r, authSessionID)
	return appai.NewDeepSeekProvider(k, m)
}

// effectiveLocalAIProvider returns a LocalAIProvider using the server-level config.
// LocalAI does not support per-user API key overrides — it always uses the server default.
func (s *ChatService) effectiveLocalAIProvider() appai.ChatProvider {
	return appai.NewLocalAIProvider(s.defaultLocalAIURL, s.defaultLocalAIKey, s.defaultLocalAIModel, s.defaultLocalAINumCtx)
}

// LocalAIAvailable reports whether the LocalAI provider is configured.
func (s *ChatService) LocalAIAvailable() bool {
	p := s.effectiveLocalAIProvider()
	return p != nil && p.IsAvailable()
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
	_, _, _, _, tavily, _, _ := s.effectiveAIConfig(ctx, r, "")
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

// GeminiAvailable reports whether the Gemini provider is configured for this request's user (and visitor session overrides).
func (s *ChatService) GeminiAvailable(ctx context.Context, r *http.Request) bool {
	p := s.effectiveGeminiProvider(ctx, r, "")
	return p != nil && p.IsAvailable()
}

// ClaudeAvailable reports whether the Claude provider is configured for this request's user (and visitor session overrides).
func (s *ChatService) ClaudeAvailable(ctx context.Context, r *http.Request) bool {
	p := s.effectiveClaudeProvider(ctx, r, "")
	return p != nil && p.IsAvailable()
}

// DeepSeekAvailable reports whether DeepSeek is configured for this request's user (and visitor session overrides).
func (s *ChatService) DeepSeekAvailable(ctx context.Context, r *http.Request) bool {
	p := s.effectiveDeepSeekProvider(ctx, r, "")
	return p != nil && p.IsAvailable()
}

// ServerTavilyKeyConfigured reports whether a non-empty Tavily API key is configured server-side (e.g. TAVILY_API_KEY in .env).
func (s *ChatService) ServerTavilyKeyConfigured() bool {
	return strings.TrimSpace(s.defaultTavilyKey) != ""
}

// ServerRunpodKeyConfigured reports whether RUNPOD_API_KEY is set server-side (.env).
func (s *ChatService) ServerRunpodKeyConfigured() bool {
	return strings.TrimSpace(os.Getenv("RUNPOD_API_KEY")) != ""
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

// ServerGeminiModelDefault returns the trimmed server default Gemini model name (e.g. from GEMINI_MODEL_NAME), or "".
func (s *ChatService) ServerGeminiModelDefault() string {
	return strings.TrimSpace(s.defaultGeminiModel)
}

// ServerClaudeModelDefault returns the trimmed server default Claude model name, or "".
func (s *ChatService) ServerClaudeModelDefault() string {
	return strings.TrimSpace(s.defaultClaudeModel)
}

// ServerDeepSeekModelDefault returns the trimmed server default DeepSeek model name, or "".
func (s *ChatService) ServerDeepSeekModelDefault() string {
	return strings.TrimSpace(s.defaultDeepSeekModel)
}

// ServerGeminiModelDefaultSet reports whether a non-empty default Gemini model is set server-side (e.g. GEMINI_MODEL_NAME in .env).
func (s *ChatService) ServerGeminiModelDefaultSet() bool {
	return s.ServerGeminiModelDefault() != ""
}

// ServerClaudeModelDefaultSet reports whether a non-empty default Claude model is set server-side (e.g. CLAUDE_MODEL_NAME / env equivalent in .env).
func (s *ChatService) ServerClaudeModelDefaultSet() bool {
	return s.ServerClaudeModelDefault() != ""
}

// ServerDeepSeekModelDefaultSet reports whether a non-empty default DeepSeek model is set server-side (e.g. DEEPSEEK_MODEL_NAME in .env).
func (s *ChatService) ServerDeepSeekModelDefaultSet() bool {
	return s.ServerDeepSeekModelDefault() != ""
}

// GenerateResponse runs a full chat generation cycle.
func (s *ChatService) GenerateResponse(ctx context.Context, r *http.Request, req model.ChatRequest) (*model.ChatResponse, error) {
	// Choose provider explicitly so the requested provider is always honoured.
	var provider appai.ChatProvider
	providerName := req.Provider
	switch req.Provider {
	case "claude":
		provider = s.effectiveClaudeProvider(ctx, r, "")
	case "deepseek":
		provider = s.effectiveDeepSeekProvider(ctx, r, "")
	case "localai":
		provider = s.effectiveLocalAIProvider()
	default:
		providerName = "gemini"
		provider = s.effectiveGeminiProvider(ctx, r, "")
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
	systemPrompt = s.appendInlinedReferenceDocumentsToSystemPrompt(ctx, r, systemPrompt)

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
	if voice == "owner" {
		genReq.PsychProfile = psychProfile
		genReq.WritingStyle = writingStyle
	}

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
	case "claude":
		provider = s.effectiveClaudeProvider(ctx, r, "")
	case "deepseek":
		provider = s.effectiveDeepSeekProvider(ctx, r, "")
	case "localai":
		provider = s.effectiveLocalAIProvider()
	default:
		providerName = "gemini"
		provider = s.effectiveGeminiProvider(ctx, r, "")
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
	systemPrompt = s.appendInlinedReferenceDocumentsToSystemPrompt(ctx, r, systemPrompt)

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
// Provider order: Claude → Gemini → DeepSeek → Local AI.
func (s *ChatService) ExtractIdentityProfile(ctx context.Context, r *http.Request, text string) (map[string]any, error) {
	prompt := identityExtractionPrompt + text

	type simpleGen interface {
		SimpleGenerate(context.Context, string) (string, *appai.LLMUsage, error)
	}

	var ai simpleGen
	var providerName string

	tryProvider := func(name string, p appai.ChatProvider) bool {
		if p == nil || !p.IsAvailable() {
			return false
		}
		switch name {
		case "claude":
			if cp, ok := p.(*appai.ClaudeProvider); ok && cp != nil {
				ai, providerName = cp, name
				return true
			}
		case "gemini":
			if gp, ok := p.(*appai.GeminiProvider); ok && gp != nil {
				ai, providerName = gp, name
				return true
			}
		case "deepseek":
			if dp, ok := p.(*appai.DeepSeekProvider); ok && dp != nil {
				ai, providerName = dp, name
				return true
			}
		case "localai":
			if lp, ok := p.(*appai.LocalAIProvider); ok && lp != nil {
				ai, providerName = lp, name
				return true
			}
		}
		return false
	}

	switch {
	case tryProvider("claude", s.effectiveClaudeProvider(ctx, r, "")):
	case tryProvider("gemini", s.effectiveGeminiProvider(ctx, r, "")):
	case tryProvider("deepseek", s.effectiveDeepSeekProvider(ctx, r, "")):
	case tryProvider("localai", s.effectiveLocalAIProvider()):
	default:
		return make(map[string]any), fmt.Errorf("no AI provider available for extraction")
	}

	var raw string
	var usage *appai.LLMUsage
	var err error
	raw, usage, err = ai.SimpleGenerate(ctx, prompt)
	if err != nil {
		return make(map[string]any), err
	}
	_ = providerName
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
	_, _, _, _, tavily, _, _ := s.effectiveAIConfig(ctx, nil, authSessionID)
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

	// Resolve provider: prefer requested, default gemini, fallback claude if gemini unavailable
	if provider == "" {
		provider = "gemini"
	}
	claudeP := s.effectiveClaudeProvider(ctx, nil, authSessionID)
	deepseekP := s.effectiveDeepSeekProvider(ctx, nil, authSessionID)
	geminiP := s.effectiveGeminiProvider(ctx, nil, authSessionID)
	localaiP := s.effectiveLocalAIProvider()
	if provider == "deepseek" && (deepseekP == nil || !deepseekP.IsAvailable()) {
		provider = "gemini" // fallback
	}
	if provider == "claude" && (claudeP == nil || !claudeP.IsAvailable()) {
		provider = "gemini" // fallback
	}
	if provider == "gemini" && (geminiP == nil || !geminiP.IsAvailable()) {
		provider = "claude" // fallback
	}
	if provider == "claude" && (claudeP == nil || !claudeP.IsAvailable()) {
		if deepseekP != nil && deepseekP.IsAvailable() {
			provider = "deepseek"
		}
	}

	type simpleGen interface {
		SimpleGenerate(context.Context, string) (string, *appai.LLMUsage, error)
	}
	var ai simpleGen
	switch provider {
	case "claude":
		if cp, ok := claudeP.(*appai.ClaudeProvider); ok && cp != nil {
			ai = cp
		}
	case "deepseek":
		if dp, ok := deepseekP.(*appai.DeepSeekProvider); ok && dp != nil {
			ai = dp
		}
	case "gemini":
		if gp, ok := geminiP.(*appai.GeminiProvider); ok && gp != nil {
			ai = gp
		}
	case "localai":
		if lp, ok := localaiP.(*appai.LocalAIProvider); ok && lp != nil {
			ai = lp
		}
	}
	if ai == nil {
		return fmt.Errorf("no AI provider available for complete profile (gemini, claude, deepseek, or localai required)")
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
