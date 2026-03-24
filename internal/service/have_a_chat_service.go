package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	appai "github.com/daveontour/digitalmuseum/internal/ai"
	"github.com/daveontour/digitalmuseum/internal/model"
)

// GenerateHaveAChatTurn generates one turn of an autonomous Claude↔Gemini conversation.
// The speaking provider responds to the accumulated history in character, using tools to
// find real data from the archive to support its points.
func (s *ChatService) GenerateHaveAChatTurn(
	ctx context.Context,
	r *http.Request,
	req model.HaveAChatRequest,
) (*model.HaveAChatResponse, error) {
	// Choose provider
	provider := s.geminiProvider
	providerName := "gemini"
	if req.SpeakingProvider == "claude" && s.claudeProvider != nil && s.claudeProvider.IsAvailable() {
		provider = s.claudeProvider
		providerName = "claude"
	}
	if provider == nil || !provider.IsAvailable() {
		return nil, fmt.Errorf("provider '%s' is not available — check API key", providerName)
	}

	// Resolve voice for the speaking provider
	voice := req.GeminiVoice
	if req.SpeakingProvider == "claude" {
		voice = req.ClaudeVoice
	}
	if voice == "" {
		voice = "expert"
	}

	// Identify the listening provider name for the system prompt
	listeningName := "Gemini"
	if req.SpeakingProvider == "gemini" {
		listeningName = "Claude"
	}
	speakingName := "Claude"
	if req.SpeakingProvider == "gemini" {
		speakingName = "Gemini"
	}

	temperature := req.Temperature
	if temperature == 0 {
		temperature = 0.7
	}

	// Load subject configuration
	cfg, _ := s.subjectRepo.GetFirst(ctx)
	subjectName := "the subject"
	subjectGender := "Male"
	var sysInstructions, coreInstructions string
	if cfg != nil {
		subjectName = cfg.SubjectName
		subjectGender = cfg.Gender
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

	// Load voice instructions for the speaking provider's chosen voice
	voiceMap := s.loadVoiceInstructions(ctx)
	voiceEntry, ok := voiceMap[voice]
	if !ok {
		voiceEntry = voiceMap["expert"]
		voice = "expert"
	}
	voiceText := replacer.Replace(voiceEntry.Instructions)

	// Build system prompt
	systemPrompt := fmt.Sprintf(`You are %s, an AI assistant, and you are having a lively intellectual conversation with %s (another AI) about the life and digital archive of %s.

Your goal is to:
- Uncover interesting insights, hidden patterns, amusing observations, or blind spots in %s's data
- Engage genuinely and thoughtfully with what %s just said — agree with strong points, push back on weak ones
- Use the available tools to look up actual emails, messages, Facebook posts, and other archive data to ground your comments in evidence
- Keep your response to 2-3 paragraphs; be conversational, analytical, and occasionally witty
- Be respectful of %s but you can have playful, good-natured digs — this is a genuine intellectual debate between two AIs who enjoy sparring

**Your voice and personality:**
%s

**Archive context and subject background:**
%s

%s`,
		speakingName, listeningName, subjectName,
		subjectName,
		listeningName,
		listeningName,
		voiceText,
		coreInstructions,
		sysInstructions,
	)

	// Build user prompt: format history as a narrative
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
			case "claude":
				sb.WriteString(fmt.Sprintf("[Claude]: %s\n\n", turn.Text))
			case "gemini":
				sb.WriteString(fmt.Sprintf("[Gemini]: %s\n\n", turn.Text))
			case "user":
				sb.WriteString(fmt.Sprintf("[User interjected]: %s\n\n", turn.Text))
			}
		}
		sb.WriteString(fmt.Sprintf("\nNow it's your turn as %s. Respond to what was just said. You can use the available tools to look up real data from the archive to support or challenge a point.", speakingName))
	}

	userPrompt := sb.String()

	// Build tool executor and generation request
	executor, toolDecls := s.buildChatTools(ctx, r, subjectName)
	genReq := appai.GenerateRequest{
		UserInput:     userPrompt,
		Temperature:   temperature,
		Voice:         voice,
		Mood:          "neutral",
		CompanionMode: false,
		WhosAsking:    "visitor",
		SubjectName:   subjectName,
		SubjectGender: subjectGender,
	}

	// Pass nil for history — all context is encoded in the user prompt
	result, err := provider.GenerateResponse(ctx, genReq, systemPrompt, nil, executor, toolDecls)
	if err != nil {
		return nil, err
	}

	// Enrich metadata
	var embeddedJSON map[string]any
	if err := json.Unmarshal([]byte(result.MetadataJSON), &embeddedJSON); err == nil {
		embeddedJSON["temperature"] = temperature
		embeddedJSON["voice"] = voice
		embeddedJSON["provider"] = providerName
	}

	return &model.HaveAChatResponse{
		Response:     result.PlainText,
		Provider:     providerName,
		Voice:        voice,
		EmbeddedJSON: embeddedJSON,
	}, nil
}
