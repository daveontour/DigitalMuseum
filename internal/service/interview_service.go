package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	appai "github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
)

// interviewStylePrompts maps style keys to persona instructions for the interviewer LLM.
var interviewStylePrompts = map[string]string{
	"formal_journalist": `You are a **formal journalist** conducting a structured, professional interview.
- Ask clear, direct questions. Follow up with probing questions when answers are vague or incomplete.
- Cross-reference what the interviewee says against known biographical material — politely note discrepancies.
- Maintain a respectful but persistent tone. You are here to uncover the full story, not just the comfortable parts.
- Structure the interview logically: establish context before diving into specifics.
- Use transitions like "You mentioned earlier..." or "That's interesting — can you elaborate on..." to build a coherent narrative.`,

	"casual_conversational": `You are a **warm, conversational interviewer** — think of a friend catching up over coffee.
- Keep questions natural and flowing. Let the conversation meander into interesting tangents.
- Use humor and empathy to put the interviewee at ease. Share brief observations to show you're engaged.
- Ask "tell me about..." and "what was that like?" style open-ended questions.
- When the interviewee shares something emotional, acknowledge it before moving on.
- Your goal is to draw out stories and personal details that a formal interview might miss.`,

	"academic_researcher": `You are a **methodical academic researcher** conducting an oral history interview.
- Proceed chronologically when possible. Establish dates, places, and people systematically.
- Ask for specific details: names, locations, timeframes. "Approximately when was this?" "Who else was involved?"
- Note gaps in the narrative and return to them later. "Earlier you skipped from 1995 to 2000 — what happened in between?"
- Distinguish between facts and the interviewee's interpretations or feelings about events.
- Your goal is to create a comprehensive, well-documented record.`,

	"podcast_host": `You are an **engaging podcast host** — energetic, curious, and audience-aware.
- Frame questions as if listeners are hearing this story for the first time. Provide context when needed.
- Look for the "hook" — the surprising detail, the turning point, the lesson learned.
- Keep the energy up: react with genuine enthusiasm, surprise, or empathy.
- Ask "what would you tell someone who..." and "if you could go back to that moment..." style questions.
- Your goal is to make the interviewee's story compelling and relatable to an audience.`,

	"therapeutic": `You are a **reflective, therapeutic interviewer** conducting a life review.
- Create a safe, non-judgmental space. Use phrases like "take your time" and "there's no wrong answer."
- Focus on meaning and feelings: "How did that experience shape who you are?" "What did you learn from that?"
- Gently explore both joyful and difficult memories. Never push if the interviewee is reluctant.
- Help the interviewee find patterns and themes across their life story.
- Your goal is to help the interviewee reflect on their life with depth and self-compassion.`,
}

// interviewPurposePrompts maps purpose keys to goal-oriented instructions.
var interviewPurposePrompts = map[string]string{
	"biography": `**Interview Purpose: Biography Creation**
Your goal is to gather material for a comprehensive life biography. Cover major life chapters: childhood, education, career, relationships, turning points, achievements, and reflections. Ensure you explore both the external events and the interviewee's inner experience of them.`,

	"specific_topic": `**Interview Purpose: Specific Topic Deep-Dive**
Focus the interview on the specific topic the interviewee has chosen. Go deep rather than broad — explore multiple facets, related stories, key people involved, and the lasting impact of this topic on the interviewee's life.`,

	"oral_history": `**Interview Purpose: Oral History / Historical Documentation**
Your goal is to create a historically valuable record. Focus on the interviewee's lived experience of historical events, social changes, and cultural moments. Capture sensory details, place descriptions, and the texture of daily life in different eras. Ask about how broader events affected their personal world.`,

	"memoir": `**Interview Purpose: Memoir Assistance**
Help the interviewee identify and develop the most compelling stories from their life. Focus on vivid personal anecdotes, emotional turning points, and moments of growth or revelation. Encourage sensory detail: "What did it look like? Sound like? How did you feel in your body?" The goal is rich, literary-quality raw material.`,

	"legacy": `**Interview Purpose: Legacy Project**
This interview is creating a record for future generations — children, grandchildren, and beyond. Focus on wisdom, values, life lessons, family history, and personal philosophy. Ask: "What do you want people to remember?" "What advice would you give your grandchildren?" "What are you most proud of?" Capture the interviewee's voice and personality.`,
}

// interviewStyleNames maps style keys to display names.
var interviewStyleNames = map[string]string{
	"formal_journalist":     "Formal Journalist",
	"casual_conversational": "Casual Conversational",
	"academic_researcher":   "Academic Researcher",
	"podcast_host":          "Podcast Host",
	"therapeutic":           "Therapeutic / Life Review",
}

// interviewPurposeNames maps purpose keys to display names.
var interviewPurposeNames = map[string]string{
	"biography":      "Biography",
	"specific_topic": "Specific Topic",
	"oral_history":   "Oral History",
	"memoir":         "Memoir",
	"legacy":         "Legacy Project",
}

// StartInterview creates a new interview session and generates the first question.
func (s *ChatService) StartInterview(
	ctx context.Context, r *http.Request, req model.StartInterviewRequest, interviewRepo *repository.InterviewRepo,
) (*model.InterviewTurnResponse, error) {

	if _, ok := interviewStylePrompts[req.Style]; !ok {
		return nil, fmt.Errorf("unknown interview style: %s", req.Style)
	}
	if _, ok := interviewPurposePrompts[req.Purpose]; !ok {
		return nil, fmt.Errorf("unknown interview purpose: %s", req.Purpose)
	}

	provider, providerName := s.pickInterviewProvider(ctx, r, req.Provider)
	if provider == nil {
		return nil, fmt.Errorf("provider '%s' is not available — check API key", req.Provider)
	}

	cfg, _ := s.subjectRepo.GetFirst(ctx)
	subjectName := "the subject"
	subjectGender := "Male"
	if cfg != nil {
		subjectName = cfg.SubjectName
		subjectGender = cfg.Gender
	}

	styleName := interviewStyleNames[req.Style]
	purposeName := interviewPurposeNames[req.Purpose]
	title := fmt.Sprintf("%s — %s (%s)", styleName, purposeName, time.Now().Format("Jan 2, 2006"))

	iv, err := interviewRepo.CreateInterview(ctx, title, req.Style, req.Purpose, req.PurposeDetail, providerName)
	if err != nil {
		return nil, fmt.Errorf("create interview: %w", err)
	}

	systemPrompt := s.buildInterviewSystemPrompt(ctx, req.Style, req.Purpose, req.PurposeDetail, subjectName, subjectGender)
	systemPrompt = s.appendInlinedReferenceDocumentsToSystemPrompt(ctx, r, systemPrompt)
	userPrompt := s.buildInterviewStartPrompt(subjectName, req.PurposeDetail)

	executor, toolDecls := s.buildChatTools(ctx, r, subjectName)
	genReq := appai.GenerateRequest{
		UserInput:     userPrompt,
		Temperature:   0.7,
		Voice:         "expert",
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

	turn, err := interviewRepo.SaveTurn(ctx, iv.ID, result.PlainText)
	if err != nil {
		return nil, fmt.Errorf("save first turn: %w", err)
	}

	return &model.InterviewTurnResponse{
		InterviewID:    iv.ID,
		Question:       turn.Question,
		TurnNumber:     turn.TurnNumber,
		InterviewState: "active",
	}, nil
}

// GenerateInterviewTurn saves the user's answer, then generates the next interviewer question.
func (s *ChatService) GenerateInterviewTurn(
	ctx context.Context, r *http.Request, req model.InterviewTurnRequest, interviewRepo *repository.InterviewRepo,
) (*model.InterviewTurnResponse, error) {

	iv, err := interviewRepo.GetInterview(ctx, req.InterviewID)
	if err != nil {
		return nil, fmt.Errorf("get interview: %w", err)
	}
	if iv == nil {
		return nil, fmt.Errorf("interview %d not found", req.InterviewID)
	}
	if iv.State != "active" {
		return nil, fmt.Errorf("interview is %s, not active", iv.State)
	}

	if err := interviewRepo.SaveAnswer(ctx, iv.ID, req.Answer); err != nil {
		return nil, fmt.Errorf("save answer: %w", err)
	}

	provider, providerName := s.pickInterviewProvider(ctx, r, iv.Provider)
	if provider == nil {
		return nil, fmt.Errorf("provider '%s' is not available", iv.Provider)
	}

	cfg, _ := s.subjectRepo.GetFirst(ctx)
	subjectName := "the subject"
	subjectGender := "Male"
	if cfg != nil {
		subjectName = cfg.SubjectName
		subjectGender = cfg.Gender
	}

	turns, err := interviewRepo.GetTurns(ctx, iv.ID)
	if err != nil {
		return nil, fmt.Errorf("get turns: %w", err)
	}

	systemPrompt := s.buildInterviewSystemPrompt(ctx, iv.Style, iv.Purpose, iv.PurposeDetail, subjectName, subjectGender)
	systemPrompt = s.appendInlinedReferenceDocumentsToSystemPrompt(ctx, r, systemPrompt)

	history := buildInterviewHistory(turns)

	userPrompt := fmt.Sprintf("The interviewee just answered: \"%s\"\n\nBased on the conversation so far and the biographical material available, ask your next interview question. Remember your interviewer style and the purpose of this interview.", req.Answer)

	executor, toolDecls := s.buildChatTools(ctx, r, subjectName)
	genReq := appai.GenerateRequest{
		UserInput:     userPrompt,
		Temperature:   0.7,
		Voice:         "expert",
		Mood:          "neutral",
		CompanionMode: false,
		WhosAsking:    "visitor",
		SubjectName:   subjectName,
		SubjectGender: subjectGender,
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

	turn, err := interviewRepo.SaveTurn(ctx, iv.ID, result.PlainText)
	if err != nil {
		return nil, fmt.Errorf("save turn: %w", err)
	}

	return &model.InterviewTurnResponse{
		InterviewID:    iv.ID,
		Question:       turn.Question,
		TurnNumber:     turn.TurnNumber,
		InterviewState: "active",
	}, nil
}

// PauseInterview sets the interview state to paused.
func (s *ChatService) PauseInterview(
	ctx context.Context, interviewID int64, interviewRepo *repository.InterviewRepo,
) error {
	iv, err := interviewRepo.GetInterview(ctx, interviewID)
	if err != nil {
		return fmt.Errorf("get interview: %w", err)
	}
	if iv == nil {
		return fmt.Errorf("interview %d not found", interviewID)
	}
	if iv.State != "active" {
		return fmt.Errorf("interview is %s, cannot pause", iv.State)
	}
	return interviewRepo.UpdateInterviewState(ctx, interviewID, "paused")
}

// ResumeInterview sets a paused interview back to active and generates a welcome-back question.
func (s *ChatService) ResumeInterview(
	ctx context.Context, r *http.Request, interviewID int64, interviewRepo *repository.InterviewRepo,
) (*model.InterviewTurnResponse, error) {

	iv, err := interviewRepo.GetInterview(ctx, interviewID)
	if err != nil {
		return nil, fmt.Errorf("get interview: %w", err)
	}
	if iv == nil {
		return nil, fmt.Errorf("interview %d not found", interviewID)
	}
	if iv.State != "paused" {
		return nil, fmt.Errorf("interview is %s, cannot resume", iv.State)
	}

	if err := interviewRepo.UpdateInterviewState(ctx, interviewID, "active"); err != nil {
		return nil, err
	}

	provider, providerName := s.pickInterviewProvider(ctx, r, iv.Provider)
	if provider == nil {
		return nil, fmt.Errorf("provider '%s' is not available", iv.Provider)
	}

	cfg, _ := s.subjectRepo.GetFirst(ctx)
	subjectName := "the subject"
	subjectGender := "Male"
	if cfg != nil {
		subjectName = cfg.SubjectName
		subjectGender = cfg.Gender
	}

	turns, err := interviewRepo.GetTurns(ctx, iv.ID)
	if err != nil {
		return nil, fmt.Errorf("get turns: %w", err)
	}

	systemPrompt := s.buildInterviewSystemPrompt(ctx, iv.Style, iv.Purpose, iv.PurposeDetail, subjectName, subjectGender)
	systemPrompt = s.appendInlinedReferenceDocumentsToSystemPrompt(ctx, r, systemPrompt)
	history := buildInterviewHistory(turns)

	userPrompt := "The interview was paused and is now being resumed. Welcome the interviewee back warmly, briefly summarize where you left off, and continue with your next interview question."

	executor, toolDecls := s.buildChatTools(ctx, r, subjectName)
	genReq := appai.GenerateRequest{
		UserInput:     userPrompt,
		Temperature:   0.7,
		Voice:         "expert",
		Mood:          "neutral",
		CompanionMode: false,
		WhosAsking:    "visitor",
		SubjectName:   subjectName,
		SubjectGender: subjectGender,
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

	turn, err := interviewRepo.SaveTurn(ctx, iv.ID, result.PlainText)
	if err != nil {
		return nil, fmt.Errorf("save resume turn: %w", err)
	}

	return &model.InterviewTurnResponse{
		InterviewID:    iv.ID,
		Question:       turn.Question,
		TurnNumber:     turn.TurnNumber,
		InterviewState: "active",
	}, nil
}

// EndInterview generates a writeup from the interview transcript then marks it finished.
func (s *ChatService) EndInterview(
	ctx context.Context, r *http.Request, interviewID int64, interviewRepo *repository.InterviewRepo,
) (*model.EndInterviewResponse, error) {
	iv, err := interviewRepo.GetInterview(ctx, interviewID)
	if err != nil || iv == nil {
		return nil, fmt.Errorf("interview %d not found: %w", interviewID, err)
	}

	turns, err := interviewRepo.GetTurns(ctx, interviewID)
	if err != nil {
		return nil, fmt.Errorf("get turns: %w", err)
	}
	if len(turns) == 0 {
		if err := interviewRepo.UpdateInterviewState(ctx, interviewID, "finished"); err != nil {
			return nil, err
		}
		return &model.EndInterviewResponse{Status: "finished", Writeup: ""}, nil
	}

	provider, providerName := s.pickInterviewProvider(ctx, r, iv.Provider)
	if provider == nil {
		return nil, fmt.Errorf("provider '%s' is not available", iv.Provider)
	}

	cfg, _ := s.subjectRepo.GetFirst(ctx)
	subjectName := "the subject"
	subjectGender := "Male"
	if cfg != nil {
		subjectName = cfg.SubjectName
		subjectGender = cfg.Gender
	}

	writeupPrompt := buildWriteupPrompt(iv.Purpose, iv.PurposeDetail, subjectName, turns)

	systemPrompt := fmt.Sprintf(
		"You are an expert writer. Based on the interview transcript provided, produce a polished, well-structured %s. "+
			"Write in third person. Use the interviewee's actual words where they add colour and authenticity (quote them). "+
			"Organise the material logically with clear sections. The subject's name is %s.",
		writeupTypeLabel(iv.Purpose), subjectName,
	)
	systemPrompt = s.appendInlinedReferenceDocumentsToSystemPrompt(ctx, r, systemPrompt)

	genReq := appai.GenerateRequest{
		UserInput:     writeupPrompt,
		Temperature:   0.6,
		Voice:         "expert",
		Mood:          "neutral",
		CompanionMode: false,
		WhosAsking:    "visitor",
		SubjectName:   subjectName,
		SubjectGender: subjectGender,
	}

	result, err := provider.GenerateResponse(ctx, genReq, systemPrompt, nil, nil, nil)
	if err != nil {
		stub := result.Usage
		if stub == nil {
			stub = StubLLMUsage(providerName, "")
		}
		s.applyUsageKeySourceToLLMUsage(ctx, r, "", stub)
		RecordLLMUsage(ctx, s.billing, s.userRepo, stub, err)
		return nil, fmt.Errorf("writeup generation failed: %w", err)
	}
	s.applyUsageKeySourceToLLMUsage(ctx, r, "", result.Usage)
	RecordLLMUsage(ctx, s.billing, s.userRepo, result.Usage, nil)

	writeup := strings.TrimSpace(result.PlainText)
	if writeup == "" {
		return nil, fmt.Errorf("writeup generation returned empty content")
	}
	if err := interviewRepo.SaveWriteup(ctx, interviewID, writeup); err != nil {
		return nil, fmt.Errorf("save writeup: %w", err)
	}

	return &model.EndInterviewResponse{Status: "finished", Writeup: writeup}, nil
}

// GetInterviewDetail returns an interview with its turns and writeup.
func (s *ChatService) GetInterviewDetail(
	ctx context.Context, interviewID int64, interviewRepo *repository.InterviewRepo,
) (*model.InterviewDetailResponse, error) {
	iv, err := interviewRepo.GetInterview(ctx, interviewID)
	if err != nil || iv == nil {
		return nil, fmt.Errorf("interview %d not found", interviewID)
	}
	turns, err := interviewRepo.GetTurns(ctx, interviewID)
	if err != nil {
		return nil, fmt.Errorf("get turns: %w", err)
	}
	if turns == nil {
		turns = []*model.InterviewTurn{}
	}
	return &model.InterviewDetailResponse{Interview: iv, Turns: turns}, nil
}

// writeupTypeLabel maps purpose keys to the type of document being produced.
func writeupTypeLabel(purpose string) string {
	labels := map[string]string{
		"biography":      "biography",
		"specific_topic": "article on the topic discussed",
		"oral_history":   "oral history narrative",
		"memoir":         "memoir chapter",
		"legacy":         "legacy document",
	}
	if l, ok := labels[purpose]; ok {
		return l
	}
	return "written piece"
}

// buildWriteupPrompt assembles the user prompt for the writeup generation.
func buildWriteupPrompt(purpose, purposeDetail, subjectName string, turns []*model.InterviewTurn) string {
	var sb strings.Builder
	sb.WriteString("Below is the full transcript of an interview with ")
	sb.WriteString(subjectName)
	sb.WriteString(".\n\n")

	if purposeDetail != "" {
		sb.WriteString("Topic/focus: ")
		sb.WriteString(purposeDetail)
		sb.WriteString("\n\n")
	}

	sb.WriteString("## Interview Transcript\n\n")
	for _, t := range turns {
		sb.WriteString(fmt.Sprintf("**Q%d:** %s\n\n", t.TurnNumber, t.Question))
		if t.Answer != nil && *t.Answer != "" {
			sb.WriteString(fmt.Sprintf("**A%d:** %s\n\n", t.TurnNumber, *t.Answer))
		}
	}

	sb.WriteString("\n---\n\n")
	sb.WriteString("Please produce a polished, well-structured ")
	sb.WriteString(writeupTypeLabel(purpose))
	sb.WriteString(" based on this interview material. ")
	sb.WriteString("Include section headings where appropriate. Weave in direct quotes from the interviewee to add authenticity and voice.")
	return sb.String()
}

// ListInterviews returns all interviews for the current user.
func (s *ChatService) ListInterviews(
	ctx context.Context, stateFilter string, interviewRepo *repository.InterviewRepo,
) ([]*model.Interview, error) {
	return interviewRepo.ListInterviews(ctx, stateFilter)
}

// GetInterviewTurns returns all Q&A turns for an interview.
func (s *ChatService) GetInterviewTurns(
	ctx context.Context, interviewID int64, interviewRepo *repository.InterviewRepo,
) ([]*model.InterviewTurn, error) {
	return interviewRepo.GetTurns(ctx, interviewID)
}

// pickInterviewProvider resolves and returns the LLM provider.
func (s *ChatService) pickInterviewProvider(ctx context.Context, r *http.Request, preferred string) (appai.ChatProvider, string) {
	if preferred == "claude" {
		cp := s.effectiveClaudeProvider(ctx, r, "")
		if cp != nil && cp.IsAvailable() {
			return cp, "claude"
		}
	}
	if preferred == "deepseek" {
		dp := s.effectiveDeepSeekProvider(ctx, r, "")
		if dp != nil && dp.IsAvailable() {
			return dp, "deepseek"
		}
	}
	if preferred == "localai" {
		lp := s.effectiveLocalAIProvider()
		if lp != nil && lp.IsAvailable() {
			return lp, "localai"
		}
	}
	gp := s.effectiveGeminiProvider(ctx, r, "")
	if gp != nil && gp.IsAvailable() {
		return gp, "gemini"
	}
	return nil, preferred
}

// buildInterviewSystemPrompt assembles the full system prompt for interview generation.
func (s *ChatService) buildInterviewSystemPrompt(ctx context.Context, style, purpose, purposeDetail, subjectName, subjectGender string) string {
	he, him, his := genderPronouns(subjectGender)
	replacer := strings.NewReplacer(
		"{SUBJECT_NAME}", subjectName,
		"{he}", he, "{him}", him, "{his}", his,
	)

	_, coreInstructions, _, _ := s.loadAppSystemInstructions(ctx)
	coreInstructions = replacer.Replace(coreInstructions)

	stylePrompt := interviewStylePrompts[style]
	purposePrompt := interviewPurposePrompts[purpose]

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`You are an AI interviewer conducting an interview with %s. Your role is to ask thoughtful, well-informed questions — you do NOT answer questions yourself. You only ask them.

**Critical Rules:**
- You ask ONE question at a time. Wait for the interviewee's response before asking the next question.
- Your output should be ONLY the question (with brief transitional remarks if appropriate). Do NOT include stage directions, metadata, or commentary.
- Use the available tools to look up biographical data (emails, messages, reference documents, etc.) to ask informed, specific questions rather than generic ones.
- Build on previous answers — refer back to things the interviewee said earlier when relevant.
- Keep questions focused but open-ended enough to elicit rich, detailed responses.

`, subjectName))

	sb.WriteString("**Your Interviewer Style:**\n")
	sb.WriteString(stylePrompt)
	sb.WriteString("\n\n")
	sb.WriteString(purposePrompt)
	sb.WriteString("\n\n")

	if purposeDetail != "" {
		sb.WriteString(fmt.Sprintf("**Specific Focus:** %s\n\n", purposeDetail))
	}

	sb.WriteString(fmt.Sprintf("**About the interviewee:** %s (%s/%s/%s)\n\n", subjectName, he, him, his))
	sb.WriteString("**Biographical and archive context (use tools to explore further):**\n")
	sb.WriteString(coreInstructions)

	return sb.String()
}

// buildInterviewStartPrompt creates the initial user prompt that kicks off the interview.
func (s *ChatService) buildInterviewStartPrompt(subjectName, purposeDetail string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("You are beginning an interview with %s. ", subjectName))
	sb.WriteString("Before asking your first question, use the available tools to review biographical material (reference documents, subject profile, etc.) to inform your questions. ")
	sb.WriteString("Then introduce yourself briefly and ask your opening question. ")
	sb.WriteString("The opening question should be warm and inviting — something that eases the interviewee into the conversation.")
	if purposeDetail != "" {
		sb.WriteString(fmt.Sprintf("\n\nThe interviewee has indicated they'd like to focus on: %s", purposeDetail))
	}
	return sb.String()
}

// buildInterviewHistory converts stored turns into the ConvTurn format the provider expects.
func buildInterviewHistory(turns []*model.InterviewTurn) []appai.ConvTurn {
	var history []appai.ConvTurn
	for _, t := range turns {
		answer := ""
		if t.Answer != nil {
			answer = *t.Answer
		}
		if answer == "" {
			continue
		}
		history = append(history, appai.ConvTurn{
			UserInput:    answer,
			ResponseText: t.Question,
		})
	}
	return history
}
