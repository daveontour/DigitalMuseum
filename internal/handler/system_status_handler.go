package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	appai "github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/imagetagsync"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/service"
	backgroundjobs "github.com/daveontour/aimuseum/internal/service/background_jobs"
	"github.com/daveontour/aimuseum/internal/service/facerecognizer"
)

// System status and setup checklist (owner only).
//
// GET /api/system-status lists one row per part of the app the owner might need to fix — AI
// connection, the AI tools server, which tools are on, the local search-indexing server, bundled
// helper programs, backups, failed scheduled jobs and post-import steps — each with a plain-language
// message and, where there is one, a fix. GET /api/setup-checklist reports the first-run steps,
// worked out from the archive's actual data, so it can't drift from reality.
//
// A fix is {label, action}; the action is a key the frontend maps to a UI action
// (static/js/museum/system-status.js FIX_ACTIONS) — the server never names DOM elements.

// SetupChecklistDismissedConfigKey hides the setup checklist pill once the owner dismisses it.
const SetupChecklistDismissedConfigKey = "setup_checklist_dismissed_v1"

// Fix actions understood by the frontend.
const (
	fixOpenAPIKeys          = "openConfigApiKeys"
	fixOpenAISetup          = "openConfigAiSetup"
	fixOpenSubject          = "openConfigSubjectConfiguration"
	fixOpenVisitorKeys      = "openConfigManageVisitorKeys"
	fixOpenToolsAccess      = "openConfigToolsAccess"
	fixEnableRecommended    = "enableRecommendedTools"
	fixRefreshToolCatalog   = "refreshToolCatalog"
	fixOpenDataImport       = "openDataImport"
	fixOpenScheduledJobs    = "openScheduledJobs"
	fixOpenPostImport       = "openPostImport"
	fixOpenBackup           = "openBackup"
	fixOpenPeopleInPhotos   = "openPeopleInPhotos"
	statusOK                = "ok"
	statusInfo              = "info"
	statusWarn              = "warn"
	statusError             = "error"
	systemStatusProbeTimeout = 8 * time.Second
)

type statusFix struct {
	Label  string `json:"label"`
	Action string `json:"action"`
}

type statusCheck struct {
	Key     string     `json:"key"`
	Title   string     `json:"title"`
	Status  string     `json:"status"` // ok | info | warn | error
	Message string     `json:"message"`
	Fix     *statusFix `json:"fix,omitempty"`
}

type setupItem struct {
	Key      string     `json:"key"`
	Title    string     `json:"title"`
	Detail   string     `json:"detail"`
	Done     bool       `json:"done"`
	Optional bool       `json:"optional"`
	NotYet   string     `json:"not_yet,omitempty"` // why the step can't be done yet (it then doesn't count)
	Fix      *statusFix `json:"fix,omitempty"`
}

// SystemStatusHandler serves the System status and setup checklist endpoints.
type SystemStatusHandler struct {
	pool            *sql.DB
	configRepo      *repository.ConfigRepo
	userRepo        *repository.UserRepo
	chatSvc         *service.ChatService
	privateStore    *service.PrivateStoreService
	backupSvc       *service.ArchiveBackupService
	authSvc         *service.AuthService
	deploymentLocal bool
}

// NewSystemStatusHandler creates a SystemStatusHandler.
func NewSystemStatusHandler(pool *sql.DB, configRepo *repository.ConfigRepo, userRepo *repository.UserRepo, chatSvc *service.ChatService, privateStore *service.PrivateStoreService, backupSvc *service.ArchiveBackupService, authSvc *service.AuthService, deploymentNature string) *SystemStatusHandler {
	return &SystemStatusHandler{
		pool:            pool,
		configRepo:      configRepo,
		userRepo:        userRepo,
		chatSvc:         chatSvc,
		privateStore:    privateStore,
		backupSvc:       backupSvc,
		authSvc:         authSvc,
		deploymentLocal: strings.EqualFold(strings.TrimSpace(deploymentNature), "local"),
	}
}

// RegisterRoutes mounts the routes.
func (h *SystemStatusHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/system-status", h.Status)
	r.Get("/api/setup-checklist", h.Checklist)
	r.Post("/api/setup-checklist/dismiss", h.DismissChecklist)
}

func (h *SystemStatusHandler) requireOwner(w http.ResponseWriter, r *http.Request) bool {
	if !ArchiveOwnerAuthenticated(r, h.authSvc) {
		writeError(w, http.StatusForbidden, "owner session required")
		return false
	}
	return true
}

// Status handles GET /api/system-status.
func (h *SystemStatusHandler) Status(w http.ResponseWriter, r *http.Request) {
	if !h.requireOwner(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), systemStatusProbeTimeout)
	defer cancel()
	checks := h.checks(ctx, r)
	writeJSON(w, map[string]any{
		"overall":    worstStatus(checks),
		"checks":     checks,
		"checked_at": time.Now().UTC(),
	})
}

func worstStatus(checks []statusCheck) string {
	rank := map[string]int{statusOK: 0, statusInfo: 1, statusWarn: 2, statusError: 3}
	worst := statusOK
	for _, c := range checks {
		if rank[c.Status] > rank[worst] {
			worst = c.Status
		}
	}
	return worst
}

func (h *SystemStatusHandler) checks(ctx context.Context, r *http.Request) []statusCheck {
	uid := appctx.UserIDFromCtx(ctx)
	out := make([]statusCheck, 0, 12)
	add := func(c statusCheck) { out = append(out, c) }

	// Archive file.
	if h.backupSvc != nil {
		c := statusCheck{Key: "archive", Title: "Archive file", Status: statusOK, Message: "Open and readable."}
		if src, err := h.backupSvc.SourcePath(ctx); err != nil {
			c.Status, c.Message = statusError, "Couldn't read the archive file: "+err.Error()
		} else if fi, err := os.Stat(src); err == nil {
			c.Message = fmt.Sprintf("Open and readable (%s).", humanBytes(fi.Size()))
			if h.deploymentLocal && IsLocalhost(r) {
				c.Message = fmt.Sprintf("%s (%s).", src, humanBytes(fi.Size()))
			}
		}
		add(c)
	}

	// AI connection.
	hasKey := h.ownerHasOpenRouterKey(ctx, uid)
	localChat := h.chatSvc != nil && h.chatSvc.LocalAIAvailable(ctx)
	switch {
	case hasKey:
		msg := "Cloud AI is connected (OpenRouter key saved)."
		if localChat {
			msg += " Local AI is available too."
		}
		add(statusCheck{Key: "ai_connection", Title: "AI connection", Status: statusOK, Message: msg})
	case localChat:
		add(statusCheck{Key: "ai_connection", Title: "AI connection", Status: statusWarn,
			Message: "Only the local AI on this computer is available. It works offline but gives simpler, slower answers. Add an OpenRouter key to use cloud AI models.",
			Fix:     &statusFix{Label: "Add API key", Action: fixOpenAPIKeys}})
	default:
		add(statusCheck{Key: "ai_connection", Title: "AI connection", Status: statusError,
			Message: "No AI is connected, so the chat can't answer questions. Add an OpenRouter key, or set up the local AI.",
			Fix:     &statusFix{Label: "Add API key", Action: fixOpenAPIKeys}})
	}

	// AI tools server + which tools are on.
	toolCount, _, toolErr := appai.DefaultToolCatalog().Status()
	switch {
	case strings.TrimSpace(os.Getenv("MCP_SERVER_URL")) == "" && toolCount == 0:
		add(statusCheck{Key: "tools_server", Title: "AI tools server", Status: statusError,
			Message: "The AI tools server isn't configured, so the AI can't look at your archive. It is started by the desktop app."})
	case toolCount == 0:
		msg := "The AI tools server isn't responding, so the AI can't look at your archive."
		if toolErr != nil {
			msg += " (" + toolErr.Error() + ")"
		}
		add(statusCheck{Key: "tools_server", Title: "AI tools server", Status: statusError, Message: msg,
			Fix: &statusFix{Label: "Try again", Action: fixRefreshToolCatalog}})
	case toolErr != nil:
		add(statusCheck{Key: "tools_server", Title: "AI tools server", Status: statusWarn,
			Message: fmt.Sprintf("%d tools available, but one tools server didn't respond: %s", toolCount, toolErr.Error()),
			Fix:     &statusFix{Label: "Try again", Action: fixRefreshToolCatalog}})
	default:
		add(statusCheck{Key: "tools_server", Title: "AI tools server", Status: statusOK,
			Message: fmt.Sprintf("Running — %d tools available.", toolCount)})
	}
	if toolCount > 0 {
		sum := h.toolSummary(ctx)
		switch {
		case sum.EnabledForOwner == 0:
			add(statusCheck{Key: "tools_enabled", Title: "AI tools turned on", Status: statusError,
				Message: "No AI tools are turned on, so the AI can't search your messages, emails or photos.",
				Fix:     &statusFix{Label: "Turn on recommended tools", Action: fixEnableRecommended}})
		case sum.RecommendedOff > 0:
			add(statusCheck{Key: "tools_enabled", Title: "AI tools turned on", Status: statusInfo,
				Message: fmt.Sprintf("%d of %d tools are on. %d recommended tools are still off.", sum.EnabledForOwner, sum.Available, sum.RecommendedOff),
				Fix:     &statusFix{Label: "Turn on recommended tools", Action: fixEnableRecommended}})
		default:
			add(statusCheck{Key: "tools_enabled", Title: "AI tools turned on", Status: statusOK,
				Message: fmt.Sprintf("%d of %d tools are on.", sum.EnabledForOwner, sum.Available),
				Fix:     &statusFix{Label: "Review", Action: fixOpenToolsAccess}})
		}
	}

	// Local AI: search indexing (embeddings) and local chat.
	if h.chatSvc != nil {
		rep := h.chatSvc.LocalAIStatusProbe(ctx, false)
		switch {
		case strings.TrimSpace(rep.EmbeddingBaseURL) == "":
			add(statusCheck{Key: "search_indexing", Title: "Search indexing (local AI)", Status: statusWarn,
				Message: "Not configured. Search by Meaning and making new items searchable won't work.",
				Fix:     &statusFix{Label: "Open AI setup", Action: fixOpenAISetup}})
		case !rep.EmbeddingServerReachable:
			add(statusCheck{Key: "search_indexing", Title: "Search indexing (local AI)", Status: statusWarn,
				Message: "The local indexing server isn't running. New items won't become searchable, and Search by Meaning won't work until it is.",
				Fix:     &statusFix{Label: "Open AI setup", Action: fixOpenAISetup}})
		case !rep.EmbeddingModelAvailable:
			add(statusCheck{Key: "search_indexing", Title: "Search indexing (local AI)", Status: statusWarn,
				Message: fmt.Sprintf("The indexing model %q hasn't been downloaded yet.", rep.EmbeddingModel),
				Fix:     &statusFix{Label: "Open AI setup", Action: fixOpenAISetup}})
		default:
			add(statusCheck{Key: "search_indexing", Title: "Search indexing (local AI)", Status: statusOK, Message: "Running."})
		}
		switch {
		case !h.chatSvc.LocalAIUseEnabled(ctx):
			add(statusCheck{Key: "local_chat", Title: "Local AI chat", Status: statusInfo, Message: "Turned off for this archive.",
				Fix: &statusFix{Label: "Open AI setup", Action: fixOpenAISetup}})
		case h.chatSvc.LocalAIInfrastructureAvailable(ctx):
			add(statusCheck{Key: "local_chat", Title: "Local AI chat", Status: statusOK, Message: "Available (starts when first used)."})
		default:
			st := statusInfo
			if !hasKey {
				st = statusWarn
			}
			add(statusCheck{Key: "local_chat", Title: "Local AI chat", Status: st, Message: "Not available — the local chat server or its model isn't ready.",
				Fix: &statusFix{Label: "Open AI setup", Action: fixOpenAISetup}})
		}
	}

	// Bundled helper programs.
	if bundledFileExists(magickRelPath()) {
		add(statusCheck{Key: "imagemagick", Title: "Thumbnail maker (ImageMagick)", Status: statusOK, Message: "Installed."})
	} else {
		add(statusCheck{Key: "imagemagick", Title: "Thumbnail maker (ImageMagick)", Status: statusWarn,
			Message: "Not found in bin/ImageMagick. Photo thumbnails and face pictures can't be made. Reinstalling Digital Museum should fix this."})
	}
	if _, err := imagetagsync.ResolveExifTool(""); err == nil {
		add(statusCheck{Key: "exiftool", Title: "Photo details reader (ExifTool)", Status: statusOK, Message: "Installed."})
	} else {
		add(statusCheck{Key: "exiftool", Title: "Photo details reader (ExifTool)", Status: statusInfo,
			Message: "Not found in bin/exiftool. Photo tags can't be read from or written back to image files."})
	}
	if bundledFileExists(facerecognizer.DefaultBundledPath()) {
		add(statusCheck{Key: "face_recognition", Title: "Face recognition", Status: statusOK, Message: "Installed."})
	} else {
		add(statusCheck{Key: "face_recognition", Title: "Face recognition", Status: statusInfo,
			Message: "Not installed, so People in Photos won't find faces. Everything else works without it."})
	}

	// Backup.
	if h.deploymentLocal && h.backupSvc != nil {
		hasData := h.backupSvc.ArchiveHasData(ctx)
		st := h.backupSvc.Status(ctx, hasData, time.Now().UTC())
		fix := &statusFix{Label: "Back up now", Action: fixOpenBackup}
		switch {
		case st.Running:
			add(statusCheck{Key: "backup", Title: "Backup", Status: statusInfo, Message: "A backup is being made now."})
		case st.LastBackupAt == nil && hasData:
			add(statusCheck{Key: "backup", Title: "Backup", Status: statusWarn, Message: "This archive has never been backed up.", Fix: fix})
		case st.LastBackupAt == nil:
			add(statusCheck{Key: "backup", Title: "Backup", Status: statusInfo, Message: "Nothing to back up yet."})
		case time.Since(*st.LastBackupAt) > service.BackupReminderAfter:
			add(statusCheck{Key: "backup", Title: "Backup", Status: statusWarn,
				Message: fmt.Sprintf("Last backup was %d days ago.", int(time.Since(*st.LastBackupAt).Hours()/24)), Fix: fix})
		default:
			add(statusCheck{Key: "backup", Title: "Backup", Status: statusOK,
				Message: "Last backup " + daysAgo(*st.LastBackupAt) + "."})
		}
	}

	// Scheduled jobs whose last run failed.
	if failed := h.failedJobs(ctx, uid); len(failed) > 0 {
		add(statusCheck{Key: "scheduled_jobs", Title: "Scheduled jobs", Status: statusWarn,
			Message: fmt.Sprintf("Last run failed: %s.", strings.Join(failed, ", ")),
			Fix:     &statusFix{Label: "Open scheduled jobs", Action: fixOpenScheduledJobs}})
	} else {
		add(statusCheck{Key: "scheduled_jobs", Title: "Scheduled jobs", Status: statusOK, Message: "No failures."})
	}

	// Last post-import pass.
	if run, _ := postImport.status(uid); run != nil && !run.Running {
		var problems, skipped []string
		for _, s := range run.Steps {
			switch s.Status {
			case "error":
				problems = append(problems, s.Title)
			case "skipped":
				skipped = append(skipped, s.Title)
			}
		}
		fix := &statusFix{Label: "See details", Action: fixOpenPostImport}
		switch {
		case len(problems) > 0:
			add(statusCheck{Key: "post_import", Title: "Preparing imported data", Status: statusWarn,
				Message: "Some steps had problems after the last import: " + strings.Join(problems, ", ") + ".", Fix: fix})
		case len(skipped) > 0:
			add(statusCheck{Key: "post_import", Title: "Preparing imported data", Status: statusInfo,
				Message: "Some steps were skipped after the last import: " + strings.Join(skipped, ", ") + ".", Fix: fix})
		}
	}
	return out
}

func (h *SystemStatusHandler) ownerHasOpenRouterKey(ctx context.Context, uid int64) bool {
	if h.userRepo == nil || uid == 0 {
		return false
	}
	stored, err := h.userRepo.GetUserLLMStored(ctx, uid)
	return err == nil && stored != nil && strings.TrimSpace(stored.OpenRouterAPIKey) != ""
}

func (h *SystemStatusHandler) toolSummary(ctx context.Context) appai.ToolAccessSummary {
	var policy appai.ToolAccessPolicy
	if h.privateStore != nil {
		policy, _ = h.privateStore.LoadLLMToolsAccessPolicyMirror(ctx)
	}
	return appai.SummarizeToolAccess(policy, appai.AllToolMetas())
}

func (h *SystemStatusHandler) failedJobs(ctx context.Context, uid int64) []string {
	out := []string{}
	if h.pool == nil {
		return out
	}
	q := `SELECT job_name FROM background_jobs WHERE last_run_result = 'error'`
	var args []any
	if uid != 0 {
		q += ` AND user_id = ?`
		args = append(args, uid)
	}
	rows, err := h.pool.QueryContext(ctx, q+` ORDER BY job_name`, args...)
	if err != nil {
		return out
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil {
			out = append(out, jobDisplayName(name))
		}
	}
	return out
}

// Checklist handles GET /api/setup-checklist.
func (h *SystemStatusHandler) Checklist(w http.ResponseWriter, r *http.Request) {
	if !h.requireOwner(w, r) {
		return
	}
	ctx := r.Context()
	items := h.checklistItems(ctx)
	done, total := 0, 0
	for _, it := range items {
		if it.Optional || it.NotYet != "" {
			continue
		}
		total++
		if it.Done {
			done++
		}
	}
	dismissed := false
	if h.configRepo != nil {
		if raw, err := h.configRepo.GetValueByKey(ctx, SetupChecklistDismissedConfigKey); err == nil && raw != nil {
			dismissed = strings.EqualFold(strings.TrimSpace(*raw), "true")
		}
	}
	writeJSON(w, map[string]any{
		"items":     items,
		"done":      done,
		"total":     total,
		"complete":  done == total,
		"dismissed": dismissed,
	})
}

// DismissChecklist handles POST /api/setup-checklist/dismiss {"dismissed": true|false}.
func (h *SystemStatusHandler) DismissChecklist(w http.ResponseWriter, r *http.Request) {
	if !h.requireOwner(w, r) {
		return
	}
	var req struct {
		Dismissed *bool `json:"dismissed"`
	}
	_ = decodeJSONBody(r, &req)
	val := "true"
	if req.Dismissed != nil && !*req.Dismissed {
		val = "false"
	}
	f := false
	desc := "Setup checklist hidden from the top bar"
	if _, err := h.configRepo.Upsert(r.Context(), SetupChecklistDismissedConfigKey, &val, &f, &desc); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"dismissed": val == "true"})
}

func (h *SystemStatusHandler) checklistItems(ctx context.Context) []setupItem {
	uid := appctx.UserIDFromCtx(ctx)
	subjectName, subjectContact := h.subjectInfo(ctx, uid)
	if subjectName == "" {
		subjectName = "the person this museum is about"
	}

	items := []setupItem{}
	aiOK := h.ownerHasOpenRouterKey(ctx, uid) || (h.chatSvc != nil && h.chatSvc.LocalAIAvailable(ctx))
	items = append(items, setupItem{
		Key: "connect_ai", Title: "Connect an AI",
		Detail: "Add an OpenRouter API key (or set up the local AI) so the chat can answer questions.",
		Done:   aiOK, Fix: &statusFix{Label: "Add API key", Action: fixOpenAPIKeys},
	})
	items = append(items, setupItem{
		Key: "subject_details", Title: "Add " + subjectName + "'s email addresses and phone numbers",
		Detail: "So imports can tell which messages and emails they sent.",
		Done:   subjectContact, Fix: &statusFix{Label: "Open subject details", Action: fixOpenSubject},
	})
	hasData := h.backupSvc != nil && h.backupSvc.ArchiveHasData(ctx)
	items = append(items, setupItem{
		Key: "first_import", Title: "Import your first data",
		Detail: "Emails, messages, Facebook, Instagram or photos.",
		Done:   hasData, Fix: &statusFix{Label: "Add data", Action: fixOpenDataImport},
	})
	toolCount, _, _ := appai.DefaultToolCatalog().Status()
	toolsItem := setupItem{
		Key: "ai_tools", Title: "Let the AI search your archive",
		Detail: "Turn on the recommended AI tools so answers can draw on your messages, emails and photos.",
		Done:   h.toolSummary(ctx).EnabledForOwner > 0, Fix: &statusFix{Label: "Turn on recommended tools", Action: fixEnableRecommended},
	}
	if toolCount == 0 && !toolsItem.Done {
		toolsItem.NotYet = "Waiting for the AI tools server — see System status."
	}
	items = append(items, toolsItem)

	clusters, named := h.faceClusterCounts(ctx, uid)
	people := setupItem{
		Key: "name_people", Title: "Name the people in your photos",
		Detail: "Tell the museum who's who, so you can find photos of a person and the AI can too.",
		Done:   named > 0, Optional: true, Fix: &statusFix{Label: "Open People in Photos", Action: fixOpenPeopleInPhotos},
	}
	if clusters == 0 {
		people.NotYet = "Available once photos have been imported and faces found."
	}
	items = append(items, people)

	if h.deploymentLocal && h.backupSvc != nil {
		st := h.backupSvc.Status(ctx, hasData, time.Now().UTC())
		backup := setupItem{
			Key: "first_backup", Title: "Make your first backup",
			Detail: "One click saves a copy of the whole archive somewhere safe.",
			Done:   st.LastBackupAt != nil, Fix: &statusFix{Label: "Back up now", Action: fixOpenBackup},
		}
		if !hasData && !backup.Done {
			backup.NotYet = "Available once you've imported some data."
		}
		items = append(items, backup)
	}

	items = append(items, setupItem{
		Key: "share", Title: "Share with family",
		Detail: "Create a visitor key so others can explore the museum with the access you choose.",
		Done:   h.visitorKeyCount(ctx, uid) > 0, Optional: true,
		Fix: &statusFix{Label: "Manage visitor keys", Action: fixOpenVisitorKeys},
	})
	return items
}

func (h *SystemStatusHandler) subjectInfo(ctx context.Context, uid int64) (name string, hasContactDetails bool) {
	if h.pool == nil {
		return "", false
	}
	q := `SELECT COALESCE(subject_name, ''), COALESCE(email_addresses, ''), COALESCE(phone_numbers, '') FROM subject_configuration WHERE 1=1`
	var args []any
	if uid != 0 {
		q += ` AND user_id = ?`
		args = append(args, uid)
	}
	var emails, phones string
	if err := h.pool.QueryRowContext(ctx, q+` ORDER BY id LIMIT 1`, args...).Scan(&name, &emails, &phones); err != nil {
		return "", false
	}
	return strings.TrimSpace(name), hasListValue(emails) || hasListValue(phones)
}

// hasListValue reports whether a stored email/phone list (plain text or JSON array) has an entry.
func hasListValue(raw string) bool {
	v := strings.TrimSpace(raw)
	v = strings.Trim(v, "[]{}\" \t\r\n,")
	return v != "" && !strings.EqualFold(v, "null")
}

func (h *SystemStatusHandler) faceClusterCounts(ctx context.Context, uid int64) (total, named int) {
	if h.pool == nil {
		return 0, 0
	}
	q := `SELECT COUNT(*), COUNT(contact_id) FROM face_clusters WHERE 1=1`
	var args []any
	if uid != 0 {
		q += ` AND user_id = ?`
		args = append(args, uid)
	}
	_ = h.pool.QueryRowContext(ctx, q, args...).Scan(&total, &named)
	return total, named
}

func (h *SystemStatusHandler) visitorKeyCount(ctx context.Context, uid int64) int {
	if h.pool == nil {
		return 0
	}
	q := `SELECT COUNT(*) FROM sensitive_keyring WHERE is_master = 0`
	var args []any
	if uid != 0 {
		q += ` AND user_id = ?`
		args = append(args, uid)
	}
	var n int
	_ = h.pool.QueryRowContext(ctx, q, args...).Scan(&n)
	return n
}

func magickRelPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join("bin", "ImageMagick", "magick.exe")
	}
	return filepath.Join("bin", "ImageMagick", "magick")
}

// bundledFileExists checks rel against the working directory (how the importers run bundled
// programs) and next to the server executable.
func bundledFileExists(rel string) bool {
	candidates := []string{rel}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), rel))
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return true
		}
	}
	return false
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func daysAgo(t time.Time) string {
	days := int(time.Since(t).Hours() / 24)
	switch {
	case days <= 0:
		return "today"
	case days == 1:
		return "yesterday"
	default:
		return fmt.Sprintf("%d days ago", days)
	}
}

// jobDisplayName maps a background job name to its title from the job registry.
func jobDisplayName(name string) string {
	for _, d := range backgroundjobs.DefaultDefinitions {
		if d.Name == name && strings.TrimSpace(d.Title) != "" {
			return d.Title
		}
	}
	return name
}

// decodeJSONBody decodes an optional JSON body; an empty body is not an error.
func decodeJSONBody(r *http.Request, dst any) error {
	if r.Body == nil {
		return nil
	}
	err := json.NewDecoder(r.Body).Decode(dst)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}
