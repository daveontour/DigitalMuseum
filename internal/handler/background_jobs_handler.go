package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
	backgroundjobs "github.com/daveontour/aimuseum/internal/service/background_jobs"
	"github.com/go-chi/chi/v5"
)

// BackgroundJobsHandler exposes /api/background-jobs/* endpoints used by the
// Configuration > Background Jobs panel to inspect and configure the per-user
// scheduler state and to trigger manual start/stop of registered jobs.
type BackgroundJobsHandler struct {
	repo      *repository.BackgroundJobRepo
	runner    backgroundjobs.JobRunner
	scheduler *backgroundjobs.Scheduler
}

// NewBackgroundJobsHandler creates a handler. scheduler may be nil — when set,
// manually-started runs are tracked so the scheduler records their outcome.
func NewBackgroundJobsHandler(
	repo *repository.BackgroundJobRepo,
	runner backgroundjobs.JobRunner,
	scheduler *backgroundjobs.Scheduler,
) *BackgroundJobsHandler {
	return &BackgroundJobsHandler{repo: repo, runner: runner, scheduler: scheduler}
}

// RegisterRoutes mounts the background-jobs HTTP API.
func (h *BackgroundJobsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/background-jobs", h.List)
	r.Put("/api/background-jobs/{job_name}", h.Update)
	r.Post("/api/background-jobs/{job_name}/start", h.Start)
	r.Post("/api/background-jobs/{job_name}/stop", h.Stop)
}

// jobNameFromURL extracts the {job_name} URL parameter and validates it
// against the runner's known names.
func (h *BackgroundJobsHandler) jobNameFromURL(w http.ResponseWriter, r *http.Request) (string, bool) {
	name := strings.TrimSpace(chi.URLParam(r, "job_name"))
	if name == "" {
		writeError(w, http.StatusBadRequest, "job_name is required")
		return "", false
	}
	for _, def := range h.runner.Definitions() {
		if def.Name == name {
			return name, true
		}
	}
	writeError(w, http.StatusNotFound, fmt.Sprintf("unknown job %q", name))
	return "", false
}

// jobView builds the JSON shape returned for a single (definition, row) pair.
func (h *BackgroundJobsHandler) jobView(def backgroundjobs.JobDef, row *model.BackgroundJob) map[string]any {
	out := map[string]any{
		"name":                     def.Name,
		"title":                    def.Title,
		"description":              def.Description,
		"default_interval_seconds": def.DefaultIntervalSeconds,
		"auto_start":               false,
		"restart_on_complete":      false,
		"interval_seconds":         def.DefaultIntervalSeconds,
		"last_run_at":              nil,
		"last_run_result":          nil,
		"last_run_message":         nil,
		"next_due_at":              nil,
	}
	if row != nil {
		out["auto_start"] = row.AutoStart
		out["restart_on_complete"] = row.RestartOnComplete
		out["interval_seconds"] = row.IntervalSeconds
		if row.LastRunAt.Valid {
			out["last_run_at"] = row.LastRunAt.Time.UTC().Format("2006-01-02T15:04:05Z")
		}
		out["last_run_result"] = row.LastRunResult
		out["last_run_message"] = row.LastRunMessage
		if row.NextDueAt.Valid {
			out["next_due_at"] = row.NextDueAt.Time.UTC().Format("2006-01-02T15:04:05Z")
		}
	}
	inProgress, statusLine := h.runner.Status(def.Name)
	out["in_progress"] = inProgress
	out["status_line"] = statusLine
	return out
}

func (h *BackgroundJobsHandler) reconcileStaleRunningRow(ctx context.Context, uid int64, jobName string, row *model.BackgroundJob) *model.BackgroundJob {
	if row == nil || row.LastRunResult == nil || *row.LastRunResult != "running" {
		return row
	}
	inProgress, _ := h.runner.Status(jobName)
	if inProgress {
		return row
	}
	result, msg := h.runner.IdleOutcome(jobName)
	if err := h.repo.MarkCompleted(ctx, uid, jobName, result, msg, nil); err != nil {
		return row
	}
	updated, err := h.repo.GetByName(ctx, jobName)
	if err != nil || updated == nil {
		return row
	}
	return updated
}

// List returns the per-user state of every registered background job. Rows are
// auto-seeded the first time the panel is opened.
func (h *BackgroundJobsHandler) List(w http.ResponseWriter, r *http.Request) {
	uid := appctx.UserIDFromCtx(r.Context())
	if uid == 0 {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	defs := h.runner.Definitions()
	for _, d := range defs {
		if _, err := h.repo.EnsureRow(r.Context(), d.Name, d.DefaultIntervalSeconds); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("ensure row: %s", err))
			return
		}
	}
	rows, err := h.repo.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("list background jobs: %s", err))
		return
	}
	byName := make(map[string]*model.BackgroundJob, len(rows))
	for _, row := range rows {
		byName[row.JobName] = row
	}
	out := make([]map[string]any, 0, len(defs))
	for _, d := range defs {
		row := byName[d.Name]
		row = h.reconcileStaleRunningRow(r.Context(), uid, d.Name, row)
		out = append(out, h.jobView(d, row))
	}
	writeJSON(w, map[string]any{"jobs": out})
}

// Update upserts the auto_start / restart_on_complete / interval_seconds
// values for the named job. Other tracking fields are left untouched.
func (h *BackgroundJobsHandler) Update(w http.ResponseWriter, r *http.Request) {
	uid := appctx.UserIDFromCtx(r.Context())
	if uid == 0 {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	jobName, ok := h.jobNameFromURL(w, r)
	if !ok {
		return
	}
	var req struct {
		AutoStart         *bool  `json:"auto_start"`
		RestartOnComplete *bool  `json:"restart_on_complete"`
		IntervalSeconds   *int64 `json:"interval_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	defaultInterval := int64(3600)
	for _, def := range h.runner.Definitions() {
		if def.Name == jobName {
			defaultInterval = def.DefaultIntervalSeconds
			break
		}
	}
	existing, err := h.repo.EnsureRow(r.Context(), jobName, defaultInterval)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("ensure row: %s", err))
		return
	}
	autoStart := false
	restartOnComplete := false
	intervalSeconds := defaultInterval
	if existing != nil {
		autoStart = existing.AutoStart
		restartOnComplete = existing.RestartOnComplete
		intervalSeconds = existing.IntervalSeconds
	}
	if req.AutoStart != nil {
		autoStart = *req.AutoStart
	}
	if req.RestartOnComplete != nil {
		restartOnComplete = *req.RestartOnComplete
	}
	if req.IntervalSeconds != nil {
		v := *req.IntervalSeconds
		if v < 30 {
			v = 30
		}
		intervalSeconds = v
	}
	row, err := h.repo.Upsert(r.Context(), jobName, autoStart, restartOnComplete, intervalSeconds)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("update background job: %s", err))
		return
	}
	def := backgroundjobs.JobDef{Name: jobName, DefaultIntervalSeconds: defaultInterval}
	for _, d := range h.runner.Definitions() {
		if d.Name == jobName {
			def = d
			break
		}
	}
	writeJSON(w, h.jobView(def, row))
}

// Start kicks off the named job for the calling user.
func (h *BackgroundJobsHandler) Start(w http.ResponseWriter, r *http.Request) {
	uid := appctx.UserIDFromCtx(r.Context())
	if uid == 0 {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	jobName, ok := h.jobNameFromURL(w, r)
	if !ok {
		return
	}
	// Ensure a row exists so MarkRunning has something to update and the
	// scheduler can later record completion.
	defaultInterval := int64(3600)
	for _, def := range h.runner.Definitions() {
		if def.Name == jobName {
			defaultInterval = def.DefaultIntervalSeconds
			break
		}
	}
	if _, err := h.repo.EnsureRow(r.Context(), jobName, defaultInterval); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("ensure row: %s", err))
		return
	}
	if err := h.runner.Start(r.Context(), jobName, uid); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.repo.MarkRunning(r.Context(), uid, jobName); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("mark running: %s", err))
		return
	}
	if h.scheduler != nil {
		h.scheduler.TrackManualStart(uid, jobName)
	}
	writeJSON(w, map[string]any{"status": "started", "job_name": jobName})
}

// Stop cancels the named job (no-op if idle) and clears auto_start so the
// scheduler does not relaunch it on the next tick.
func (h *BackgroundJobsHandler) Stop(w http.ResponseWriter, r *http.Request) {
	uid := appctx.UserIDFromCtx(r.Context())
	if uid == 0 {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	jobName, ok := h.jobNameFromURL(w, r)
	if !ok {
		return
	}
	if err := h.runner.Cancel(jobName); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Best-effort: clear auto_start so it does not relaunch immediately.
	_ = h.repo.SetAutoStart(r.Context(), jobName, false)
	writeJSON(w, map[string]any{"status": "cancelling", "job_name": jobName})
}
