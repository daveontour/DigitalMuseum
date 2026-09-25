package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	appai "github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/appctx"
)

const (
	jevClassifierProvider = "jev"
	// jevNoulYesThreshold is the probability above which a Jev yes/no answer is treated as yes.
	jevNoulYesThreshold = 0.5
	jevCriterionDescMax = 400
)

// AutoRouteDecision is the parsed output of the Auto routing classifier.
type AutoRouteDecision struct {
	Decision                string // enabled model key, or "hosted" when classification fell back
	Reason                  string
	Confidence              float64
	ClassifierFallback      bool
	ClassifierError         string
	ClassifierProvider      string // always "jev" when Jev ran or was attempted
	NeedsReferenceDocuments bool
	NeedsUserProfile        bool
	ClassifierRequestJSON   string
	ClassifierResponseJSON  string
	ClassifierDurationMS    int64
	ClassifierTimed         bool
}

// AutoExecutionContext controls optional user context included in the follow-up chat request.
type AutoExecutionContext struct {
	IncludeReferenceDocuments bool
	IncludeUserProfile        bool
}

type jevChoiceAnswer struct {
	Choice     string  `json:"choice"`
	Confidence float64 `json:"confidence"`
}

type jevNoulAnswer struct {
	Noul *float64 `json:"noul"`
}

// buildJevModelCriteria maps each enabled model key to the text Jev uses to compare options.
// Catalog fields (description, context length, price) are included when the slug matches.
func buildJevModelCriteria(models []AIModel, catalog []appai.OpenRouterCatalogModel) map[string]string {
	bySlug := indexCatalogBySlug(catalog)
	out := make(map[string]string, len(models))
	for _, m := range models {
		key := strings.ToLower(strings.TrimSpace(m.Key))
		if key == "" {
			continue
		}
		if key == localAIModelKey {
			out[key] = "Local AI (Ollama). Runs on this computer with no per-token API cost. Best for a short factual answer such as the current time or count. Very weak if the response requires accessing reference documents, user profiles, or multi-step reasoning or persona-heavy writing."
			continue
		}
		out[key] = hostedModelCriterion(m, bySlug)
	}
	return out
}

func indexCatalogBySlug(catalog []appai.OpenRouterCatalogModel) map[string]appai.OpenRouterCatalogModel {
	out := make(map[string]appai.OpenRouterCatalogModel, len(catalog)*2)
	for _, m := range catalog {
		if id := strings.ToLower(strings.TrimSpace(m.ID)); id != "" {
			out[id] = m
		}
		if slug := strings.ToLower(strings.TrimSpace(m.CanonicalSlug)); slug != "" {
			out[slug] = m
		}
	}
	return out
}

func hostedModelCriterion(m AIModel, bySlug map[string]appai.OpenRouterCatalogModel) string {
	name := strings.TrimSpace(m.DisplayName)
	if name == "" {
		name = m.Key
	}
	slug := strings.TrimSpace(m.ModelSlug)
	parts := []string{name}
	if slug != "" {
		parts = append(parts, "OpenRouter slug: "+slug)
	}
	if cat, ok := bySlug[strings.ToLower(slug)]; ok {
		if desc := truncateRunes(strings.TrimSpace(cat.Description), jevCriterionDescMax); desc != "" {
			parts = append(parts, desc)
		}
		if cat.ContextLength > 0 {
			parts = append(parts, fmt.Sprintf("Context length: %d tokens", cat.ContextLength))
		}
		if price := formatPerMillion(cat.Pricing.Prompt); price != "" {
			parts = append(parts, "Prompt price: "+price)
		}
		if price := formatPerMillion(cat.Pricing.Completion); price != "" {
			parts = append(parts, "Completion price: "+price)
		}
	}
	return strings.Join(parts, ". ")
}

func formatPerMillion(perToken string) string {
	f, err := strconv.ParseFloat(strings.TrimSpace(perToken), 64)
	if err != nil || f <= 0 {
		return ""
	}
	return "$" + strconv.FormatFloat(f*1_000_000, 'f', -1, 64) + " per 1M tokens"
}

func truncateRunes(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

func buildJevQuestions(criteria map[string]string) map[string]any {
	return map[string]any{
		"model": map[string]any{
			"type":         "choice",
			"instructions": "Which model should answer the request in `user_request`? Prefer the cheapest option that can do the job. Choose localai for a simple lookup, count, time, or short factual answer. Choose a hosted model when the request needs multi-step reasoning, synthesis across records, summaries, stories, or persona-heavy writing.",
			"criteria":     criteria,
		},
		"needs_reference_documents": map[string]any{
			"type":         "noul",
			"instructions": "Does answering `user_request` require material from the user's inlined reference documents (identity notes, background docs)?",
			"criteria": map[string]string{
				"true":  "The answer depends on those documents.",
				"false": "The request is generic, factual, or answerable with tools alone.",
			},
		},
		"needs_user_profile": map[string]any{
			"type":         "noul",
			"instructions": "Does answering `user_request` require the subject's psychological or writing-style profile, or what data the archive holds?",
			"criteria": map[string]string{
				"true":  "The answer depends on who the archive subject is or what data they have.",
				"false": "The request does not depend on the subject's identity or archive inventory.",
			},
		},
	}
}

func jevState(prompt string, toolsCount, refDocCount int, hasSubjectProfile bool) map[string]any {
	return map[string]any{
		"user_request":              prompt,
		"tools_available":           toolsCount,
		"reference_document_count":  refDocCount,
		"subject_profile_available": hasSubjectProfile,
	}
}

func prettyJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

func prettyRawJSON(raw []byte) string {
	if len(bytesTrim(raw)) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	return prettyJSON(v)
}

func bytesTrim(raw []byte) []byte {
	return []byte(strings.TrimSpace(string(raw)))
}

// parseJevDecision reads a Decisions API body and returns the chosen model key.
// Missing yes/no answers default to including that context.
func parseJevDecision(body []byte, allowed map[string]struct{}) (AutoRouteDecision, error) {
	var parsed appai.JevDecisionResult
	if err := json.Unmarshal(body, &parsed); err != nil {
		return AutoRouteDecision{}, fmt.Errorf("invalid classifier JSON: %w", err)
	}
	rawChoice := parsed.Answers["model"]
	if len(rawChoice) == 0 {
		return AutoRouteDecision{}, fmt.Errorf("missing model choice")
	}
	var choice jevChoiceAnswer
	if err := json.Unmarshal(rawChoice, &choice); err != nil {
		return AutoRouteDecision{}, fmt.Errorf("invalid model choice: %w", err)
	}
	key := strings.ToLower(strings.TrimSpace(choice.Choice))
	if _, ok := allowed[key]; !ok {
		return AutoRouteDecision{}, fmt.Errorf("invalid route %q", choice.Choice)
	}
	return AutoRouteDecision{
		Decision:                key,
		Reason:                  "Jev selected " + key,
		Confidence:              choice.Confidence,
		ClassifierProvider:      jevClassifierProvider,
		NeedsReferenceDocuments: jevNoulYes(parsed.Answers["needs_reference_documents"]),
		NeedsUserProfile:        jevNoulYes(parsed.Answers["needs_user_profile"]),
	}, nil
}

func jevNoulYes(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	var n jevNoulAnswer
	if err := json.Unmarshal(raw, &n); err != nil || n.Noul == nil {
		return true
	}
	return *n.Noul > jevNoulYesThreshold
}

func (s *ChatService) pickHostedProviderForAuto(ctx context.Context, r *http.Request, lastManualHosted string) (string, appai.ChatProvider) {
	for _, name := range s.HostedProviderTryOrder(ctx, lastManualHosted) {
		p := s.effectiveProviderByKey(ctx, r, "", name)
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
		ClassifierProvider:      jevClassifierProvider,
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
		IncludeUserProfile:        decision.NeedsUserProfile,
	}
}

func (s *ChatService) effectiveProviderByName(ctx context.Context, r *http.Request, name string) (appai.ChatProvider, string) {
	key := s.normalizeClassifierProvider(ctx, name)
	if key == "localai" {
		return s.localAIProviderForChat(ctx), "localai"
	}
	return s.effectiveProviderByKey(ctx, r, "", key), key
}

func (s *ChatService) classifyAutoRoute(ctx context.Context, r *http.Request, prompt string, toolsCount, refDocCount int, hasSubjectProfile bool) AutoRouteDecision {
	var models []AIModel
	if s.aiModelsSvc != nil {
		listed, err := s.aiModelsSvc.ListEnabledInTableOrder(ctx)
		if err != nil {
			slog.Warn("auto classifier: list models", "err", err)
		} else {
			models = listed
		}
	}
	var catalog []appai.OpenRouterCatalogModel
	if s.openRouterCatalog != nil {
		listed, err := s.openRouterCatalog.List(ctx)
		if err != nil {
			slog.Warn("auto classifier: catalog unavailable", "err", err)
		} else {
			catalog = listed
		}
	}
	criteria := buildJevModelCriteria(models, catalog)
	logAttrs := []any{
		"user_id", appctx.UserIDFromCtx(ctx),
		"user_prompt", prompt,
		"tools_count", toolsCount,
		"ref_doc_count", refDocCount,
		"has_subject_profile", hasSubjectProfile,
		"classifier_provider", jevClassifierProvider,
		"model_options", len(criteria),
	}
	if len(criteria) == 0 {
		decision := hostedClassifierFallbackDecision("No enabled models to choose from", "empty model list")
		slog.Warn("auto classifier response", append(logAttrs, "decision", decision.Decision, "classifier_fallback", true, "err", decision.ClassifierError)...)
		return decision
	}

	apiKey := s.effectiveOpenRouterKey(ctx, r, "")
	state := jevState(prompt, toolsCount, refDocCount, hasSubjectProfile)
	questions := buildJevQuestions(criteria)
	reqJSON := prettyJSON(map[string]any{
		"model":     appai.JevModelID,
		"state":     state,
		"questions": questions,
	})
	slog.Info("auto classifier request", append(logAttrs, "jev_model", appai.JevModelID)...)

	started := time.Now()
	result, rawResp, err := appai.CallJevDecisions(ctx, apiKey, state, questions)
	elapsedMS := time.Since(started).Milliseconds()
	respJSON := prettyRawJSON(rawResp)
	stamp := func(d AutoRouteDecision) AutoRouteDecision {
		d.ClassifierRequestJSON = reqJSON
		d.ClassifierResponseJSON = respJSON
		d.ClassifierDurationMS = elapsedMS
		d.ClassifierTimed = true
		return d
	}
	var usage *appai.LLMUsage
	if result != nil && (result.Usage.InputTokens > 0 || result.Usage.OutputTokens > 0) {
		model := result.Model
		if model == "" {
			model = appai.JevModelID
		}
		usage = &appai.LLMUsage{
			Provider:     jevClassifierProvider,
			Model:        model,
			InputTokens:  result.Usage.InputTokens,
			OutputTokens: result.Usage.OutputTokens,
		}
		s.applyUsageKeySourceToLLMUsage(ctx, r, "", usage)
		RecordLLMUsage(ctx, s.billing, s.userRepo, usage, err)
	}
	if err != nil {
		decision := stamp(hostedClassifierFallbackDecision("Classifier request failed", err.Error()))
		slog.Warn("auto classifier response", append(logAttrs,
			"err", err,
			"decision", decision.Decision,
			"reason", decision.Reason,
			"classifier_error", decision.ClassifierError,
			"classifier_fallback", true,
			"classifier_duration_ms", elapsedMS,
		)...)
		return decision
	}

	allowed := make(map[string]struct{}, len(criteria))
	for key := range criteria {
		allowed[key] = struct{}{}
	}
	raw, _ := json.Marshal(result)
	decision, parseErr := parseJevDecision(raw, allowed)
	if parseErr != nil {
		decision = stamp(hostedClassifierFallbackDecision("Could not parse classifier response", parseErr.Error()))
		slog.Warn("auto classifier response", append(logAttrs,
			"err", parseErr,
			"decision", decision.Decision,
			"reason", decision.Reason,
			"classifier_error", decision.ClassifierError,
			"classifier_fallback", true,
		)...)
		return decision
	}
	slog.Info("auto classifier response", append(logAttrs,
		"decision", decision.Decision,
		"reason", decision.Reason,
		"confidence", decision.Confidence,
		"needs_reference_documents", decision.NeedsReferenceDocuments,
		"needs_user_profile", decision.NeedsUserProfile,
		"classifier_fallback", false,
		"classifier_duration_ms", elapsedMS,
	)...)
	return stamp(decision)
}

func autoRouteMetaFromDecision(decision AutoRouteDecision, routedProvider string, executionFallback bool) map[string]any {
	classifierProvider := strings.TrimSpace(decision.ClassifierProvider)
	if classifierProvider == "" {
		classifierProvider = jevClassifierProvider
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
	if decision.ClassifierRequestJSON != "" {
		meta["classifier_request_json"] = decision.ClassifierRequestJSON
	}
	if decision.ClassifierResponseJSON != "" {
		meta["classifier_response_json"] = decision.ClassifierResponseJSON
	}
	if decision.ClassifierTimed {
		meta["classifier_duration_ms"] = decision.ClassifierDurationMS
	}
	return meta
}

// resolveAutoProvider classifies the prompt and returns the provider to execute the chat request.
// When Auto selection mode is disabled, skips the query classifier and picks the first available
// model in AI Models table order (with last-manual-hosted preference).
func (s *ChatService) resolveAutoProvider(ctx context.Context, r *http.Request, prompt string, toolsCount, refDocCount int, hasSubjectProfile bool, lastManualHosted string) (appai.ChatProvider, string, map[string]any, AutoExecutionContext, error) {
	orderCfg := s.loadHostedLLMProviderOrderConfig(ctx)
	if !orderCfg.AutoSelectionEnabled {
		providerName, provider := s.pickHostedProviderForAuto(ctx, r, lastManualHosted)
		executionFallback := false
		if provider == nil || !provider.IsAvailable() {
			provider = s.localAIProviderForChat(ctx)
			providerName = "localai"
			executionFallback = true
		}
		if provider == nil || !provider.IsAvailable() {
			meta := map[string]any{
				"requested_provider":        "auto",
				"auto_selection_enabled":    false,
				"routed_provider":           providerName,
				"classifier_skipped":        true,
				"execution_fallback":        executionFallback,
				"needs_reference_documents": true,
				"needs_user_profile":        true,
			}
			return nil, "", meta, AutoExecutionContext{IncludeReferenceDocuments: true, IncludeUserProfile: true}, fmt.Errorf("auto routing: no provider available")
		}
		meta := map[string]any{
			"requested_provider":        "auto",
			"auto_selection_enabled":    false,
			"routed_provider":           providerName,
			"classifier_skipped":        true,
			"execution_fallback":        executionFallback,
			"needs_reference_documents": true,
			"needs_user_profile":        true,
		}
		return provider, providerName, meta, AutoExecutionContext{IncludeReferenceDocuments: true, IncludeUserProfile: true}, nil
	}

	decision := s.classifyAutoRoute(ctx, r, prompt, toolsCount, refDocCount, hasSubjectProfile)
	execCtx := autoExecutionContextFromDecision(decision)

	var provider appai.ChatProvider
	var providerName string
	executionFallback := false

	if decision.ClassifierFallback || decision.Decision == "" || decision.Decision == "hosted" {
		providerName, provider = s.pickHostedProviderForAuto(ctx, r, lastManualHosted)
		if provider == nil || !provider.IsAvailable() {
			provider = s.localAIProviderForChat(ctx)
			providerName = "localai"
			executionFallback = true
		}
	} else {
		provider, providerName = s.effectiveProviderByName(ctx, r, decision.Decision)
		if provider == nil || !provider.IsAvailable() {
			executionFallback = true
			providerName, provider = s.pickHostedProviderForAuto(ctx, r, lastManualHosted)
			if provider == nil || !provider.IsAvailable() {
				provider = s.localAIProviderForChat(ctx)
				providerName = "localai"
			}
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
	if !cfg.AutoSelectionEnabled {
		return false
	}
	if !s.classifierAvailable(ctx, r, cfg.ClassifierProvider) {
		return false
	}
	if s.LocalAIAvailable(ctx) {
		return true
	}
	if s.aiModelsSvc == nil {
		return false
	}
	models, _ := s.aiModelsSvc.ListEnabled(ctx)
	for _, m := range models {
		if s.ModelAvailable(ctx, r, m.Key) {
			return true
		}
	}
	return false
}

func (s *ChatService) classifierAvailable(ctx context.Context, r *http.Request, configured string) bool {
	_ = configured
	return strings.TrimSpace(s.effectiveOpenRouterKey(ctx, r, "")) != ""
}
