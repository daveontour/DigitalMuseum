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

// autoClassifierNumCtx is the preferred Ollama context window for Auto routing classification.
// Some model tags (e.g. gemma4:e2b on Windows) crash Ollama when num_ctx is set too low;
// SimpleGenerateClassifier retries without num_ctx on failure.
const autoClassifierNumCtx = 4096

var autoClassifierFormatSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"route":                     map[string]any{"type": "string", "enum": []any{"local", "hosted"}},
		"reason":                    map[string]any{"type": "string"},
		"confidence":                map[string]any{"type": "number"},
		"needs_reference_documents": map[string]any{"type": "boolean"},
		"needs_user_profile":        map[string]any{"type": "boolean"},
	},
	"required": []any{"route", "reason"},
}

// AutoRouteDecision is the parsed output of the Auto routing classifier.
type AutoRouteDecision struct {
	Decision                  string // "local" or "hosted"
	Reason                    string
	Confidence                float64
	ClassifierFallback        bool
	ClassifierError           string
	ClassifierProvider        string // provider that ran classification (localai, gemini, claude, deepseek)
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

func extractJSONObject(raw string) string {
	raw = strings.TrimSpace(raw)
	start := strings.Index(raw, "{")
	if start == -1 {
		return raw
	}
	depth := 0
	for i := start; i < len(raw); i++ {
		switch raw[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return raw[start : i+1]
			}
		}
	}
	return raw[start:]
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
	raw = extractJSONObject(raw)
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
	case "gemini", "claude", "deepseek", "openai":
		return true
	default:
		return false
	}
}

// hostedProviderTryOrder returns hosted provider names to try: last manual choice first, then defaults.
func hostedProviderTryOrder(lastManualHosted string, configuredOrder []string) []string {
	return HostedProviderTryOrder(lastManualHosted, configuredOrder)
}

func (s *ChatService) pickHostedProviderForAuto(ctx context.Context, r *http.Request, lastManualHosted string, autoOrder []string) (string, appai.ChatProvider) {
	for _, name := range hostedProviderTryOrder(lastManualHosted, autoOrder) {
		var p appai.ChatProvider
		switch name {
		case "claude":
			p = s.effectiveClaudeProvider(ctx, r, "")
		case "deepseek":
			p = s.effectiveDeepSeekProvider(ctx, r, "")
		case "openai":
			p = s.effectiveOpenAIProvider(ctx, r, "")
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

func (s *ChatService) effectiveProviderByName(ctx context.Context, r *http.Request, name string) (appai.ChatProvider, string) {
	switch normalizeClassifierProvider(name) {
	case "claude":
		return s.effectiveClaudeProvider(ctx, r, ""), "claude"
	case "deepseek":
		return s.effectiveDeepSeekProvider(ctx, r, ""), "deepseek"
	case "openai":
		return s.effectiveOpenAIProvider(ctx, r, ""), "openai"
	case "localai":
		return s.localAIProviderForChat(ctx), "localai"
	default:
		return s.effectiveGeminiProvider(ctx, r, ""), "gemini"
	}
}

type classifierSimpleGen interface {
	SimpleGenerate(context.Context, string) (string, *appai.LLMUsage, error)
}

func (s *ChatService) runClassifierGenerate(ctx context.Context, providerName string, provider appai.ChatProvider, prompt string) (string, *appai.LLMUsage, error) {
	if provider == nil || !provider.IsAvailable() {
		return "", nil, fmt.Errorf("classifier provider %q unavailable", providerName)
	}
	switch providerName {
	case "localai":
		lp, ok := provider.(*appai.LocalAIProvider)
		if !ok || lp == nil {
			return "", nil, fmt.Errorf("classifier provider %q unavailable", providerName)
		}
		return lp.SimpleGenerateClassifier(ctx, prompt, autoClassifierNumCtx, autoClassifierFormatSchema)
	default:
		sg, ok := provider.(classifierSimpleGen)
		if !ok {
			return "", nil, fmt.Errorf("classifier provider %q does not support simple generate", providerName)
		}
		return sg.SimpleGenerate(ctx, prompt)
	}
}

func (s *ChatService) EffectiveClassifierProvider(ctx context.Context, r *http.Request, configured string) string {
	name := normalizeClassifierProvider(configured)
	if name != DefaultClassifierProvider {
		return name
	}
	if s.LocalAIAvailable(ctx) {
		return DefaultClassifierProvider
	}
	return s.firstAvailableHostedClassifierProvider(ctx, r)
}

func (s *ChatService) firstAvailableHostedClassifierProvider(ctx context.Context, r *http.Request) string {
	autoOrder, _ := s.loadHostedLLMProviderOrder(ctx)
	for _, p := range autoOrder {
		prov, pname := s.effectiveProviderByName(ctx, r, p)
		if prov != nil && prov.IsAvailable() {
			return pname
		}
	}
	for _, p := range DefaultHostedLLMProviderOrder {
		prov, pname := s.effectiveProviderByName(ctx, r, p)
		if prov != nil && prov.IsAvailable() {
			return pname
		}
	}
	return "gemini"
}

func (s *ChatService) hostedClassifierFallbackCandidates(ctx context.Context, r *http.Request, exclude string) []string {
	exclude = strings.ToLower(strings.TrimSpace(exclude))
	var out []string
	if s.LocalAIAvailable(ctx) && exclude != DefaultClassifierProvider {
		out = append(out, DefaultClassifierProvider)
	}
	autoOrder, _ := s.loadHostedLLMProviderOrder(ctx)
	seen := map[string]bool{exclude: true}
	for _, p := range autoOrder {
		if seen[p] {
			continue
		}
		seen[p] = true
		prov, _ := s.effectiveProviderByName(ctx, r, p)
		if prov != nil && prov.IsAvailable() {
			out = append(out, p)
		}
	}
	return out
}

func (s *ChatService) classifyAutoRoute(ctx context.Context, r *http.Request, prompt string, toolsCount, refDocCount int, hasSubjectProfile bool) AutoRouteDecision {
	orderCfg := s.loadHostedLLMProviderOrderConfig(ctx)
	configuredClassifier := s.EffectiveClassifierProvider(ctx, r, orderCfg.ClassifierProvider)

	logAttrs := []any{
		"user_id", appctx.UserIDFromCtx(ctx),
		"user_prompt", prompt,
		"tools_count", toolsCount,
		"ref_doc_count", refDocCount,
		"has_subject_profile", hasSubjectProfile,
		"num_ctx", autoClassifierNumCtx,
		"configured_classifier_provider", configuredClassifier,
	}

	classifierPrompt := buildAutoClassifierPrompt(prompt, toolsCount, refDocCount, hasSubjectProfile)
	slog.Info("auto classifier request", append(logAttrs, "classifier_prompt", classifierPrompt)...)

	provider, providerName := s.effectiveProviderByName(ctx, r, configuredClassifier)
	classifierProviderUsed := providerName
	raw, usage, err := s.runClassifierGenerate(ctx, providerName, provider, classifierPrompt)

	if err != nil {
		for _, alt := range s.hostedClassifierFallbackCandidates(ctx, r, providerName) {
			altProv, altName := s.effectiveProviderByName(ctx, r, alt)
			raw2, usage2, err2 := s.runClassifierGenerate(ctx, altName, altProv, classifierPrompt)
			if err2 != nil {
				continue
			}
			if usage != nil && usage2 != nil {
				usage.InputTokens += usage2.InputTokens
				usage.OutputTokens += usage2.OutputTokens
			} else if usage2 != nil {
				usage = usage2
			}
			raw, err = raw2, nil
			classifierProviderUsed = altName
			break
		}
	}

	if usage != nil {
		s.applyUsageKeySourceToLLMUsage(ctx, r, "", usage)
		RecordLLMUsage(ctx, s.billing, s.userRepo, usage, err)
	}

	logAttrs = append(logAttrs, "classifier_provider", classifierProviderUsed)

	if err != nil {
		decision := hostedClassifierFallbackDecision("Classifier request failed", err.Error())
		decision.ClassifierProvider = classifierProviderUsed
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
	decision.ClassifierProvider = classifierProviderUsed
	if parseErr != nil {
		decision = hostedClassifierFallbackDecision("Could not parse classifier response", parseErr.Error())
		decision.ClassifierProvider = classifierProviderUsed
		slog.Warn("auto classifier response", append(logAttrs,
			"raw_response", raw,
			"err", parseErr,
			"decision", decision.Decision,
			"reason", decision.Reason,
			"classifier_error", decision.ClassifierError,
			"classifier_fallback", decision.ClassifierFallback,
		)...)
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
	classifierProvider := strings.TrimSpace(decision.ClassifierProvider)
	if classifierProvider == "" {
		classifierProvider = DefaultClassifierProvider
	}
	meta := map[string]any{
		"requested_provider":        "auto",
		"classifier_provider":       classifierProvider,
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
	autoOrder, _ := s.loadHostedLLMProviderOrder(ctx)

	var provider appai.ChatProvider
	var providerName string
	executionFallback := false

	if decision.Decision == "local" {
		provider = s.localAIProviderForChat(ctx)
		providerName = "localai"
		if provider == nil || !provider.IsAvailable() {
			providerName, provider = s.pickHostedProviderForAuto(ctx, r, lastManualHosted, autoOrder)
			executionFallback = true
		}
	} else {
		providerName, provider = s.pickHostedProviderForAuto(ctx, r, lastManualHosted, autoOrder)
		if provider == nil || !provider.IsAvailable() {
			provider = s.localAIProviderForChat(ctx)
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
	cfg := s.loadHostedLLMProviderOrderConfig(ctx)
	if !s.classifierAvailable(ctx, r, cfg.ClassifierProvider) {
		return false
	}
	return s.LocalAIAvailable(ctx) ||
		s.GeminiAvailable(ctx, r) ||
		s.ClaudeAvailable(ctx, r) ||
		s.DeepSeekAvailable(ctx, r) ||
		s.OpenAIAvailable(ctx, r)
}

func (s *ChatService) classifierAvailable(ctx context.Context, r *http.Request, configured string) bool {
	effective := s.EffectiveClassifierProvider(ctx, r, configured)
	p, _ := s.effectiveProviderByName(ctx, r, effective)
	if p != nil && p.IsAvailable() {
		return true
	}
	for _, alt := range s.hostedClassifierFallbackCandidates(ctx, r, effective) {
		altProv, _ := s.effectiveProviderByName(ctx, r, alt)
		if altProv != nil && altProv.IsAvailable() {
			return true
		}
	}
	return false
}
