// Package backgroundjobs provides a small per-user scheduler for the named
// maintenance jobs surfaced in the Configuration > Background Jobs panel.
//
// The scheduler does not own job execution. It delegates to a JobRunner
// (typically implemented by package handler) that wraps the existing
// *importer.ImportJob singletons. Persisted state lives in the
// background_jobs table.
package backgroundjobs

import "context"

// JobDef is a static description of a registered job, used by the UI for
// labelling and by the scheduler to seed default rows.
type JobDef struct {
	Name                   string `json:"name"`
	Title                  string `json:"title"`
	Description            string `json:"description"`
	DefaultIntervalSeconds int64  `json:"default_interval_seconds"`
}

// JobRunner is the contract the scheduler and HTTP handler call to actually
// run / cancel / observe a registered job. Implementations live in
// package handler.
type JobRunner interface {
	// Definitions lists every job recognised by this runner. The scheduler
	// uses this to seed background_jobs rows and the UI uses it for labels.
	Definitions() []JobDef

	// Start launches the named job for uid. It returns an error if the job
	// is unknown or already running. Implementations are non-blocking — they
	// kick off a goroutine and return.
	Start(ctx context.Context, jobName string, uid int64) error

	// Cancel signals cancellation of the named job (no-op if idle).
	Cancel(jobName string) error

	// Status reports whether the named job is currently running and the most
	// recent status_line for display in the UI.
	Status(jobName string) (inProgress bool, statusLine string)

	// IdleOutcome returns the result/message to persist when a DB row is still
	// marked running but the in-process worker is idle. Only valid when Status
	// reports inProgress=false.
	IdleOutcome(jobName string) (result, message string)
}
