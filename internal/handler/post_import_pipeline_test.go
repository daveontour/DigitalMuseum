package handler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	backgroundjobs "github.com/daveontour/aimuseum/internal/service/background_jobs"
)

// fakePostImportRunner starts the real job singletons and finishes them shortly after with
// the configured outcome, without touching a database.
type fakePostImportRunner struct {
	mu       sync.Mutex
	started  []string
	failWith map[string]error  // Start returns this error
	outcome  map[string]string // final "status" (default "completed")
	hold     time.Duration
}

func (f *fakePostImportRunner) Definitions() []backgroundjobs.JobDef { return nil }
func (f *fakePostImportRunner) Cancel(string) error                  { return nil }
func (f *fakePostImportRunner) Status(string) (bool, string)         { return false, "" }
func (f *fakePostImportRunner) IdleOutcome(string) (string, string)  { return "completed", "" }

func (f *fakePostImportRunner) Start(_ context.Context, name string, _ int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.failWith[name]; err != nil {
		return err
	}
	job, ok := backgroundJobSingleton(name)
	if !ok {
		return errors.New("unknown")
	}
	f.started = append(f.started, name)
	job.Start()
	job.UpdateState(map[string]any{"status": "in_progress", "status_line": "working"})
	final := f.outcome[name]
	if final == "" {
		final = "completed"
	}
	hold := f.hold
	go func() {
		deadline := time.Now().Add(hold)
		for time.Now().Before(deadline) && !job.IsCancelled() {
			time.Sleep(time.Millisecond)
		}
		if job.IsCancelled() {
			final = "cancelled"
		}
		job.UpdateState(map[string]any{"status": final, "status_line": name + " " + final})
		job.Finish()
	}()
	return nil
}

func newTestPipeline(r backgroundjobs.JobRunner) *postImportPipeline {
	return &postImportPipeline{
		runner:       r,
		pending:      map[int64]postImportKind{},
		lastByUID:    map[int64]*postImportRun{},
		pollInterval: time.Millisecond,
		stepTimeout:  5 * time.Second,
	}
}

func TestPostImportPlan(t *testing.T) {
	keys := func(steps []postImportStep) []string {
		out := make([]string, len(steps))
		for i, s := range steps {
			out[i] = s.Key
		}
		return out
	}
	photos := keys(postImportPlan(postImportPhotos))
	want := []string{
		backgroundjobs.JobThumbnails, backgroundjobs.JobImageTagEmbeddings,
		backgroundjobs.JobFaceDetection, backgroundjobs.JobFaceClustering, backgroundjobs.JobFaceSuggestions,
	}
	if len(photos) != len(want) {
		t.Fatalf("photos plan = %v, want %v", photos, want)
	}
	for i := range want {
		if photos[i] != want[i] {
			t.Fatalf("photos plan = %v, want %v", photos, want)
		}
	}
	all := keys(postImportPlan(postImportMessages | postImportEmails | postImportPhotos))
	if all[0] != postImportStepContacts {
		t.Fatalf("contacts should run first, got %v", all)
	}
	for _, s := range all {
		if s == backgroundjobs.JobImageAIClassification {
			t.Fatal("paid AI classification must never be part of the automatic plan")
		}
	}
}

func TestPostImportExecute_RunsSkipsAndRecordsErrors(t *testing.T) {
	r := &fakePostImportRunner{
		failWith: map[string]error{backgroundjobs.JobImageTagEmbeddings: errors.New("image tag embeddings: embedding service not available — set LOCALAI_BASE_URL")},
		outcome:  map[string]string{backgroundjobs.JobFaceDetection: "error"},
		hold:     5 * time.Millisecond,
	}
	p := newTestPipeline(r)
	run := &postImportRun{UserID: 2, Steps: postImportPlan(postImportPhotos), Running: true}
	p.execute(run)

	got := map[string]string{}
	for _, s := range run.Steps {
		got[s.Key] = s.Status
	}
	if got[backgroundjobs.JobThumbnails] != "done" ||
		got[backgroundjobs.JobImageTagEmbeddings] != "skipped" ||
		got[backgroundjobs.JobFaceDetection] != "error" ||
		got[backgroundjobs.JobFaceClustering] != "done" ||
		got[backgroundjobs.JobFaceSuggestions] != "done" {
		t.Fatalf("unexpected step statuses: %+v", run.Steps)
	}
	if run.Running || run.FinishedAt == nil {
		t.Fatal("run should be marked finished")
	}
}

func TestPostImportCancelStopsRemainingSteps(t *testing.T) {
	r := &fakePostImportRunner{hold: 2 * time.Second}
	p := newTestPipeline(r)
	run := &postImportRun{UserID: 2, Steps: postImportPlan(postImportPhotos), Running: true}
	p.current = run
	done := make(chan struct{})
	go func() { p.execute(run); close(done) }()

	time.Sleep(20 * time.Millisecond)
	if !p.cancel(2) {
		t.Fatal("cancel should report a running pass")
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("pipeline did not stop after cancel")
	}
	for _, s := range run.Steps {
		if s.Status != "cancelled" {
			t.Fatalf("every step should be cancelled, got %+v", run.Steps)
		}
	}
	if !run.Cancelled {
		t.Fatal("run should be flagged cancelled")
	}
}
