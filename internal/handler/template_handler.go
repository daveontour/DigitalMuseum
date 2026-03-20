package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/daveontour/digitalmuseum/internal/config"
	"github.com/daveontour/digitalmuseum/internal/keystore"
	"github.com/daveontour/digitalmuseum/internal/repository"
	"github.com/go-chi/chi/v5"
)

// TemplateHandler serves the templated endpoints:
//   - GET /                              → index.template.html (or non_user_init)
//   - GET /api/suggestions               → suggestions.json rendered with subject vars
//   - GET /static/js/museum/foundation.js
//   - GET /static/js/museum/modals-people.js
type TemplateHandler struct {
	subjectRepo     *repository.SubjectConfigRepo
	templatesDir    string
	pythonStaticDir string
	geminiAvail     bool
	claudeAvail     bool
	pageTitle       string
	sessionStore    *keystore.SessionMasterStore
}

// NewTemplateHandler creates a TemplateHandler.
// sessionStore is cleared for this client on every GET / so each full HTML load requires unlocking the master key again.
func NewTemplateHandler(subjectRepo *repository.SubjectConfigRepo, cfg *config.Config, sessionStore *keystore.SessionMasterStore) *TemplateHandler {
	return &TemplateHandler{
		subjectRepo:     subjectRepo,
		templatesDir:    cfg.App.TemplatesDir,
		pythonStaticDir: cfg.App.AssetStaticDir,
		geminiAvail:     cfg.AI.GeminiAPIKey != "",
		claudeAvail:     cfg.AI.AnthropicAPIKey != "",
		pageTitle:       cfg.App.PageTitle,
		sessionStore:    sessionStore,
	}
}

// RegisterRoutes mounts the templated routes.
// The specific JS paths are registered before /static/* so chi matches them first.
func (h *TemplateHandler) RegisterRoutes(r chi.Router) {
	r.Get("/", h.GetRoot)
	r.Get("/api/suggestions", h.GetSuggestions)
	r.Get("/static/js/museum/foundation.js", h.GetFoundationJS)
	r.Get("/static/js/museum/modals-people.js", h.GetModalsPeopleJS)
}

// GetRoot handles GET / → renders index.template.html (or non_user_init.template.html).
func (h *TemplateHandler) GetRoot(w http.ResponseWriter, r *http.Request) {
	h.sessionStore.Clear(w, r)

	ctx := h.buildContext(r)

	cfg, err := h.subjectRepo.GetFirst(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error loading subject configuration: %s", err))
		return
	}

	// Choose template: non_user_init when subject_configuration exists but both
	// subject_name and family_name are empty strings (matches Python logic).
	templateName := "index.template.html"
	if cfg == nil || (cfg != nil && cfg.SubjectName == "" && (cfg.FamilyName == nil || *cfg.FamilyName == "")) {
		templateName = "non_user_init.template.html"
	}

	// Derive page title (substituting SUBJECT_NAME placeholder if present).
	pageTitle := strings.ReplaceAll(h.pageTitle, "SUBJECT_NAME", ctx["owner"])

	content, err := h.readFile(h.templatesDir, templateName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error reading template: %s", err))
		return
	}

	extras := map[string]string{
		"page_title": pageTitle,
	}
	rendered := renderJinja(content, ctx, extras)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(rendered))
}

// GetSuggestions handles GET /api/suggestions.
func (h *TemplateHandler) GetSuggestions(w http.ResponseWriter, r *http.Request) {
	ctx := h.buildContext(r)

	content, err := h.readFile(h.pythonStaticDir, filepath.Join("data", "suggestions.json"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error reading suggestions: %s", err))
		return
	}

	rendered := renderJinja(content, ctx, nil)

	// Validate it's still valid JSON after substitution.
	var parsed any
	if err := json.Unmarshal([]byte(rendered), &parsed); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("suggestions template produced invalid JSON: %s", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	_, _ = w.Write([]byte(rendered))
}

// GetFoundationJS handles GET /static/js/museum/foundation.js.
func (h *TemplateHandler) GetFoundationJS(w http.ResponseWriter, r *http.Request) {
	ctx := h.buildContext(r)
	if h.geminiAvail {
		ctx["gemini_configured"] = "True"
	} else {
		ctx["gemini_configured"] = "False"
	}
	if h.claudeAvail {
		ctx["claude_configured"] = "True"
	} else {
		ctx["claude_configured"] = "False"
	}

	content, err := h.readFile(h.pythonStaticDir, filepath.Join("js", "museum", "foundation.js"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error reading foundation.js: %s", err))
		return
	}

	rendered := renderJinja(content, ctx, nil)
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	_, _ = w.Write([]byte(rendered))
}

// GetModalsPeopleJS handles GET /static/js/museum/modals-people.js.
func (h *TemplateHandler) GetModalsPeopleJS(w http.ResponseWriter, r *http.Request) {
	ctx := h.buildContext(r)

	content, err := h.readFile(h.pythonStaticDir, filepath.Join("js", "museum", "modals-people.js"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error reading modals-people.js: %s", err))
		return
	}

	rendered := renderJinja(content, ctx, nil)
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	_, _ = w.Write([]byte(rendered))
}

// ── helpers ───────────────────────────────────────────────────────────────────

// buildContext fetches the subject config and builds the Jinja2 variable map.
// On error or missing config it falls back to safe defaults matching Python behaviour.
func (h *TemplateHandler) buildContext(r *http.Request) map[string]string {
	cfg, _ := h.subjectRepo.GetFirst(r.Context())

	var (
		subjectName = "<Error>"
		gender      = "Male"
		familyName  = ""
	)
	if cfg != nil {
		subjectName = cfg.SubjectName
		gender = cfg.Gender
		if cfg.FamilyName != nil {
			familyName = *cfg.FamilyName
		}
	}

	var (
		him            = "him"
		his            = "his"
		he             = "he"
		himself        = "himself"
		ownerImage     = "male.png"
		ownerImageSm   = "male_sm.png"
		admirerImage   = "female.png"
		admirerImageSm = "female_sm.png"
	)
	if gender != "Male" {
		him = "her"
		his = "her"
		he = "she"
		himself = "herself"
		ownerImage = "female.png"
		ownerImageSm = "female_sm.png"
		admirerImage = "male.png"
		admirerImageSm = "male_sm.png"
	}

	fullName := strings.TrimSpace(subjectName + " " + familyName)

	return map[string]string{
		"owner":               subjectName,
		"owners":              subjectName + "'s",
		"full_name":           fullName,
		"his":                 his,
		"he":                  he,
		"him":                 him,
		"himself":             himself,
		"owner_image":         ownerImage,
		"owner_image_small":   ownerImageSm,
		"admirer_image":       admirerImage,
		"admirer_image_small": admirerImageSm,
		// Undefined in Python context too — render as empty string.
		"owner_gender":        gender,
		"todays_thing_prompt": "",
	}
}

// readFile reads a file from baseDir/relPath.
func (h *TemplateHandler) readFile(baseDir, relPath string) (string, error) {
	data, err := os.ReadFile(filepath.Join(baseDir, relPath))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// forLoopRe matches Jinja2 {% for ... %} ... {% endfor %} blocks (including nested).
// The (?s) flag makes . match newlines.
var forLoopRe = regexp.MustCompile(`(?s)\{%-?\s*for\s[^%]*?-?%\}.*?\{%-?\s*endfor\s*-?%\}`)

// blockTagRe matches any remaining {% ... %} Jinja2 control-flow tags.
var blockTagRe = regexp.MustCompile(`\{%-?[^}]*?-?%\}`)

// renderJinja performs Jinja2-style variable substitution on content.
// It strips {% for %}...{% endfor %} blocks, strips remaining {% %} tags,
// then replaces {{ varname }} with values from ctx and extras.
func renderJinja(content string, ctx map[string]string, extras map[string]string) string {
	// Remove for-loop blocks (they reference variables not in our context).
	content = forLoopRe.ReplaceAllString(content, "")
	// Remove remaining block tags ({% if %}, {% set %}, etc.).
	content = blockTagRe.ReplaceAllString(content, "")

	// Build replacer pairs: handle both {{var}} and {{ var }} forms.
	pairs := make([]string, 0, (len(ctx)+len(extras))*4)
	addVar := func(k, v string) {
		pairs = append(pairs, "{{"+k+"}}", v)
		pairs = append(pairs, "{{ "+k+" }}", v)
		pairs = append(pairs, "{{"+k+" }}", v)
		pairs = append(pairs, "{{ "+k+"}}", v)
	}
	for k, v := range ctx {
		addVar(k, v)
	}
	for k, v := range extras {
		addVar(k, v)
	}

	return strings.NewReplacer(pairs...).Replace(content)
}
