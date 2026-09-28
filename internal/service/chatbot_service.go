package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	appai "github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/chatbotdocs"
	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
)

// chatBotHistoryTurnLimit mirrors chatHistoryTurnLimit (chat_service.go) for the ChatBot's own,
// separate conversation history.
const chatBotHistoryTurnLimit = 10

// chatBotSystemPromptBase is the ChatBot's entire system prompt (plus any attached-document
// section appended below) — deliberately minimal: no persona, no voice, no subject/archive
// context, no reference-document inlining. This is what distinguishes the ChatBot from the
// persona chat at the prompt level, matching its "generic assistant" product intent.
const chatBotSystemPromptBase = `You are a helpful, general-purpose AI assistant embedded in this Digital Museum application. ` +
	`You have no persona and are not the archive subject — answer plainly and helpfully as yourself. ` +
	`Use the tools available to you when they help answer the request. If the user has attached documents, ` +
	`their extracted text is included below; if they attached images, look at them directly.`

// ChatBotService orchestrates the generic ChatBot chat modality: no persona/voice/archive
// context, its own history tables (ChatBotRepo, fully separate from ChatRepo), its own MCP tool
// scope (ScopeChatbot + ScopeShared — never ScopeArchive), and document/image attachments. It
// holds a *ChatService and calls its unexported provider-resolution/auto-routing/billing helpers
// directly (same package) rather than duplicating that logic — see CLAUDE.md's ChatBot notes.
type ChatBotService struct {
	chat *ChatService
	repo *repository.ChatBotRepo
}

// NewChatBotService creates a ChatBotService.
func NewChatBotService(chat *ChatService, repo *repository.ChatBotRepo) *ChatBotService {
	return &ChatBotService{chat: chat, repo: repo}
}

// buildChatBotTools mirrors ChatService.buildChatTools but scopes tool discovery/execution to
// ScopeChatbot + ScopeShared — the bundled ChatBot utility server and any owner-added additional
// MCP servers, never the archive-data server.
func (s *ChatBotService) buildChatBotTools(ctx context.Context, r *http.Request) (appai.ToolExecutor, *[]map[string]any) {
	getRAM := s.chat.perRequestGetRAM(r)
	tier := appai.UnlockTierFromSession(s.chat.sessionStore, r)
	pw, ok := getRAM()
	var policy appai.ToolAccessPolicy
	if ok && pw != "" {
		policy = s.chat.resolveToolAccessPolicy(ctx, pw)
	} else if tier != appai.TierNone && s.chat.privateStore != nil {
		policy, _ = s.chat.privateStore.LoadLLMToolsAccessPolicyMirror(ctx)
	}
	filtered := appai.FilterToolDefinitionsForScopesAndTier(policy, tier, appai.ScopeChatbot, appai.ScopeShared)
	_, tavily := s.chat.effectiveOpenRouterConfig(ctx, r, "")
	base := appai.NewMCPToolExecutor(getRAM, tavily, []string{appai.ScopeChatbot, appai.ScopeShared})
	wrapped := appai.WrapToolExecutorWithPolicy(base, policy, tier)
	return wrapped, &filtered
}

// ContextStatus returns the number of LLM tools offered to the ChatBot for this request (policy +
// unlock tier) — the ChatBot analogue of ChatService.ChatContextStatus.
func (s *ChatBotService) ContextStatus(ctx context.Context, r *http.Request) (toolCount int) {
	_, decls := s.buildChatBotTools(ctx, r)
	if decls != nil {
		toolCount = len(*decls)
	}
	return toolCount
}

// providerSupportsVision looks up providerName's OpenRouter model slug in the cached catalog and
// reports whether its architecture lists "image" as an input modality. checked is false when the
// lookup couldn't be performed at all (no aiModelsSvc/catalog, unknown key, or a catalog fetch
// error) — callers should fail open in that case, same as every other best-effort catalog lookup
// in this codebase (e.g. auto_route.go's buildJevModelCriteria).
func (s *ChatBotService) providerSupportsVision(ctx context.Context, providerName string) (supported bool, checked bool) {
	if s.chat.aiModelsSvc == nil || s.chat.openRouterCatalog == nil {
		return true, false
	}
	m, ok := s.chat.aiModelsSvc.GetByKey(ctx, providerName)
	if !ok {
		return true, false
	}
	models, err := s.chat.openRouterCatalog.List(ctx)
	if err != nil {
		return true, false
	}
	for _, cm := range models {
		if cm.ID != m.ModelSlug && cm.CanonicalSlug != m.ModelSlug {
			continue
		}
		for _, mod := range cm.Architecture.InputModalities {
			if mod == "image" {
				return true, true
			}
		}
		return false, true
	}
	return true, false
}

// GenerateResponse runs one ChatBot turn: resolve the provider (manual key, "localai", or "auto"
// via the same JEV-backed router the persona chat uses), resolve any attachments (gating images
// to vision-capable hosted providers and weaving document text into the system prompt), call the
// provider, record billing, and persist the turn.
func (s *ChatBotService) GenerateResponse(ctx context.Context, r *http.Request, req model.ChatBotRequest) (*model.ChatBotResponse, error) {
	var provider appai.ChatProvider
	providerName := req.Provider
	var autoRouteMeta map[string]any
	switch req.Provider {
	case "auto":
		toolsCount := 0
		_, decls := s.buildChatBotTools(ctx, r)
		if decls != nil {
			toolsCount = len(*decls)
		}
		var resolveErr error
		provider, providerName, autoRouteMeta, _, resolveErr = s.chat.resolveAutoProvider(ctx, r, req.Prompt, toolsCount, 0, false, "")
		if resolveErr != nil {
			stub := StubLLMUsage("auto", "")
			s.chat.applyUsageKeySourceToLLMUsage(ctx, r, "", stub)
			RecordLLMUsage(ctx, s.chat.billing, s.chat.userRepo, stub, resolveErr)
			return nil, resolveErr
		}
	case "localai":
		provider = s.chat.localAIProviderForChat(ctx)
	default:
		providerName = s.chat.defaultProviderName(ctx, req.Provider)
		provider = s.chat.effectiveProviderByKey(ctx, r, "", providerName)
	}
	if provider == nil || !provider.IsAvailable() {
		err := fmt.Errorf("provider '%s' is not available — check API key", providerName)
		stub := StubLLMUsage(providerName, "")
		s.chat.applyUsageKeySourceToLLMUsage(ctx, r, "", stub)
		RecordLLMUsage(ctx, s.chat.billing, s.chat.userRepo, stub, err)
		return nil, err
	}

	// Resolve attachments: images become multimodal content parts (gated to hosted
	// vision-capable providers), documents' extracted text is folded into the system prompt.
	var imageAttachments []appai.Attachment
	var docSections []string
	hasImage := false
	for _, id := range req.AttachmentIDs {
		att, aerr := s.repo.GetAttachment(ctx, id)
		if aerr != nil || att == nil {
			continue // best-effort: an attachment that vanished/isn't owned is silently skipped
		}
		if att.Kind == chatbotdocs.KindImage {
			hasImage = true
			imageAttachments = append(imageAttachments, appai.Attachment{
				MimeType:   att.ContentType,
				DataBase64: base64.StdEncoding.EncodeToString(att.Data),
				Filename:   att.Filename,
			})
			continue
		}
		switch {
		case att.ExtractedText != nil && *att.ExtractedText != "":
			docSections = append(docSections, fmt.Sprintf("### %s\n\n%s", att.Filename, *att.ExtractedText))
		case att.ExtractionError != nil:
			docSections = append(docSections, fmt.Sprintf("### %s\n\n[could not extract text: %s]", att.Filename, *att.ExtractionError))
		}
	}
	if hasImage {
		if strings.EqualFold(strings.TrimSpace(providerName), "localai") {
			return nil, fmt.Errorf("image attachments require a hosted AI model — Local AI (Ollama) does not support image input. Switch providers or remove the image")
		}
		if supported, checked := s.providerSupportsVision(ctx, providerName); checked && !supported {
			return nil, fmt.Errorf("the selected model (%s) does not support image input — pick a vision-capable model", providerName)
		}
	}

	systemPrompt := chatBotSystemPromptBase
	if len(docSections) > 0 {
		systemPrompt += "\n\n**Attached documents:**\n\n" + strings.Join(docSections, "\n\n---\n\n")
	}

	temperature := 0.0
	if req.Temperature != nil {
		temperature = *req.Temperature
	}

	var history []appai.ConvTurn
	if req.ConversationID != nil {
		turns, terr := s.repo.GetTurns(ctx, *req.ConversationID, chatBotHistoryTurnLimit)
		if terr == nil {
			for _, t := range turns {
				history = append(history, appai.ConvTurn{UserInput: t.UserInput, ResponseText: t.ResponseText})
			}
		}
	}

	executor, toolDecls := s.buildChatBotTools(ctx, r)
	genReq := appai.GenerateRequest{
		UserInput:   req.Prompt,
		Temperature: temperature,
		Attachments: imageAttachments,
	}
	s.chat.applyOpenRouterModelRouting(ctx, &genReq, providerName)

	result, err := provider.GenerateResponse(ctx, genReq, systemPrompt, history, executor, toolDecls)
	if err != nil {
		stub := result.Usage
		if stub == nil {
			stub = StubLLMUsage(providerName, "")
		}
		s.chat.applyUsageKeySourceToLLMUsage(ctx, r, "", stub)
		RecordLLMUsage(ctx, s.chat.billing, s.chat.userRepo, stub, err)
		return nil, err
	}
	s.chat.applyUsageKeySourceToLLMUsage(ctx, r, "", result.Usage)
	RecordLLMUsage(ctx, s.chat.billing, s.chat.userRepo, result.Usage, nil)

	if req.ConversationID != nil {
		_, _ = s.repo.SaveTurn(ctx, *req.ConversationID, req.Prompt, result.PlainText, providerName, temperature, req.AttachmentIDs)
	}

	var embeddedJSON map[string]any
	if jerr := json.Unmarshal([]byte(result.MetadataJSON), &embeddedJSON); jerr == nil {
		embeddedJSON["temperature"] = temperature
		embeddedJSON["prompt"] = req.Prompt
		embeddedJSON["response_text"] = result.PlainText
		if autoRouteMeta != nil {
			embeddedJSON["auto_route"] = autoRouteMeta
			embeddedJSON["provider"] = providerName
		}
	}
	return &model.ChatBotResponse{Response: result.PlainText, EmbeddedJSON: embeddedJSON}, nil
}

// ── Conversation CRUD ─────────────────────────────────────────────────────────

func (s *ChatBotService) CreateConversation(ctx context.Context, title string) (*model.ChatBotConversation, error) {
	return s.repo.CreateConversation(ctx, title)
}

func (s *ChatBotService) GetConversation(ctx context.Context, id int64) (*model.ChatBotConversation, error) {
	return s.repo.GetConversation(ctx, id)
}

func (s *ChatBotService) ListConversations(ctx context.Context, limit *int) ([]*model.ChatBotConversation, error) {
	return s.repo.ListConversations(ctx, limit)
}

func (s *ChatBotService) UpdateConversation(ctx context.Context, id int64, title *string) (*model.ChatBotConversation, error) {
	return s.repo.UpdateConversation(ctx, id, title)
}

func (s *ChatBotService) DeleteConversation(ctx context.Context, id int64) error {
	return s.repo.DeleteConversation(ctx, id)
}

// ClearConversationHistory removes all stored turns (and their now-orphaned attachments) for the
// conversation (LLM context resets).
func (s *ChatBotService) ClearConversationHistory(ctx context.Context, id int64) (turnsDeleted int64, err error) {
	return s.repo.ClearConversationTurns(ctx, id)
}

func (s *ChatBotService) GetTurns(ctx context.Context, conversationID int64, limit int) ([]*model.ChatBotTurn, error) {
	return s.repo.GetTurns(ctx, conversationID, limit)
}

func (s *ChatBotService) TurnCountsBatch(ctx context.Context, ids []int64) (map[int64]int64, error) {
	return s.repo.TurnCountsBatch(ctx, ids)
}

func (s *ChatBotService) ListPendingAttachments(ctx context.Context, conversationID int64) ([]*model.ChatBotAttachment, error) {
	return s.repo.ListPendingAttachments(ctx, conversationID)
}

func (s *ChatBotService) DeleteAttachment(ctx context.Context, id int64) (bool, error) {
	return s.repo.DeleteAttachment(ctx, id)
}

// DownloadAttachment returns one attachment's full row (including bytes) for a download route.
func (s *ChatBotService) DownloadAttachment(ctx context.Context, id int64) (*model.ChatBotAttachment, error) {
	return s.repo.GetAttachment(ctx, id)
}

// UploadAttachment validates conversation ownership, classifies/extracts the uploaded file, and
// stores it as a pending (turn_id NULL) chatbot_attachments row.
func (s *ChatBotService) UploadAttachment(ctx context.Context, conversationID int64, filename, contentType string, data []byte) (*model.ChatBotAttachment, error) {
	conv, err := s.repo.GetConversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, fmt.Errorf("conversation not found")
	}
	if !chatbotdocs.IsSupportedUpload(contentType, filename) {
		return nil, fmt.Errorf("unsupported file type")
	}
	kind, extractedText, extractionErr := chatbotdocs.Extract(contentType, filename, data)
	return s.repo.CreateAttachment(ctx, conversationID, filename, contentType, kind, int64(len(data)), data, extractedText, extractionErr)
}
