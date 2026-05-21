package backgroundjobs

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/daveontour/aimuseum/internal/model"
	"github.com/daveontour/aimuseum/internal/repository"
)

// Scheduler periodically inspects background_jobs rows and starts jobs that
// are due, then watches running jobs to record their outcome and (optionally)
// requeue them for the next interval.
//
// One Scheduler runs per process; it is started from cmd/server/main.go.
type Scheduler struct {
	repo   *repository.BackgroundJobRepo
	runner JobRunner
	tick   time.Duration

	// runningMu guards `running`.
	runningMu sync.Mutex
	// running tracks rows the scheduler has marked 'running' so it can detect
	// the transition back to idle even though the underlying singleton job
	// has no notion of which user kicked it off.
	running map[runKey]struct{}
}

type runKey struct {
	userID  int64
	jobName string
}

// NewScheduler wires a Scheduler. tick controls how often it polls the table;
// 30s is reasonable for maintenance jobs.
func NewScheduler(repo *repository.BackgroundJobRepo, runner JobRunner, tick time.Duration) *Scheduler {
	if tick <= 0 {
		tick = 30 * time.Second
	}
	return &Scheduler{
		repo:    repo,
		runner:  runner,
		tick:    tick,
		running: make(map[runKey]struct{}),
	}
}

// Run blocks until ctx is cancelled, ticking every Scheduler.tick. The first
// pass runs immediately so auto_start jobs fire promptly after server boot.
func (s *Scheduler) Run(ctx context.Context) {
	if s == nil || s.repo == nil || s.runner == nil {
		return
	}
	slog.Info("background jobs scheduler started", "tick", s.tick)
	s.tickOnce(ctx)
	t := time.NewTicker(s.tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("background jobs scheduler stopped")
			return
		case <-t.C:
			s.tickOnce(ctx)
		}
	}
}

// tickOnce performs one scheduling pass.
func (s *Scheduler) tickOnce(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("background jobs scheduler panic recovered", "panic", r)
		}
	}()
	rows, err := s.repo.AllRows(ctx)
	if err != nil {
		slog.Warn("background jobs scheduler: list rows failed", "err", err)
		return
	}
	now := time.Now().UTC()
	for _, row := range rows {
		if row == nil || row.UserID == nil {
			continue
		}
		s.evaluateRow(ctx, row, now)
	}
}

// evaluateRow handles one (user, job) row per tick.
func (s *Scheduler) evaluateRow(ctx context.Context, row *model.BackgroundJob, now time.Time) {
	uid := *row.UserID
	key := runKey{userID: uid, jobName: row.JobName}

	// 1) If we previously marked this row as running, check whether the
	//    underlying singleton job is still in progress; record the outcome
	//    when it transitions back to idle.
	s.runningMu.Lock()
	_, weStartedIt := s.running[key]
	s.runningMu.Unlock()

	if weStartedIt {
		inProgress, statusLine := s.runner.Status(row.JobName)
		if inProgress {
			return
		}
		// Underlying job finished — clear our tracker and record outcome.
		s.runningMu.Lock()
		delete(s.running, key)
		s.runningMu.Unlock()

		var nextDue *time.Time
		if row.RestartOnComplete {
			interval := row.IntervalSeconds
			if interval <= 0 {
				interval = 3600
			}
			t := now.Add(time.Duration(interval) * time.Second)
			nextDue = &t
		}
		result := "completed"
		if statusLine == "" {
			statusLine = "completed"
		}
		if err := s.repo.MarkCompleted(ctx, uid, row.JobName, result, statusLine, nextDue); err != nil {
			slog.Warn("background jobs: mark completed failed", "job", row.JobName, "err", err)
		}
		return
	}

	// 2) Otherwise, decide whether to start the job now.
	if !row.AutoStart {
		return
	}
	due := !row.NextDueAt.Valid || !row.NextDueAt.Time.After(now)
	if !due {
		return
	}
	// Skip if the underlying singleton is busy (e.g. another user's run).
	if inProgress, _ := s.runner.Status(row.JobName); inProgress {
		return
	}
	if err := s.runner.Start(ctx, row.JobName, uid); err != nil {
		slog.Warn("background jobs: start failed", "job", row.JobName, "uid", uid, "err", err)
		// Record a non-running result so the UI shows the failure and we do
		// not loop trying every tick — push next attempt out by interval.
		interval := row.IntervalSeconds
		if interval <= 0 {
			interval = 3600
		}
		t := now.Add(time.Duration(interval) * time.Second)
		_ = s.repo.MarkCompleted(ctx, uid, row.JobName, "error", err.Error(), &t)
		return
	}
	if err := s.repo.MarkRunning(ctx, uid, row.JobName); err != nil {
		slog.Warn("background jobs: mark running failed", "job", row.JobName, "err", err)
	}
	s.runningMu.Lock()
	s.running[key] = struct{}{}
	s.runningMu.Unlock()
}

// TrackManualStart records that a manually-started run (via the HTTP handler)
// should be observed by the scheduler so it can record completion / reschedule.
// Call this from the handler immediately after a successful Runner.Start.
func (s *Scheduler) TrackManualStart(uid int64, jobName string) {
	if s == nil {
		return
	}
	s.runningMu.Lock()
	s.running[runKey{userID: uid, jobName: jobName}] = struct{}{}
	s.runningMu.Unlock()
}
