package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	appai "github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/model"
)

// GenerateHaveAChatTurn generates one turn of an autonomous voice-to-voice conversation.
// Either voice can be powered by Claude or Gemini independently; if both use the same
// underlying model the system prompt instructs it to maintain the speaking voice's distinct
// identity throughout.
func (s *ChatService) GenerateHaveAChatTurn(
	ctx context.Context,
	r *http.Request,
	req model.HaveAChatRequest,
) (*model.HaveAChatResponse, error) {

	// ── Resolve which slot is speaking ──────────────────────────────────────
	isSlotA := req.SpeakingSlot == "a"

	speakingVoiceKey := req.VoiceA
	listeningVoiceKey := req.VoiceB
	speakingProviderKey := req.ProviderA
	listeningProviderKey := req.ProviderB
	if !isSlotA {
		speakingVoiceKey = req.VoiceB
		listeningVoiceKey = req.VoiceA
		speakingProviderKey = req.ProviderB
		listeningProviderKey = req.ProviderA
	}

	if speakingVoiceKey == "" {
		speakingVoiceKey = "expert"
	}
	if listeningVoiceKey == "" {
		listeningVoiceKey = "expert"
	}

	// ── Pick the actual AI provider ──────────────────────────────────────────
	var provider appai.ChatProvider
	providerName := "gemini"
	if speakingProviderKey == "claude" {
		cp := s.effectiveClaudeProvider(ctx, r, "")
		if cp != nil && cp.IsAvailable() {
			provider = cp
			providerName = "claude"
		}
	} else if speakingProviderKey == "deepseek" {
		dp := s.effectiveDeepSeekProvider(ctx, r, "")
		if dp != nil && dp.IsAvailable() {
			provider = dp
			providerName = "deepseek"
		}
	} else if speakingProviderKey == "localai" {
		lp := s.effectiveLocalAIProvider()
		if lp != nil && lp.IsAvailable() {
			provider = lp
			providerName = "localai"
		}
	}
	if provider == nil {
		provider = s.effectiveGeminiProvider(ctx, r, "")
		providerName = "gemini"
	}
	if provider == nil || !provider.IsAvailable() {
		err := fmt.Errorf("provider '%s' is not available — check API key", speakingProviderKey)
		stub := StubLLMUsage(providerName, "")
		s.applyUsageKeySourceToLLMUsage(ctx, r, "", stub)
		RecordLLMUsage(ctx, s.billing, s.userRepo, stub, err)
		return nil, err
	}

	temperature := req.Temperature
	if temperature == 0 {
		temperature = 0.7
	}

	// ── Subject config ───────────────────────────────────────────────────────
	cfg, _ := s.subjectRepo.GetFirst(ctx)
	subjectName := "the subject"
	subjectGender := "Male"
	var sysInstructions, coreInstructions string
	if cfg != nil {
		subjectName = cfg.SubjectName
		subjectGender = cfg.Gender
	}
	sysInstructions, coreInstructions, _, err := s.loadAppSystemInstructions(ctx)
	if err != nil {
		return nil, err
	}

	he, him, his := genderPronouns(subjectGender)
	replacer := strings.NewReplacer(
		"{SUBJECT_NAME}", subjectName,
		"{he}", he, "{him}", him, "{his}", his,
	)
	sysInstructions = replacer.Replace(sysInstructions)
	coreInstructions = replacer.Replace(coreInstructions)

	// ── Voice display names & instructions ───────────────────────────────────
	voiceMap := s.loadVoiceInstructions(ctx)

	speakingEntry, ok := voiceMap[speakingVoiceKey]
	if !ok {
		speakingEntry = voiceMap["expert"]
		speakingVoiceKey = "expert"
	}
	listeningEntry, ok := voiceMap[listeningVoiceKey]
	if !ok {
		listeningEntry = voiceMap["expert"]
		listeningVoiceKey = "expert"
	}

	speakingName := speakingEntry.Name
	if speakingName == "" {
		speakingName = speakingVoiceKey
	}
	listeningName := listeningEntry.Name
	if listeningName == "" {
		listeningName = listeningVoiceKey
	}

	voiceText := replacer.Replace(speakingEntry.Instructions)

	// ── Response-length / tone instruction ──────────────────────────────────
	var lengthToneInstruction string
	if req.BanterMode {
		lengthToneInstruction = `- Keep your response SHORT — one or two punchy paragraphs at most; wit and speed over depth
- This is a sparring match: quick, sharp, direct. Agree with a single sentence when you agree; push back hard when you don't
- Playful digs, quips, and comedic one-liners are encouraged — think debate club meets comedy roast`
	} else {
		lengthToneInstruction = `- Keep your response to 2–3 paragraphs; be conversational, analytical, and occasionally witty
- Be respectful but enjoy playful, good-natured digs — this is a genuine intellectual debate between two AIs who enjoy sparring`
	}

	// ── Same-model identity reminder ─────────────────────────────────────────
	sameLLM := req.ProviderA == req.ProviderB
	var sameLLMReminder string
	if sameLLM {
		sameLLMReminder = fmt.Sprintf(`
**Identity reminder:** You and %s are both running on the same underlying AI model for this conversation. You must maintain your distinct voice, perspective, and character — %s — at all times. Do not break character, do not acknowledge the shared model, and do not mirror %s's framing. You are %s.`,
			listeningName, speakingName, listeningName, speakingName)
	}

	// ── System prompt ────────────────────────────────────────────────────────
	conversationType := "lively intellectual conversation"
	if req.BanterMode {
		conversationType = "quick-fire sparring match"
	}

	systemPrompt := fmt.Sprintf(`You are %s and you are having a %s with %s about the life and digital archive of %s.

Your goal is to:
- Uncover interesting insights, hidden patterns, amusing observations, or blind spots in %s's data
- Engage genuinely with what %s just said — agree with strong points, push back on weak ones
- Use the available tools to look up actual emails, messages, Facebook posts, and other archive data to ground your comments in evidence
%s

**Your voice and personality:**
%s
%s

**Archive context and subject background:**
%s

%s`,
		speakingName, conversationType, listeningName, subjectName,
		subjectName,
		listeningName,
		lengthToneInstruction,
		voiceText,
		sameLLMReminder,
		coreInstructions,
		sysInstructions,
	)
	systemPrompt = appendExplicitContentPolicy(systemPrompt, req.AllowExplicitContent)
	systemPrompt = s.enrichChatSystemPrompt(ctx, r, systemPrompt)

	// ── User prompt: history as narrative ────────────────────────────────────
	var sb strings.Builder
	topic := strings.TrimSpace(req.Topic)
	if topic != "" {
		sb.WriteString(fmt.Sprintf("**Topic for this conversation:** %s\n\n", topic))
	}

	if len(req.History) == 0 {
		if topic != "" {
			sb.WriteString(fmt.Sprintf("This is the start of the conversation. Begin by introducing an interesting angle you'd like to explore about %s's life related to the topic above. Use the tools to find something concrete to talk about.", subjectName))
		} else {
			sb.WriteString(fmt.Sprintf("This is the start of the conversation. Begin by introducing an interesting angle you'd like to explore about %s's life. Use the tools to find something concrete to start with — a pattern in their messages, an interesting email thread, or a Facebook memory.", subjectName))
		}
	} else {
		sb.WriteString("**The conversation so far:**\n")
		for _, turn := range req.History {
			switch turn.Speaker {
			case "a":
				sb.WriteString(fmt.Sprintf("[%s]: %s\n\n", req.VoiceA, turn.Text))
			case "b":
				sb.WriteString(fmt.Sprintf("[%s]: %s\n\n", req.VoiceB, turn.Text))
			case "user":
				sb.WriteString(fmt.Sprintf("[User interjected]: %s\n\n", turn.Text))
			}
		}
		_ = listeningProviderKey // referenced only for same-LLM detection above
		sb.WriteString(fmt.Sprintf("\nNow it's your turn as %s. Respond to what was just said. You can use the available tools to look up real data from the archive to support or challenge a point.", speakingName))
	}

	userPrompt := sb.String()

	// ── Generate ─────────────────────────────────────────────────────────────
	executor, toolDecls := s.buildChatTools(ctx, r, subjectName)
	genReq := appai.GenerateRequest{
		UserInput:     userPrompt,
		Temperature:   temperature,
		Voice:         speakingVoiceKey,
		Mood:          "neutral",
		CompanionMode: false,
		WhosAsking:    "visitor",
		SubjectName:   subjectName,
		SubjectGender: subjectGender,
	}

	result, err := provider.GenerateResponse(ctx, genReq, systemPrompt, nil, executor, toolDecls)
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

	var embeddedJSON map[string]any
	if err := json.Unmarshal([]byte(result.MetadataJSON), &embeddedJSON); err == nil {
		embeddedJSON["temperature"] = temperature
		embeddedJSON["voice"] = speakingVoiceKey
		embeddedJSON["provider"] = providerName
		embeddedJSON["banter_mode"] = req.BanterMode
	}

	return &model.HaveAChatResponse{
		Response:     result.PlainText,
		Provider:     providerName,
		Voice:        speakingVoiceKey,
		EmbeddedJSON: embeddedJSON,
	}, nil
}
