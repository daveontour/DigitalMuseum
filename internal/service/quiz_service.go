package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	appai "github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/appctx"
)

// quizQuestionCount is how many multiple-choice questions GenerateQuiz produces.
const quizQuestionCount = 10

// quizProviderAttemptTimeout bounds a single provider's tool-calling loop (up to
// maxToolCallIterations round trips, see internal/ai/provider.go). Without this, a
// provider that keeps calling tools (e.g. paging through every Facebook album) without
// ever producing a final answer can burn through all 15 iterations before failing.
// A legitimate, successful run with a handful of tool calls can genuinely take over a
// minute on slower providers (observed with DeepSeek) — keep this generous enough that
// it only cuts off runaway loops, not real-but-slow completions.
const quizProviderAttemptTimeout = 90 * time.Second

// quizOverallTimeout bounds the whole GenerateQuiz call across every candidate provider,
// so a run of slow/misbehaving providers can't keep the fallback loop going indefinitely.
const quizOverallTimeout = 3 * time.Minute

// QuizQuestion is one multiple-choice question in a generated quiz.
type QuizQuestion struct {
	Question     string   `json:"question"`
	Options      []string `json:"options"`
	CorrectIndex int      `json:"correct_index"`
	FunFact      string   `json:"fun_fact"`
}

// QuizResult is the payload returned to the "Take the Quiz" page.
type QuizResult struct {
	SubjectName string         `json:"subject_name"`
	Questions   []QuizQuestion `json:"questions"`
}

// QuizProgressFunc reports live progress while GenerateQuiz runs: which provider
// (config name, e.g. "gemini") is currently being attempted, and — once a tool call is
// in flight — which tool. tool is "" right when a provider attempt starts, before it has
// made any tool call yet. May be called many times; must be cheap and non-blocking.
type QuizProgressFunc func(provider, tool string)

// GenerateQuiz builds a fresh, AI-authored multiple-choice trivia quiz about the
// archive subject for the unauthenticated "Take the Quiz" login-page feature.
//
// Grounding starts from content the owner has already curated for AI use (the
// subject's AI-authored profile fields, the Identity Profile Wizard output, and
// non-sensitive reference documents flagged "include in system prompt"), then the
// model may call a small, hard-coded allowlist of read-only tools (see
// quizAllowedTools) to pull in more specific facts — including Facebook album
// and post metadata for concrete, specific trivia. That allowlist is the actual
// safety boundary — it deliberately excludes every tool that can return raw
// private messages, emails, interviews, or relationship profiles, since this
// endpoint is reachable by anonymous, unauthenticated visitors on the login page.
//
// onProgress (may be nil) is called as generation proceeds so a caller can surface
// which AI provider and tool are currently in use — see QuizProgressFunc.
func (s *ChatService) GenerateQuiz(ctx context.Context, visitorSvc *VisitorService, includeMature, roastMode bool, onProgress QuizProgressFunc) (*QuizResult, error) {
	if onProgress == nil {
		onProgress = func(string, string) {}
	}

	ownerUID, err := visitorSvc.ResolveArchiveOwnerUserID(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve archive owner: %w", err)
	}
	if ownerUID == notFound {
		return nil, fmt.Errorf("no archive owner configured")
	}

	// Scope reads to the archive owner and force the "restricted visitor" posture
	// so sensitive/private material is never pulled in, regardless of the caller.
	dCtx := context.WithValue(ctx, appctx.ContextKeyUserID, ownerUID)
	dCtx = context.WithValue(dCtx, appctx.ContextKeyVisitorAccess, appctx.VisitorAccess{Restricted: true})

	subjectName, grounding := s.buildQuizGroundingText(dCtx)

	candidates := s.quizProviderCandidates(dCtx)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no AI provider available to generate the quiz")
	}

	// The reference-document tools below check appctx.VisitorAccess only to gate
	// visitor-key hint allowlists (not applicable to this server-triggered call) and
	// otherwise already hard-filter to is_sensitive = FALSE at the SQL layer, so this
	// doesn't reopen anything the quizAllowedTools allowlist doesn't already cover.
	toolCtx := context.WithValue(dCtx, appctx.ContextKeyVisitorAccess, appctx.VisitorAccess{Restricted: false})
	toolDecls := quizToolDecls()

	systemPrompt := buildQuizPrompt(subjectName, grounding, includeMature, roastMode)

	// Overall budget across every candidate — bounds the whole fallback loop even if
	// every provider is slow or keeps calling tools without concluding.
	overallCtx, cancel := context.WithTimeout(toolCtx, quizOverallTimeout)
	defer cancel()

	// Try each configured provider in turn (Hosted LLM Order → LocalAI last). A quota /
	// rate-limit error (or any other provider failure) moves on to the next candidate
	// instead of failing the whole request. Each attempt gets its own bounded timeout so
	// a single misbehaving provider can't consume the entire overall budget by itself.
	// Every attempt's failure is recorded (not just the last) so the final error explains
	// the whole fallback chain instead of looking like it came from a single provider.
	var attemptErrs []string
	for _, cand := range candidates {
		if overallCtx.Err() != nil {
			attemptErrs = append(attemptErrs, fmt.Sprintf("%s: timed out waiting for a turn (%v)", cand.name, overallCtx.Err()))
			break
		}

		onProgress(cand.name, "")
		executor := s.buildQuizToolExecutor(func(tool string) {
			onProgress(cand.name, tool)
		})

		genReq := appai.GenerateRequest{
			UserInput:   "Generate the quiz now.",
			Temperature: 0.2,
			SubjectName: subjectName,
		}

		attemptCtx, cancelAttempt := context.WithTimeout(overallCtx, quizProviderAttemptTimeout)
		result, err := cand.provider.GenerateResponse(attemptCtx, genReq, systemPrompt, nil, executor, toolDecls)
		cancelAttempt()
		if err != nil {
			attemptErrs = append(attemptErrs, quizAttemptErrLabel(cand.name, err))
			stub := StubLLMUsage(cand.name, "")
			s.applyUsageKeySourceToLLMUsage(dCtx, nil, "", stub)
			RecordLLMUsage(dCtx, s.billing, s.userRepo, stub, err)
			continue
		}
		s.applyUsageKeySourceToLLMUsage(dCtx, nil, "", result.Usage)
		RecordLLMUsage(dCtx, s.billing, s.userRepo, result.Usage, nil)

		questions, perr := parseQuizQuestions(quizJSONFromResult(result))
		if perr != nil {
			attemptErrs = append(attemptErrs, quizAttemptErrLabel(cand.name, perr))
			continue
		}

		return &QuizResult{SubjectName: subjectName, Questions: questions}, nil
	}

	if len(attemptErrs) == 0 {
		attemptErrs = []string{"no provider was attempted"}
	}
	return nil, fmt.Errorf("generate quiz: tried %d provider(s), all failed — %s", len(attemptErrs), strings.Join(attemptErrs, " | "))
}

// quizAttemptErrLabel formats one candidate's failure for the aggregated error message.
// Several providers already prefix their own errors with their name (e.g. "gemini: gemini
// API 429: ..."), so this avoids stuttering that into "gemini: gemini: gemini API 429: ...".
func quizAttemptErrLabel(providerName string, err error) string {
	msg := err.Error()
	if strings.HasPrefix(msg, providerName+": ") {
		return msg
	}
	return fmt.Sprintf("%s: %s", providerName, msg)
}

// quizProviderCandidate pairs a resolved, available ChatProvider with its config name.
type quizProviderCandidate struct {
	name     string
	provider appai.ChatProvider
}

// quizProviderByName resolves a ChatProvider for one of the Hosted LLM Order provider
// names (any enabled AI model key) or "localai".
func (s *ChatService) quizProviderByName(ctx context.Context, name string) appai.ChatProvider {
	if name == "localai" {
		return s.localAIProviderForChat(ctx)
	}
	return s.effectiveProviderByKey(ctx, nil, "", name)
}

// quizProviderCandidates returns available providers in the order configured on the AI
// Models tab, which includes Local AI at its own sort position (see defaultHostedLLMProviderOrder).
func (s *ChatService) quizProviderCandidates(ctx context.Context) []quizProviderCandidate {
	names := s.defaultHostedLLMProviderOrder(ctx)

	var out []quizProviderCandidate
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		p := s.quizProviderByName(ctx, name)
		if p == nil || !p.IsAvailable() {
			continue
		}
		out = append(out, quizProviderCandidate{name: name, provider: p})
	}
	return out
}

// quizJSONFromResult returns the raw quiz JSON text from a tool-loop result. Every
// provider's GenerateResponse strips ```json fenced blocks out of PlainText and moves
// the parsed content into MetadataJSON's "embedded_json" field instead (see
// extractEmbeddedJSON in internal/ai/provider.go). If the model wraps its answer in a
// fence despite being told not to, PlainText comes back empty — recover the JSON from
// MetadataJSON in that case instead of failing.
func quizJSONFromResult(result appai.GenerateResult) string {
	if raw := strings.TrimSpace(result.PlainText); raw != "" {
		return raw
	}
	var meta struct {
		EmbeddedJSON []json.RawMessage `json:"embedded_json"`
	}
	if err := json.Unmarshal([]byte(result.MetadataJSON), &meta); err != nil || len(meta.EmbeddedJSON) == 0 {
		return result.PlainText
	}
	return string(meta.EmbeddedJSON[0])
}

// quizAllowedTools is the fixed, hand-picked set of read-only tools GenerateQuiz
// may call. This is the real security boundary for the quiz's tool loop: every
// tool that can surface raw private messages, emails, interviews, or
// relationship/complete profiles is deliberately left out, since GenerateQuiz
// runs for anonymous, unauthenticated visitors on the login page. Facebook
// album/post tools are included deliberately — they return titles, descriptions,
// and dates (not raw private conversations), giving the quiz concrete, specific
// facts to ground questions in.
var quizAllowedTools = map[string]bool{
	"get_user_interests":                true,
	"get_unique_tags_count":             true,
	"get_available_reference_documents": true,
	"get_reference_document":            true,
	"search_facebook_albums":            true,
	"get_album_images":                  true,
	"search_facebook_posts":             true,
}

// buildQuizToolExecutor returns a tool executor restricted to quizAllowedTools. The
// underlying tool implementations still hard-filter to is_sensitive = FALSE reference
// documents and never decrypt without a session master password (none exists here), so
// quiz tool calls can only ever surface non-sensitive, unencrypted (or placeholder)
// content. onTool (may be nil) is invoked with the tool name just before each allowed
// call runs, for progress reporting.
func (s *ChatService) buildQuizToolExecutor(onTool func(toolName string)) appai.ToolExecutor {
	base := appai.NewToolExecutor(s.pool, "", "", s.pepper, nil)
	return func(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
		if !quizAllowedTools[name] {
			return map[string]any{"error": "tool not available for quiz generation"}, nil
		}
		if onTool != nil {
			onTool(name)
		}
		return base(ctx, name, args)
	}
}

// quizToolDecls returns the tool schemas restricted to quizAllowedTools. Static per
// process, so callers may build it once and reuse it across candidate provider attempts.
func quizToolDecls() *[]map[string]any {
	var decls []map[string]any
	for _, td := range appai.GetToolDefinitions() {
		if name, _ := td["name"].(string); quizAllowedTools[name] {
			decls = append(decls, td)
		}
	}
	return &decls
}

// buildQuizGroundingText assembles safe, already-curated facts about the subject:
// AI-authored profile fields, the Identity Profile Wizard output, and
// non-sensitive/non-encrypted reference documents flagged for system-prompt use.
func (s *ChatService) buildQuizGroundingText(ctx context.Context) (subjectName string, grounding string) {
	subjectName = "the archive subject"
	subjectGender := "Male"

	var sb strings.Builder

	cfg, _ := s.subjectRepo.GetFirst(ctx)
	if cfg != nil {
		if strings.TrimSpace(cfg.SubjectName) != "" {
			subjectName = cfg.SubjectName
		}
		if strings.TrimSpace(cfg.Gender) != "" {
			subjectGender = cfg.Gender
		}
		if cfg.PsychologicalProfileAI != nil && strings.TrimSpace(*cfg.PsychologicalProfileAI) != "" {
			sb.WriteString("Psychological / personality profile:\n" + strings.TrimSpace(*cfg.PsychologicalProfileAI) + "\n\n")
		}
		if cfg.WritingStyleAI != nil && strings.TrimSpace(*cfg.WritingStyleAI) != "" {
			sb.WriteString("Writing style notes:\n" + strings.TrimSpace(*cfg.WritingStyleAI) + "\n\n")
		}
	}
	fmt.Fprintf(&sb, "Subject name: %s\nGender: %s\n\n", subjectName, subjectGender)

	if s.cpRepo != nil {
		if found, profile, pending, err := s.cpRepo.GetByName(ctx, subjectName); err == nil && found && !pending && profile != nil {
			if text := strings.TrimSpace(*profile); text != "" {
				sb.WriteString("Identity profile:\n" + text + "\n\n")
			}
		}
	}

	if s.docRepo != nil {
		if rows, err := s.docRepo.ListForSystemPromptInclusion(ctx); err == nil {
			for _, row := range rows {
				if row.IsEncrypted || row.IsSensitive {
					continue
				}
				if strings.TrimSpace(row.ContentType) == "application/pdf" {
					continue
				}
				title := refDocDisplayTitle(row.Title, row.Filename)
				fmt.Fprintf(&sb, "### %s\n%s\n\n", title, string(row.Data))
				if sb.Len() > 12000 {
					break
				}
			}
		}
	}

	grounding = strings.TrimSpace(sb.String())
	if len(grounding) > 14000 {
		grounding = grounding[:14000]
	}
	return subjectName, grounding
}

func buildQuizPrompt(subjectName, grounding string, includeMature, roastMode bool) string {
	matureInstruction := "Keep every question, option, and fun fact strictly family-friendly and appropriate for all ages. Do not reference romance, adult themes, or sensitive/private topics."
	if includeMature {
		matureInstruction = "You may include cheeky, playful, risqué humour where the material supports it (innuendo at most). Keep it tasteful and never sexually explicit or crude."
	}
	roastInstruction := ""
	if roastMode {
		roastInstruction = fmt.Sprintf("\n- ROAST MODE is on: lean into questions and fun facts that gently poke fun at %s — quirky habits, awkward moments, funny mishaps, questionable fashion choices — if the material supports it. Keep it affectionate teasing a friend would recognize, never mean-spirited or humiliating.", subjectName)
	}
	if grounding == "" {
		grounding = "(No detailed background material is available yet for this archive.)"
	}

	return fmt.Sprintf(`You are creating a fun, gameshow-style trivia quiz about %s for visitors of a digital museum/archive app.

Background material about %s (may be partial or thin):
---
%s
---

You also have tools available (get_user_interests, get_unique_tags_count,
get_available_reference_documents, get_reference_document,
search_facebook_albums, get_album_images, search_facebook_posts). Before
finalizing your questions, use them to pull in more specific, factual details
about %s whenever the background material above is thin on a topic:
- call get_available_reference_documents first to see what's available, then
  get_reference_document to read the ones that look relevant.
- call search_facebook_albums and search_facebook_posts with an empty keyword
  to list everything, or a specific keyword to narrow down; use get_album_images
  on an interesting album_id to see what photos it contains (titles, year,
  month only — you can't view the images themselves, so don't describe their
  visual content).
Base questions on the real album names, post text, dates, tags, and interests
these tools return rather than guessing.

IMPORTANT — budget your tool calls: make at most 5-6 tool calls in total, then
write your final answer. Do not enumerate every album, post, or document one by
one (e.g. do not call get_album_images for more than 2-3 albums) — sample a
handful, then stop searching and write the quiz with what you have. Running out
of your tool-call budget without answering is a failure.

Write exactly %d multiple-choice trivia questions about %s grounded in the material above and anything you retrieve via the tools.
Rules:
- Each question has exactly 4 answer options; exactly one is correct.
- Vary the topics and difficulty across the %d questions.
- %s
- Every fact you state (names, dates, places, events) must be directly supported by the background material or a tool result. If you can't find enough specific, verifiable material on a topic, ask a lighter general-knowledge or personality-style question about %s that stays plausible given what you have, rather than inventing specific facts.
- Each question needs a short one-sentence "fun_fact" shown to the player after they answer — an amusing or interesting remark related to the correct answer, and it must also be factually grounded, not invented.%s
- Return ONLY raw JSON, no markdown code fences, no commentary, matching exactly this shape:
{"questions":[{"question":"...","options":["...","...","...","..."],"correct_index":0,"fun_fact":"..."}]}`,
		subjectName, subjectName, grounding, subjectName, quizQuestionCount, subjectName, quizQuestionCount, matureInstruction, subjectName, roastInstruction)
}

// parseQuizQuestions strips optional markdown fences and validates the model's JSON output.
func parseQuizQuestions(raw string) ([]QuizQuestion, error) {
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

	var parsed struct {
		Questions []QuizQuestion `json:"questions"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("parse quiz response: %w", err)
	}

	questions := make([]QuizQuestion, 0, len(parsed.Questions))
	for _, q := range parsed.Questions {
		q.Question = strings.TrimSpace(q.Question)
		if q.Question == "" || len(q.Options) != 4 || q.CorrectIndex < 0 || q.CorrectIndex > 3 {
			continue
		}
		ok := true
		for i, o := range q.Options {
			o = strings.TrimSpace(o)
			if o == "" {
				ok = false
				break
			}
			q.Options[i] = o
		}
		if !ok {
			continue
		}
		q.FunFact = strings.TrimSpace(q.FunFact)
		questions = append(questions, q)
	}
	if len(questions) == 0 {
		return nil, fmt.Errorf("AI did not return any usable quiz questions")
	}
	if len(questions) > quizQuestionCount {
		questions = questions[:quizQuestionCount]
	}
	return questions, nil
}
