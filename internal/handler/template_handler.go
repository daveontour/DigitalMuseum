package handler

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/georegion"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/service"
	"github.com/go-chi/chi/v5"
)

// TemplateHandler serves the templated endpoints:
//   - GET /                              → index.template.html (or non_user_init)
//   - GET /favicon.ico                   → templates/favicon.ico
//   - GET /api/suggestions               → suggestions from DB, rendered with subject vars
//   - GET /api/regions                   → in-memory registry (loaded from DB at startup)
//   - GET /static/js/museum/foundation.js
//   - GET /static/js/museum/modals-people.js
type TemplateHandler struct {
	subjectRepo                        *repository.SubjectConfigRepo
	userRepo                           *repository.UserRepo
	suggestionsSvc                     *service.SuggestionsService
	templatesDir                       string
	pythonStaticDir                    string
	defaultGeminiOK                    bool
	defaultClaudeOK                    bool
	defaultDeepSeekOK                  bool
	defaultOpenAIOK                    bool
	defaultLocalAIOK                   bool
	pageTitle                          string
	deploymentNatureLocal              bool
	guideTopicsReloadFromFileOnStartup bool
}

// NewTemplateHandler creates a TemplateHandler.
func NewTemplateHandler(subjectRepo *repository.SubjectConfigRepo, userRepo *repository.UserRepo, suggestionsSvc *service.SuggestionsService, cfg *config.Config) *TemplateHandler {
	return &TemplateHandler{
		subjectRepo:                        subjectRepo,
		userRepo:                           userRepo,
		suggestionsSvc:                     suggestionsSvc,
		templatesDir:                       cfg.App.TemplatesDir,
		pythonStaticDir:                    cfg.App.AssetStaticDir,
		defaultGeminiOK:                    cfg.AI.GeminiAPIKey != "",
		defaultClaudeOK:                    cfg.AI.AnthropicAPIKey != "",
		defaultDeepSeekOK:                  strings.TrimSpace(cfg.AI.DeepSeekAPIKey) != "",
		defaultOpenAIOK:                    strings.TrimSpace(cfg.AI.OpenAIAPIKey) != "",
		defaultLocalAIOK:                   strings.TrimSpace(cfg.AI.LocalAIBaseURL) != "",
		pageTitle:                          cfg.App.PageTitle,
		deploymentNatureLocal:              strings.EqualFold(strings.TrimSpace(cfg.App.DeploymentNature), "local"),
		guideTopicsReloadFromFileOnStartup: cfg.App.GuideTopicsReloadFromFileOnStartup,
	}
}

// RegisterRoutes mounts the templated routes.
// Router must call this before r.Handle("/static/*", ...) so /static/js/museum/foundation.js
// is served here (with {{ }} substitution) instead of the raw file from disk.
func (h *TemplateHandler) RegisterRoutes(r chi.Router) {
	r.Get("/", h.GetRoot)
	r.Get("/favicon.ico", h.GetFavicon)
	r.Get("/api/suggestions", h.GetSuggestions)
	r.Get("/api/regions", h.GetRegions)
	r.Get("/static/js/museum/foundation.js", h.GetFoundationJS)
	r.Get("/static/js/museum/modals-people.js", h.GetModalsPeopleJS)
	r.Get("/login", h.GetLogin)
	r.Get("/s/{token}", h.GetSharePage)
}

// loginHostShowsAdvancedPanel is true for loopback hosts so dev servers (e.g. go run on
// localhost) still offer the Advanced entry without requiring DEPLOYMENT_NATURE=local.
func loginHostShowsAdvancedPanel(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return false
	}
	h := host
	if hostname, _, err := net.SplitHostPort(host); err == nil {
		h = strings.ToLower(hostname)
	}
	h = strings.Trim(h, "[]")
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

// GetFavicon handles GET /favicon.ico → serves templates/favicon.ico.
func (h *TemplateHandler) GetFavicon(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(filepath.Join(h.templatesDir, "favicon.ico"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/x-icon")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(data)
}

// GetLogin handles GET /login → serves login.html.
func (h *TemplateHandler) GetLogin(w http.ResponseWriter, r *http.Request) {
	content, err := h.readFile(h.templatesDir, "login.html")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "login page not found")
		return
	}
	hasRegisteredUser := false
	hasVisitorKeys := false
	if h.userRepo != nil {
		exists, err := h.userRepo.AnyNonAdminUserExists(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not load login state")
			return
		}
		hasRegisteredUser = exists
		if hasRegisteredUser {
			visitorKeysExist, err := h.userRepo.HasAnyVisitorKeys(r.Context())
			if err != nil {
				writeError(w, http.StatusInternalServerError, "could not load visitor login state")
				return
			}
			hasVisitorKeys = visitorKeysExist
		}
	}
	showLoginAdvanced := h.deploymentNatureLocal || loginHostShowsAdvancedPanel(r.Host)
	loginExtras := map[string]string{
		"has_registered_user": fmt.Sprintf("%t", hasRegisteredUser),
		"has_visitor_keys":    fmt.Sprintf("%t", hasVisitorKeys),
		"show_login_advanced": fmt.Sprintf("%t", showLoginAdvanced),
	}
	if fi, err := os.Stat(filepath.Join(h.pythonStaticDir, "js", "museum", "local-ai-setup.js")); err == nil {
		loginExtras["static_local_ai_setup_js_cache_bust"] = fmt.Sprintf("%d", fi.ModTime().Unix())
	} else {
		loginExtras["static_local_ai_setup_js_cache_bust"] = "0"
	}
	content = renderJinja(content, map[string]string{}, loginExtras)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(content))
}

// GetSharePage handles GET /s/{token} → serves share.html.
func (h *TemplateHandler) GetSharePage(w http.ResponseWriter, r *http.Request) {
	content, err := h.readFile(h.templatesDir, "share.html")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "share page not found")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(content))
}

// GetRoot handles GET / → renders index.template.html.
func (h *TemplateHandler) GetRoot(w http.ResponseWriter, r *http.Request) {

	ctx := h.buildContext(r)

	if _, err := h.subjectRepo.GetFirst(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error loading subject configuration: %s", err))
		return
	}

	templateName := "index.template.html"

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
	// Bust browser/Electron disk cache for the main app bundle when the file changes on disk.
	if fi, err := os.Stat(filepath.Join(h.pythonStaticDir, "js", "museum", "app.js")); err == nil {
		extras["static_app_js_cache_bust"] = fmt.Sprintf("%d", fi.ModTime().Unix())
	} else {
		extras["static_app_js_cache_bust"] = "0"
	}
	// Bust cache for modals-settings.js because upload modal wiring lives there.
	if fi, err := os.Stat(filepath.Join(h.pythonStaticDir, "js", "museum", "modals-settings.js")); err == nil {
		extras["static_modals_settings_js_cache_bust"] = fmt.Sprintf("%d", fi.ModTime().Unix())
	} else {
		extras["static_modals_settings_js_cache_bust"] = "0"
	}
	if fi, err := os.Stat(filepath.Join(h.pythonStaticDir, "js", "museum", "local-ai-setup.js")); err == nil {
		extras["static_local_ai_setup_js_cache_bust"] = fmt.Sprintf("%d", fi.ModTime().Unix())
	} else {
		extras["static_local_ai_setup_js_cache_bust"] = "0"
	}
	// Bust cache for modals-comms.js (Locations map, SMS, etc.).
	if fi, err := os.Stat(filepath.Join(h.pythonStaticDir, "js", "museum", "modals-comms.js")); err == nil {
		extras["static_modals_comms_js_cache_bust"] = fmt.Sprintf("%d", fi.ModTime().Unix())
	} else {
		extras["static_modals_comms_js_cache_bust"] = "0"
	}
	// Bust cache for modals-media.js (image detail modal, GPS actions, gallery).
	if fi, err := os.Stat(filepath.Join(h.pythonStaticDir, "js", "museum", "modals-media.js")); err == nil {
		extras["static_modals_media_js_cache_bust"] = fmt.Sprintf("%d", fi.ModTime().Unix())
	} else {
		extras["static_modals_media_js_cache_bust"] = "0"
	}
	// Bust cache for modals-artefacts.js (artefact gallery, native file import).
	if fi, err := os.Stat(filepath.Join(h.pythonStaticDir, "js", "museum", "modals-artefacts.js")); err == nil {
		extras["static_modals_artefacts_js_cache_bust"] = fmt.Sprintf("%d", fi.ModTime().Unix())
	} else {
		extras["static_modals_artefacts_js_cache_bust"] = "0"
	}
	// Bust cache for museum_of.css and images-grid.js (Email Attachments layout lives here).
	if fi, err := os.Stat(filepath.Join(h.pythonStaticDir, "css", "museum_of.css")); err == nil {
		extras["static_museum_css_cache_bust"] = fmt.Sprintf("%d", fi.ModTime().Unix())
	} else {
		extras["static_museum_css_cache_bust"] = "0"
	}
	if fi, err := os.Stat(filepath.Join(h.pythonStaticDir, "js", "museum", "images-grid.js")); err == nil {
		extras["static_images_grid_js_cache_bust"] = fmt.Sprintf("%d", fi.ModTime().Unix())
	} else {
		extras["static_images_grid_js_cache_bust"] = "0"
	}
	regionsJSON, err := json.Marshal(georegion.Default().ConfigJSON())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error encoding regions config: %s", err))
		return
	}
	extras["regions_config_json"] = string(regionsJSON)
	// Non-local deployments hide server filesystem path import tiles; local shows them.
	if h.deploymentNatureLocal {
		extras["deployment_nature_body_class"] = ""
		//extras["index_filesystem_import_tile"] = indexFilesystemImportTileHTML
		//		extras["index_filesystem_reference_import_tile"] = indexFilesystemReferenceImportTileHTML
	} else {
		extras["deployment_nature_body_class"] = "deployment-hide-path-import-tiles"
		extras["index_filesystem_import_tile"] = ""
		//		extras["index_filesystem_reference_import_tile"] = ""
	}
	rendered := renderJinja(content, ctx, extras)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	_, _ = w.Write([]byte(rendered))
}

// GetRegions handles GET /api/regions — returns the regions config loaded at startup.
func (h *TemplateHandler) GetRegions(w http.ResponseWriter, r *http.Request) {
	cfg := georegion.Default().ConfigJSON()
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, cfg)
}

// GetSuggestions handles GET /api/suggestions.
// It assembles categories from the database, renders Jinja subject variables, and injects _meta.
func (h *TemplateHandler) GetSuggestions(w http.ResponseWriter, r *http.Request) {
	if h.suggestionsSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "suggestions not available")
		return
	}
	ctx := h.buildContext(r)

	root, err := h.suggestionsSvc.BuildCategoriesDocument(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error loading suggestions: %s", err))
		return
	}

	raw, err := json.Marshal(root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error encoding suggestions: %s", err))
		return
	}
	rendered := renderJinja(string(raw), ctx, nil)
	root, err = unmarshalSuggestionsJSON(rendered)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("suggestions template produced invalid JSON: %s", err))
		return
	}

	meta := map[string]any{
		"deployment_nature_local": ctx["deployment_nature_local"],
	}
	if existing, ok := root["_meta"].(map[string]any); ok {
		for k, v := range meta {
			existing[k] = v
		}
		root["_meta"] = existing
	} else {
		root["_meta"] = meta
	}

	outBytes, err := json.Marshal(root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("suggestions marshal: %s", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	_, _ = w.Write(outBytes)
}

func unmarshalSuggestionsJSON(rendered string) (map[string]any, error) {
	var root map[string]any
	if err := json.Unmarshal([]byte(rendered), &root); err != nil {
		return nil, err
	}
	return root, nil
}

// GetFoundationJS handles GET /static/js/museum/foundation.js.
func (h *TemplateHandler) GetFoundationJS(w http.ResponseWriter, r *http.Request) {
	ctx := h.buildContext(r)
	geminiOK := h.defaultGeminiOK
	claudeOK := h.defaultClaudeOK
	deepseekOK := h.defaultDeepSeekOK
	openaiOK := h.defaultOpenAIOK
	if uid := appctx.UserIDFromCtx(r.Context()); uid != 0 && h.userRepo != nil {
		if stored, err := h.userRepo.GetUserLLMStored(r.Context(), uid); err == nil && stored != nil {
			if strings.TrimSpace(stored.GeminiAPIKey) != "" {
				geminiOK = true
			}
			if strings.TrimSpace(stored.AnthropicAPIKey) != "" {
				claudeOK = true
			}
			if strings.TrimSpace(stored.DeepSeekAPIKey) != "" {
				deepseekOK = true
			}
			if strings.TrimSpace(stored.OpenAIAPIKey) != "" {
				openaiOK = true
			}
		}
	}
	if geminiOK {
		ctx["gemini_configured"] = "True"
	} else {
		ctx["gemini_configured"] = "False"
	}
	if claudeOK {
		ctx["claude_configured"] = "True"
	} else {
		ctx["claude_configured"] = "False"
	}
	if deepseekOK {
		ctx["deepseek_configured"] = "True"
	} else {
		ctx["deepseek_configured"] = "False"
	}
	if openaiOK {
		ctx["openai_configured"] = "True"
	} else {
		ctx["openai_configured"] = "False"
	}
	if h.defaultLocalAIOK {
		ctx["localai_configured"] = "True"
	} else {
		ctx["localai_configured"] = "False"
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

	depLocal := "False"
	if h.deploymentNatureLocal {
		depLocal = "True"
	}
	guideReload := "False"
	if h.guideTopicsReloadFromFileOnStartup {
		guideReload = "True"
	}

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
		"owner_gender":                             gender,
		"todays_thing_prompt":                      "",
		"deployment_nature_local":                  depLocal,
		"guide_topics_reload_from_file_on_startup": guideReload,
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

// indexFilesystemImportTileHTML is the Data Import “Picture and Images” tile (server path scan).
// Omitted from the rendered page when DEPLOYMENT_NATURE is not local (see GetRoot extras).
// const indexFilesystemImportTileHTML = `
//                                 <article class="data-import-card data-import-row data-import-path-row" data-import="filesystem">
//                                     <div class="data-import-card-header">
//                                         <h3 class="data-import-card-title"><i class="fas fa-link"></i> Link to Images on Disk</h3>
//                                     </div>
//                                     <div class="data-import-card-metrics-source" hidden aria-hidden="true">
//                                         <span class="data-import-count" data-import-count-key="filesystem">—</span>
//                                     </div>
//                                     <div class="data-import-card-detail-body" hidden aria-hidden="true"><p>Scan folders on this machine and register image paths as referenced media (local Electron deployments only).<br/><br/>Thumbnails are generated and stored in the database for quick access.  <br/><br/> Optonally, you can select to import the images into the database as part of the process or run the import process at a later stage. <br/><br/> <strong>Clear Data</strong> removes filesystem-linked rows.</p></div>
//                                     <div class="data-import-actions-td">
//                                         <div class="data-import-card-actions-row">
//                                             <div class="data-import-action-group">
//                                                 <button type="button" class="modal-btn modal-btn-primary data-import-start-btn" data-import-start="filesystem"><i class="fas fa-link"></i> Link</button>
//                                                 <button type="button" class="modal-btn modal-btn-secondary data-import-row-cancel-btn" hidden title="Cancel this import"><i class="fas fa-stop"></i> Cancel</button>
//                                             </div>
//                                             <button type="button" class="modal-btn modal-btn-secondary data-import-delete-btn" aria-label="Clear data" data-import-purge-kind="filesystem_media" title="Remove filesystem-sourced images from the library"><i class="fas fa-trash-alt" aria-hidden="true"></i> Clear Data</button>
//                                         </div>
//                                     </div>
//                                 </article>`

// indexFilesystemReferenceImportTileHTML registers paths only (is_referenced); same purge as folder scan.
// const indexFilesystemReferenceImportTileHTML = `                                <tr class="data-import-row data-import-path-row" data-import="filesystem_reference">
//                                     <td><i class="fas fa-link"></i> Picture and Images (reference paths on disk)</td>
//                                     <td class="data-import-count" data-import-count-key="filesystem_reference">—</td>
//                                     <td class="data-import-actions-td">
//                                         <div class="data-import-action-group">
//                                             <button type="button" class="modal-btn modal-btn-primary data-import-start-btn" data-import-start="filesystem_reference"><i class="fas fa-play"></i> Start</button>
//                                             <button type="button" class="modal-btn modal-btn-secondary data-import-row-cancel-btn" hidden title="Cancel this import"><i class="fas fa-stop"></i> Cancel</button>
//                                         </div>
//                                     </td>
//                                     <td><button type="button" class="data-import-delete-btn" aria-label="Delete data" data-import-purge-kind="filesystem_media" title="Remove filesystem-sourced images from the library"><i class="fas fa-trash-alt" aria-hidden="true"></i></button></td>
//                                 </tr>`

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
