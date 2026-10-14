package handler

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/importer"
	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/service"
	backgroundjobs "github.com/daveontour/aimuseum/internal/service/background_jobs"
)

// Post-import processing ("Getting your archive ready").
//
// After any import finishes, startPostImportProcessing queues a one-shot pass over the
// maintenance steps that make the new data useful — contact refresh, thumbnails, search
// embeddings, and face detection/grouping — so the owner doesn't have to know to click each
// "Optimise … for Searching" card or enable each background job by hand.
//
// Each step reuses the existing job singletons (and, for background jobs, the same
// JobRunner.Start the scheduler and the "Run now" button use), so behaviour is identical to
// running them manually. Steps run one at a time; a step that can't run (e.g. the local
// embedding server isn't up) is recorded as skipped and the pipeline moves on.
//
// Paid steps are never started here: "Tag photos automatically" (RunPod) is left to the
// owner's own Background Jobs schedule.
//
// The whole feature can be turned off per archive with app_configuration key
// PostImportProcessingConfigKey ("false"); default is on.

// PostImportProcessingConfigKey is the app_configuration key that enables/disables automatic
// post-import processing. Unset means enabled.
const PostImportProcessingConfigKey = "post_import_processing_enabled_v1"

// postImportKind flags which kinds of data an import added.
type postImportKind uint8

const (
	postImportMessages postImportKind = 1 << iota
	postImportEmails
	postImportPhotos
)

// Step keys (shown in the UI by title; keys are stable identifiers).
const (
	postImportStepContacts = "contacts"
)

type postImportStep struct {
	Key     string `json:"key"`
	Title   string `json:"title"`
	Status  string `json:"status"` // pending | running | done | skipped | error | cancelled
	Message string `json:"message,omitempty"`
}

type postImportRun struct {
	UserID     int64            `json:"-"`
	Steps      []postImportStep `json:"steps"`
	StartedAt  time.Time        `json:"started_at"`
	FinishedAt *time.Time       `json:"finished_at,omitempty"`
	Running    bool             `json:"running"`
	Cancelled  bool             `json:"cancelled"`
}

type postImportPipeline struct {
	mu sync.Mutex

	pool       *sql.DB
	runner     backgroundjobs.JobRunner
	jobsRepo   *repository.BackgroundJobRepo
	scheduler  *backgroundjobs.Scheduler
	configRepo *repository.ConfigRepo

	pending   map[int64]postImportKind
	active    bool
	cancelReq bool
	current   *postImportRun
	lastByUID map[int64]*postImportRun

	// pollInterval and stepTimeout are fields so tests can shorten them.
	pollInterval time.Duration
	stepTimeout  time.Duration
}

var postImport = &postImportPipeline{
	pending:      map[int64]postImportKind{},
	lastByUID:    map[int64]*postImportRun{},
	pollInterval: 2 * time.Second,
	stepTimeout:  12 * time.Hour,
}

// ConfigurePostImportPipeline wires the dependencies the pipeline needs. Called once from the
// router after the background jobs runner/scheduler exist. Until it is called (e.g. in the
// no-archive minimal router), startPostImportProcessing is a no-op.
func ConfigurePostImportPipeline(pool *sql.DB, runner backgroundjobs.JobRunner, jobsRepo *repository.BackgroundJobRepo, scheduler *backgroundjobs.Scheduler, configRepo *repository.ConfigRepo) {
	postImport.mu.Lock()
	defer postImport.mu.Unlock()
	postImport.pool = pool
	postImport.runner = runner
	postImport.jobsRepo = jobsRepo
	postImport.scheduler = scheduler
	postImport.configRepo = configRepo
}

// startPostImportProcessing queues post-import processing for uid. Safe to call from any import
// goroutine; returns immediately. Multiple imports in a row are merged into one follow-up pass.
func startPostImportProcessing(uid int64, kinds postImportKind) {
	postImport.enqueue(uid, kinds)
}

func (p *postImportPipeline) enqueue(uid int64, kinds postImportKind) {
	if uid <= 0 || kinds == 0 {
		return
	}
	p.mu.Lock()
	if p.runner == nil || p.pool == nil {
		p.mu.Unlock()
		return
	}
	configRepo := p.configRepo
	p.mu.Unlock()

	if !postImportEnabled(context.WithValue(context.Background(), appctx.ContextKeyUserID, uid), configRepo) {
		slog.Info("post-import processing disabled for archive; skipping", "uid", uid)
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.pending[uid] |= kinds
	if p.active {
		return // the running loop picks it up when the current pass ends
	}
	p.active = true
	go p.loop()
}

func postImportEnabled(ctx context.Context, configRepo *repository.ConfigRepo) bool {
	if configRepo == nil {
		return true
	}
	raw, err := configRepo.GetValueByKey(ctx, PostImportProcessingConfigKey)
	if err != nil || raw == nil {
		return true
	}
	switch strings.TrimSpace(strings.ToLower(*raw)) {
	case "false", "0", "no", "off":
		return false
	}
	return true
}

// loop drains the pending queue one user-pass at a time.
func (p *postImportPipeline) loop() {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("post-import pipeline panic recovered", "panic", r)
			p.mu.Lock()
			p.active = false
			if p.current != nil {
				now := time.Now().UTC()
				p.current.Running = false
				p.current.FinishedAt = &now
			}
			p.mu.Unlock()
		}
	}()
	for {
		p.mu.Lock()
		var uid int64
		var kinds postImportKind
		for u, k := range p.pending {
			uid, kinds = u, k
			break
		}
		if uid == 0 {
			p.active = false
			p.mu.Unlock()
			return
		}
		delete(p.pending, uid)
		p.cancelReq = false
		run := &postImportRun{
			UserID:    uid,
			Steps:     postImportPlan(kinds),
			StartedAt: time.Now().UTC(),
			Running:   true,
		}
		p.current = run
		p.lastByUID[uid] = run
		p.mu.Unlock()

		p.execute(run)
	}
}

// postImportPlan returns the ordered steps for the given data kinds.
func postImportPlan(kinds postImportKind) []postImportStep {
	var steps []postImportStep
	add := func(key, title string) {
		steps = append(steps, postImportStep{Key: key, Title: title, Status: "pending"})
	}
	if kinds&(postImportMessages|postImportEmails) != 0 {
		add(postImportStepContacts, "Update contacts")
	}
	// Every import can bring images (message attachments, email attachments, album photos).
	add(backgroundjobs.JobThumbnails, "Create photo thumbnails")
	if kinds&postImportMessages != 0 {
		add(backgroundjobs.JobMessageContextEmbeddings, "Make messages searchable")
	}
	if kinds&postImportEmails != 0 {
		add(backgroundjobs.JobEmailEmbeddings, "Make emails searchable")
	}
	add(backgroundjobs.JobImageTagEmbeddings, "Make photo tags searchable")
	add(backgroundjobs.JobFaceDetection, "Find faces in photos")
	add(backgroundjobs.JobFaceClustering, "Group similar faces")
	add(backgroundjobs.JobFaceSuggestions, "Suggest names for unnamed faces")
	return steps
}

func (p *postImportPipeline) setStep(run *postImportRun, i int, status, msg string) {
	p.mu.Lock()
	run.Steps[i].Status = status
	run.Steps[i].Message = msg
	p.mu.Unlock()
}

func (p *postImportPipeline) cancelled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cancelReq
}

func (p *postImportPipeline) execute(run *postImportRun) {
	slog.Info("post-import processing started", "uid", run.UserID, "steps", len(run.Steps))
	for i := range run.Steps {
		if p.cancelled() {
			for j := i; j < len(run.Steps); j++ {
				p.setStep(run, j, "cancelled", "")
			}
			break
		}
		step := run.Steps[i]
		p.setStep(run, i, "running", "")
		status, msg := p.runStep(run.UserID, step.Key)
		p.setStep(run, i, status, msg)
		slog.Info("post-import step finished", "uid", run.UserID, "step", step.Key, "status", status, "message", msg)
	}
	p.mu.Lock()
	now := time.Now().UTC()
	run.Running = false
	run.FinishedAt = &now
	run.Cancelled = p.cancelReq
	p.mu.Unlock()
}

// runStep starts one step and blocks until it finishes. Returns (status, message).
func (p *postImportPipeline) runStep(uid int64, key string) (string, string) {
	if key == postImportStepContacts {
		job := contactsExtractJob
		if job.InProgress() {
			return p.waitFor(job, true)
		}
		if err := job.AssertNotRunning(); err != nil {
			return p.waitFor(job, true)
		}
		job.Start()
		job.UpdateState(map[string]any{
			"status": "in_progress", "status_line": "Updating contacts (automatic, after import)...",
			"contacts_processed": 0, "contacts_merged": 0, "contacts_created": 0,
		})
		job.Broadcast("status", map[string]any{"status_line": "Updating contacts (automatic, after import)..."})
		go runContactsExtractInProcess(p.pool, job, uid)
		return p.waitFor(job, false)
	}

	job, ok := backgroundJobSingleton(key)
	if !ok {
		return "skipped", "Unknown step"
	}
	// Already running (the import's own thumbnail pass, or a scheduled run): wait for it.
	if job.InProgress() {
		return p.waitFor(job, true)
	}
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
	if err := p.runner.Start(ctx, key, uid); err != nil {
		if job.InProgress() { // lost a race with the scheduler — same outcome
			return p.waitFor(job, true)
		}
		return "skipped", friendlyPostImportSkip(err)
	}
	// Keep the Background Jobs panel in step, exactly like its "Run now" button does.
	if p.jobsRepo != nil {
		if row, err := p.jobsRepo.GetByName(ctx, key); err == nil && row != nil {
			if err := p.jobsRepo.MarkRunning(ctx, uid, key); err == nil && p.scheduler != nil {
				p.scheduler.TrackManualStart(uid, key)
			}
		}
	}
	return p.waitFor(job, false)
}

// waitFor polls job until it is idle (or the pipeline is cancelled), then maps its final state.
func (p *postImportPipeline) waitFor(job *importer.ImportJob, joinedExisting bool) (string, string) {
	deadline := time.Now().Add(p.stepTimeout)
	for job.InProgress() {
		if p.cancelled() {
			job.Cancel()
			for job.InProgress() && time.Now().Before(deadline) {
				time.Sleep(p.pollInterval)
			}
			return "cancelled", ""
		}
		if time.Now().After(deadline) {
			return "error", "Timed out waiting for this step"
		}
		time.Sleep(p.pollInterval)
	}
	state := job.Status()
	st, _ := state["status"].(string)
	line, _ := state["status_line"].(string)
	line = strings.TrimSpace(line)
	if b, _ := state["cancelled"].(bool); b && st != "completed" {
		return "cancelled", line
	}
	switch st {
	case "error":
		return "error", line
	case "cancelled":
		return "cancelled", line
	}
	if joinedExisting && line == "" {
		line = "Finished (was already running)"
	}
	return "done", line
}

func friendlyPostImportSkip(err error) string {
	msg := err.Error()
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "embedding service not available"):
		return "Local AI embedding server isn't running — start Local AI in Configuration → Local AI Setup, then run this from Scheduled Jobs"
	case strings.Contains(low, "not configured"):
		return "Not available in this installation"
	}
	return msg
}

// backgroundJobSingleton maps a background job name to its ImportJob singleton.
func backgroundJobSingleton(name string) (*importer.ImportJob, bool) {
	switch name {
	case backgroundjobs.JobThumbnails:
		return thumbnailsJob, true
	case backgroundjobs.JobImageTagEmbeddings:
		return imageTagEmbeddingJob, true
	case backgroundjobs.JobMessageContextEmbeddings:
		return messageContextEmbeddingBackfillJob, true
	case backgroundjobs.JobEmailEmbeddings:
		return emailEmbeddingBackfillJob, true
	case backgroundjobs.JobFaceDetection:
		return faceDetectionJob, true
	case backgroundjobs.JobFaceClustering:
		return faceClusteringJob, true
	case backgroundjobs.JobFaceSuggestions:
		return faceSuggestionsJob, true
	}
	return nil, false
}

// status returns a copy of the caller's most recent run (nil when none).
func (p *postImportPipeline) status(uid int64) (*postImportRun, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	run := p.lastByUID[uid]
	_, queued := p.pending[uid]
	if run == nil {
		return nil, queued
	}
	cp := *run
	cp.Steps = append([]postImportStep(nil), run.Steps...)
	return &cp, queued
}

// postImportBusy reports whether uid has a post-import pass running or queued.
func postImportBusy(uid int64) bool {
	run, queued := postImport.status(uid)
	return queued || (run != nil && run.Running)
}

// cancel stops the caller's run (current step's job is cancelled) and drops any queued pass.
func (p *postImportPipeline) cancel(uid int64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.pending, uid)
	if p.current != nil && p.current.UserID == uid && p.current.Running {
		p.cancelReq = true
		return true
	}
	return false
}

// ── HTTP ──────────────────────────────────────────────────────────────────────

// PostImportHandler exposes the post-import pipeline status to the UI.
type PostImportHandler struct {
	configRepo   *repository.ConfigRepo
	sessionStore *keystore.SessionMasterStore
	sensitiveSvc *service.SensitiveService
	authSvc      *service.AuthService
}

// NewPostImportHandler creates a PostImportHandler.
func NewPostImportHandler(configRepo *repository.ConfigRepo, sessionStore *keystore.SessionMasterStore, sensitiveSvc *service.SensitiveService, authSvc *service.AuthService) *PostImportHandler {
	return &PostImportHandler{configRepo: configRepo, sessionStore: sessionStore, sensitiveSvc: sensitiveSvc, authSvc: authSvc}
}

// RegisterRoutes mounts the post-import routes.
func (h *PostImportHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/post-import/status", h.Status)
	r.Post("/api/post-import/cancel", h.Cancel)
}

// Status returns {enabled, queued, run} for the calling user.
func (h *PostImportHandler) Status(w http.ResponseWriter, r *http.Request) {
	uid := appctx.UserIDFromCtx(r.Context())
	run, queued := postImport.status(uid)
	out := map[string]any{
		"enabled": postImportEnabled(r.Context(), h.configRepo),
		"queued":  queued,
		"run":     nil,
	}
	if run != nil {
		out["run"] = run
	}
	writeJSON(w, out)
}

// Cancel stops the calling user's post-import processing.
func (h *PostImportHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlockOrNoKeyring(w, r, h.sessionStore, h.sensitiveSvc, h.authSvc) {
		return
	}
	uid := appctx.UserIDFromCtx(r.Context())
	stopped := postImport.cancel(uid)
	writeJSON(w, map[string]any{"stopped": stopped, "message": fmt.Sprintf("Post-import processing %s", map[bool]string{true: "stopping", false: "was not running"}[stopped])})
}
