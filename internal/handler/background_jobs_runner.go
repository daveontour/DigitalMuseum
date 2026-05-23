package handler

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/importer"
	"github.com/daveontour/aimuseum/internal/service"
	backgroundjobs "github.com/daveontour/aimuseum/internal/service/background_jobs"
)

// backgroundJobsRunner adapts the existing in-process worker functions
// (and their *importer.ImportJob singletons) to the
// service/background_jobs.JobRunner interface so the scheduler and the new
// HTTP handler can drive them without going through the SSE endpoints.
type backgroundJobsRunner struct {
	pool         *sql.DB
	imageSvc     *service.ImageService
	embeddingSvc *service.EmbeddingService
}

// NewBackgroundJobsRunner builds a runner over the existing import-job singletons
// in this package. pool, imageSvc, and embeddingSvc may be nil — jobs requiring
// a missing dependency report a clear error from Start().
func NewBackgroundJobsRunner(pool *sql.DB, imageSvc *service.ImageService, embeddingSvc *service.EmbeddingService) backgroundjobs.JobRunner {
	return &backgroundJobsRunner{
		pool:         pool,
		imageSvc:     imageSvc,
		embeddingSvc: embeddingSvc,
	}
}

// Definitions lists the five maintenance jobs surfaced in the UI.
func (r *backgroundJobsRunner) Definitions() []backgroundjobs.JobDef {
	return backgroundjobs.DefaultDefinitions
}

// jobByName returns the underlying singleton ImportJob for a given canonical name.
func (r *backgroundJobsRunner) jobByName(name string) (*importer.ImportJob, bool) {
	switch name {
	case backgroundjobs.JobThumbnails:
		return thumbnailsJob, true
	case backgroundjobs.JobImageTagEmbeddings:
		return imageTagEmbeddingJob, true
	case backgroundjobs.JobImageAIClassification:
		return imageAIClassificationJob, true
	case backgroundjobs.JobMessageContextEmbeddings:
		return messageContextEmbeddingBackfillJob, true
	case backgroundjobs.JobEmailEmbeddings:
		return emailEmbeddingBackfillJob, true
	}
	return nil, false
}

// Status reports whether the named job is running and the most recent status_line.
func (r *backgroundJobsRunner) Status(jobName string) (bool, string) {
	job, ok := r.jobByName(jobName)
	if !ok || job == nil {
		return false, ""
	}
	state := job.Status()
	inProgress, _ := state["in_progress"].(bool)
	line, _ := state["status_line"].(string)
	return inProgress, line
}

// IdleOutcome maps the in-memory job status to a persisted last_run_result.
func (r *backgroundJobsRunner) IdleOutcome(jobName string) (string, string) {
	job, ok := r.jobByName(jobName)
	if !ok || job == nil {
		return "interrupted", "Job was marked running but is no longer in progress"
	}
	state := job.Status()
	line, _ := state["status_line"].(string)
	if strings.TrimSpace(line) == "" {
		line = "Job was marked running but is no longer in progress"
	}
	st, _ := state["status"].(string)
	switch st {
	case "completed":
		return "completed", line
	case "cancelled":
		return "cancelled", line
	case "error":
		return "error", line
	default:
		return "interrupted", line
	}
}

// Cancel signals cancellation of the named job (no-op if idle).
func (r *backgroundJobsRunner) Cancel(jobName string) error {
	job, ok := r.jobByName(jobName)
	if !ok || job == nil {
		return fmt.Errorf("unknown background job %q", jobName)
	}
	job.Cancel()
	return nil
}

// Start launches the named job for uid. It returns an error when the job is
// already running, the dependencies required for that job are absent, or the
// name is unknown. The actual work runs in a goroutine.
func (r *backgroundJobsRunner) Start(ctx context.Context, jobName string, uid int64) error {
	switch jobName {
	case backgroundjobs.JobThumbnails:
		return r.startThumbnails(uid)
	case backgroundjobs.JobImageTagEmbeddings:
		return r.startImageTagEmbeddings(uid)
	case backgroundjobs.JobImageAIClassification:
		return r.startImageAIClassification(ctx, uid)
	case backgroundjobs.JobMessageContextEmbeddings:
		return r.startMessageContextEmbeddings(uid)
	case backgroundjobs.JobEmailEmbeddings:
		return r.startEmailEmbeddings(uid)
	}
	return fmt.Errorf("unknown background job %q", jobName)
}

// ── per-job starters ─────────────────────────────────────────────────────────

func (r *backgroundJobsRunner) startThumbnails(uid int64) error {
	if r.pool == nil {
		return fmt.Errorf("thumbnails: database not configured")
	}
	if err := thumbnailsJob.AssertNotRunning(); err != nil {
		return err
	}
	thumbnailsJob.Start()
	thumbnailsJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting thumbnail processing (background scheduler)...",
		"phase": nil, "phase1_scanned": 0, "phase1_updated": 0,
		"phase2_scanned": 0, "phase2_total": 0, "phase2_processed": 0, "phase2_errors": 0,
	})
	thumbnailsJob.Broadcast("status", map[string]any{"status_line": "Starting thumbnail processing (background scheduler)..."})
	go runThumbnailsInProcess(r.pool, thumbnailsJob, false, uid)
	return nil
}

func (r *backgroundJobsRunner) startImageTagEmbeddings(uid int64) error {
	if r.pool == nil {
		return fmt.Errorf("image tag embeddings: database not configured")
	}
	if r.imageSvc == nil {
		return fmt.Errorf("image tag embeddings: image service not configured")
	}
	if r.embeddingSvc == nil || !r.embeddingSvc.IsAvailable() {
		return fmt.Errorf("image tag embeddings: embedding service not available — set LOCALAI_BASE_URL and LOCALAI_EMBEDDING_MODEL")
	}
	if err := imageTagEmbeddingJob.AssertNotRunning(); err != nil {
		return err
	}
	imageTagEmbeddingJob.Start()
	imageTagEmbeddingJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting image tag embedding backfill (background scheduler)...",
		"total": 0, "processed": 0, "embedded": 0, "skipped": 0, "errors": 0,
	})
	imageTagEmbeddingJob.Broadcast("status", map[string]any{"status_line": "Starting image tag embedding backfill (background scheduler)..."})
	go runImageTagEmbeddingBackfill(r.imageSvc, r.pool, imageTagEmbeddingJob, uid, false)
	return nil
}

func (r *backgroundJobsRunner) startImageAIClassification(ctx context.Context, uid int64) error {
	if r.pool == nil {
		return fmt.Errorf("image AI classification: database not configured")
	}
	if r.imageSvc == nil {
		return fmt.Errorf("image AI classification: image service not configured")
	}
	if err := imageAIClassificationJob.AssertNotRunning(); err != nil {
		return err
	}
	// Discover unclassified images for this user. Scope the lookup to uid by
	// threading appctx so the repo's user-scoped filter is applied.
	scopedCtx := context.WithValue(ctx, appctx.ContextKeyUserID, uid)
	ids, err := r.imageSvc.ListImageIDsRequireClassification(scopedCtx)
	if err != nil {
		return fmt.Errorf("image AI classification: list queued items: %w", err)
	}
	imageAIClassificationJob.Start()
	imageAIClassificationJob.UpdateState(map[string]any{
		"status":        "in_progress",
		"status_line":   fmt.Sprintf("Starting AI classification for %d queued image(s) (background scheduler)...", len(ids)),
		"total":         len(ids),
		"processed":     0,
		"classified":    0,
		"errors":        0,
		"error_message": nil,
	})
	imageAIClassificationJob.Broadcast("status", map[string]any{
		"status_line": fmt.Sprintf("Starting AI classification for %d queued image(s) (background scheduler)...", len(ids)),
	})
	go runImageAIClassificationStub(r.imageSvc, imageAIClassificationJob, r.pool, uid, append([]int64(nil), ids...), 0)
	return nil
}

func (r *backgroundJobsRunner) startMessageContextEmbeddings(uid int64) error {
	if r.pool == nil {
		return fmt.Errorf("message context embeddings: database not configured")
	}
	if r.embeddingSvc == nil || !r.embeddingSvc.IsAvailable() {
		return fmt.Errorf("message context embeddings: embedding service not available — set LOCALAI_BASE_URL and LOCALAI_EMBEDDING_MODEL")
	}
	if err := messageContextEmbeddingBackfillJob.AssertNotRunning(); err != nil {
		return err
	}
	messageContextEmbeddingBackfillJob.Start()
	messageContextEmbeddingBackfillJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting message context embedding backfill (background scheduler)...",
		"total": 0, "processed": 0, "embedded": 0, "skipped": 0, "errors": 0,
	})
	messageContextEmbeddingBackfillJob.Broadcast("status", map[string]any{"status_line": "Starting message context embedding backfill (background scheduler)..."})
	go runMessageContextEmbeddingBackfill(r.pool, r.embeddingSvc, messageContextEmbeddingBackfillJob, uid)
	return nil
}

func (r *backgroundJobsRunner) startEmailEmbeddings(uid int64) error {
	if r.pool == nil {
		return fmt.Errorf("email embeddings: database not configured")
	}
	if r.embeddingSvc == nil || !r.embeddingSvc.IsAvailable() {
		return fmt.Errorf("email embeddings: embedding service not available — set LOCALAI_BASE_URL and LOCALAI_EMBEDDING_MODEL")
	}
	if err := emailEmbeddingBackfillJob.AssertNotRunning(); err != nil {
		return err
	}
	emailEmbeddingBackfillJob.Start()
	emailEmbeddingBackfillJob.UpdateState(map[string]any{
		"status": "in_progress", "status_line": "Starting email embedding backfill (background scheduler)...",
		"total": 0, "processed": 0, "embedded": 0, "skipped": 0, "errors": 0,
	})
	emailEmbeddingBackfillJob.Broadcast("status", map[string]any{"status_line": "Starting email embedding backfill (background scheduler)..."})
	go runEmailEmbeddingBackfill(r.pool, r.embeddingSvc, emailEmbeddingBackfillJob, uid)
	return nil
}
