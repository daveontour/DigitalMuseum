package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	appai "github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/appctx"
)

// autoClassifierNumCtx is the Ollama context window for Auto routing classification only.
const autoClassifierNumCtx = 4096

// AutoRouteDecision is the parsed output of the Local AI classifier.
type AutoRouteDecision struct {
	Decision                  string // "local" or "hosted"
	Reason                    string
	Confidence                float64
	ClassifierFallback        bool
	ClassifierError           string
	NeedsReferenceDocuments   bool
	NeedsUserProfile          bool
}

// AutoExecutionContext controls optional user context included in the follow-up chat request.
type AutoExecutionContext struct {
	IncludeReferenceDocuments bool
	IncludeUserProfile        bool
}

type autoClassifierJSON struct {
	Route                     string  `json:"route"`
	Reason                    string  `json:"reason"`
	Confidence                float64 `json:"confidence"`
	NeedsReferenceDocuments   *bool   `json:"needs_reference_documents"`
	NeedsUserProfile          *bool   `json:"needs_user_profile"`
}

func buildAutoClassifierPrompt(userPrompt string, toolsCount, refDocCount int, hasSubjectProfile bool) string {
	profileAvail := "no"
	if hasSubjectProfile {
		profileAvail = "yes"
	}
	return fmt.Sprintf(`You are a routing classifier for a digital archive chat assistant. The assistant has access to %d tools (database lookups, time/date, search, counts, etc.).

Available context (may be omitted from the follow-up request if not needed):
- %d reference document(s) can be inlined in the system prompt
- Subject psychological/writing-style profile summaries available: %s

Decide whether the user's request should be handled by a LOCAL small model (with tools) or a HOSTED larger model.

Route LOCAL when the request:
- Can be answered with one or two simple tool calls (e.g. current time/date, a count, a single lookup)
- Needs only brief factual output with minimal reasoning or narrative
- Is a straightforward command or question with low inference requirements

Route HOSTED when the request:
- Needs multi-step reasoning, synthesis across many records, or rich prose
- Requires persona-heavy, creative, reflective, or emotionally nuanced answers
- Explores the archive open-endedly or asks for summaries, stories, or essays
- Is ambiguous, conversational, or would benefit from stronger language capability

Also decide whether the follow-up request needs:
- needs_reference_documents: true if answering requires material from the user's inlined reference documents (identity notes, background docs, etc.); false for generic/time/factual/tool-only queries
- needs_user_profile: true if answering requires the subject's psychological or writing-style profile or archive data inventory; false when the question does not depend on who the archive subject is or what data they have

Reply with ONLY valid JSON (no markdown fences):
{"route":"local" or "hosted","reason":"brief explanation","confidence":0.0 to 1.0,"needs_reference_documents":true or false,"needs_user_profile":true or false}

User request:
%s`, toolsCount, refDocCount, profileAvail, userPrompt)
}

func parseAutoClassifierResponse(raw string) (AutoRouteDecision, error) {
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
	var parsed autoClassifierJSON
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return AutoRouteDecision{}, fmt.Errorf("invalid classifier JSON: %w", err)
	}
	route := strings.ToLower(strings.TrimSpace(parsed.Route))
	if route != "local" && route != "hosted" {
		return AutoRouteDecision{}, fmt.Errorf("invalid route %q", parsed.Route)
	}
	reason := strings.TrimSpace(parsed.Reason)
	if reason == "" {
		reason = "No reason provided"
	}
	needsRefDocs := true
	if parsed.NeedsReferenceDocuments != nil {
		needsRefDocs = *parsed.NeedsReferenceDocuments
	}
	needsProfile := true
	if parsed.NeedsUserProfile != nil {
		needsProfile = *parsed.NeedsUserProfile
	}
	return AutoRouteDecision{
		Decision:                route,
		Reason:                  reason,
		Confidence:              parsed.Confidence,
		NeedsReferenceDocuments: needsRefDocs,
		NeedsUserProfile:        needsProfile,
	}, nil
}

func isValidHostedProviderName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "gemini", "claude", "deepseek":
		return true
	default:
		return false
	}
}

// hostedProviderTryOrder returns hosted provider names to try: last manual choice first, then defaults.
func hostedProviderTryOrder(lastManualHosted string) []string {
	last := strings.ToLower(strings.TrimSpace(lastManualHosted))
	seen := map[string]bool{}
	var order []string
	if isValidHostedProviderName(last) {
		order = append(order, last)
		seen[last] = true
	}
	for _, p := range []string{"gemini", "claude", "deepseek"} {
		if !seen[p] {
			order = append(order, p)
		}
	}
	return order
}

func (s *ChatService) pickHostedProviderForAuto(ctx context.Context, r *http.Request, lastManualHosted string) (string, appai.ChatProvider) {
	for _, name := range hostedProviderTryOrder(lastManualHosted) {
		var p appai.ChatProvider
		switch name {
		case "claude":
			p = s.effectiveClaudeProvider(ctx, r, "")
		case "deepseek":
			p = s.effectiveDeepSeekProvider(ctx, r, "")
		default:
			p = s.effectiveGeminiProvider(ctx, r, "")
			name = "gemini"
		}
		if p != nil && p.IsAvailable() {
			return name, p
		}
	}
	return "", nil
}

func hostedClassifierFallbackDecision(reason string, classifierError string) AutoRouteDecision {
	return AutoRouteDecision{
		Decision:                "hosted",
		Reason:                  reason,
		ClassifierFallback:      true,
		ClassifierError:         classifierError,
		NeedsReferenceDocuments: true,
		NeedsUserProfile:        true,
	}
}

func (s *ChatService) subjectProfileContextAvailable(ctx context.Context) bool {
	cfg, _ := s.subjectRepo.GetFirst(ctx)
	if cfg == nil {
		return false
	}
	if cfg.PsychologicalProfileAI != nil && strings.TrimSpace(*cfg.PsychologicalProfileAI) != "" {
		return true
	}
	if cfg.WritingStyleAI != nil && strings.TrimSpace(*cfg.WritingStyleAI) != "" {
		return true
	}
	return false
}

func autoExecutionContextFromDecision(decision AutoRouteDecision) AutoExecutionContext {
	return AutoExecutionContext{
		IncludeReferenceDocuments: decision.NeedsReferenceDocuments,
		IncludeUserProfile:      decision.NeedsUserProfile,
	}
}

func (s *ChatService) classifyAutoRoute(ctx context.Context, r *http.Request, prompt string, toolsCount, refDocCount int, hasSubjectProfile bool) AutoRouteDecision {
	logAttrs := []any{
		"user_id", appctx.UserIDFromCtx(ctx),
		"user_prompt", prompt,
		"tools_count", toolsCount,
		"ref_doc_count", refDocCount,
		"has_subject_profile", hasSubjectProfile,
		"num_ctx", autoClassifierNumCtx,
	}

	local := s.effectiveLocalAIProvider()
	lp, ok := local.(*appai.LocalAIProvider)
	if !ok || lp == nil || !lp.IsAvailable() {
		decision := hostedClassifierFallbackDecision("Local AI classifier unavailable", "")
		slog.Warn("auto classifier request skipped", append(logAttrs, "decision", decision.Decision, "reason", decision.Reason)...)
		return decision
	}

	classifierPrompt := buildAutoClassifierPrompt(prompt, toolsCount, refDocCount, hasSubjectProfile)
	slog.Info("auto classifier request", append(logAttrs, "classifier_prompt", classifierPrompt)...)

	raw, usage, err := lp.SimpleGenerateWithNumCtx(ctx, classifierPrompt, autoClassifierNumCtx)
	if usage != nil {
		s.applyUsageKeySourceToLLMUsage(ctx, r, "", usage)
		RecordLLMUsage(ctx, s.billing, s.userRepo, usage, err)
	}
	if err != nil {
		decision := hostedClassifierFallbackDecision("Classifier request failed", err.Error())
		respAttrs := append(logAttrs,
			"raw_response", raw,
			"err", err,
			"decision", decision.Decision,
			"reason", decision.Reason,
			"classifier_error", decision.ClassifierError,
			"classifier_fallback", decision.ClassifierFallback,
		)
		if usage != nil {
			respAttrs = append(respAttrs, "input_tokens", usage.InputTokens, "output_tokens", usage.OutputTokens, "model", usage.Model)
		}
		slog.Warn("auto classifier response", respAttrs...)
		return decision
	}

	decision, parseErr := parseAutoClassifierResponse(raw)
	if parseErr != nil {
		decision = hostedClassifierFallbackDecision("Could not parse classifier response", parseErr.Error())
		respAttrs := append(logAttrs,
			"raw_response", raw,
			"err", parseErr,
			"decision", decision.Decision,
			"reason", decision.Reason,
			"classifier_error", decision.ClassifierError,
			"classifier_fallback", decision.ClassifierFallback,
		)
		if usage != nil {
			respAttrs = append(respAttrs, "input_tokens", usage.InputTokens, "output_tokens", usage.OutputTokens, "model", usage.Model)
		}
		slog.Warn("auto classifier response", respAttrs...)
		return decision
	}

	respAttrs := append(logAttrs,
		"raw_response", raw,
		"decision", decision.Decision,
		"reason", decision.Reason,
		"confidence", decision.Confidence,
		"needs_reference_documents", decision.NeedsReferenceDocuments,
		"needs_user_profile", decision.NeedsUserProfile,
		"classifier_fallback", decision.ClassifierFallback,
	)
	if usage != nil {
		respAttrs = append(respAttrs, "input_tokens", usage.InputTokens, "output_tokens", usage.OutputTokens, "model", usage.Model)
	}
	slog.Info("auto classifier response", respAttrs...)
	return decision
}

func autoRouteMetaFromDecision(decision AutoRouteDecision, routedProvider string, executionFallback bool) map[string]any {
	meta := map[string]any{
		"requested_provider":        "auto",
		"classifier_provider":       "localai",
		"decision":                  decision.Decision,
		"routed_provider":           routedProvider,
		"reason":                    decision.Reason,
		"classifier_fallback":       decision.ClassifierFallback,
		"execution_fallback":        executionFallback,
		"needs_reference_documents": decision.NeedsReferenceDocuments,
		"needs_user_profile":        decision.NeedsUserProfile,
	}
	if decision.ClassifierError != "" {
		meta["classifier_error"] = decision.ClassifierError
	}
	if decision.Confidence > 0 {
		meta["confidence"] = decision.Confidence
	}
	return meta
}

// resolveAutoProvider classifies the prompt and returns the provider to execute the chat request.
func (s *ChatService) resolveAutoProvider(ctx context.Context, r *http.Request, prompt string, toolsCount, refDocCount int, hasSubjectProfile bool, lastManualHosted string) (appai.ChatProvider, string, map[string]any, AutoExecutionContext, error) {
	decision := s.classifyAutoRoute(ctx, r, prompt, toolsCount, refDocCount, hasSubjectProfile)
	execCtx := autoExecutionContextFromDecision(decision)

	var provider appai.ChatProvider
	var providerName string
	executionFallback := false

	if decision.Decision == "local" {
		provider = s.effectiveLocalAIProvider()
		providerName = "localai"
		if provider == nil || !provider.IsAvailable() {
			providerName, provider = s.pickHostedProviderForAuto(ctx, r, lastManualHosted)
			executionFallback = true
		}
	} else {
		providerName, provider = s.pickHostedProviderForAuto(ctx, r, lastManualHosted)
		if provider == nil || !provider.IsAvailable() {
			provider = s.effectiveLocalAIProvider()
			providerName = "localai"
			executionFallback = true
		}
	}

	if provider == nil || !provider.IsAvailable() {
		meta := autoRouteMetaFromDecision(decision, providerName, executionFallback)
		return nil, "", meta, execCtx, fmt.Errorf("auto routing: no provider available")
	}

	meta := autoRouteMetaFromDecision(decision, providerName, executionFallback)
	return provider, providerName, meta, execCtx, nil
}

// AutoAvailable reports whether the Auto provider can classify and execute requests.
func (s *ChatService) AutoAvailable(ctx context.Context, r *http.Request) bool {
	if !s.LocalAIAvailable() {
		return false
	}
	return s.LocalAIAvailable() ||
		s.GeminiAvailable(ctx, r) ||
		s.ClaudeAvailable(ctx, r) ||
		s.DeepSeekAvailable(ctx, r)
}
