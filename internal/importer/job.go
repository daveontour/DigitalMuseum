// Package importer provides a generic ImportJob for SSE-streamed background import jobs.
package importer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// ImportJob manages state and SSE clients for a single background import job.
type ImportJob struct {
	name      string
	mu        sync.Mutex
	inProgress bool
	cancelled chan struct{}
	progress  map[string]any

	clientsMu sync.Mutex
	clients   []chan string
}

// NewImportJob creates an ImportJob with the given name and initial progress state.
func NewImportJob(name string, initial map[string]any) *ImportJob {
	prog := make(map[string]any, len(initial))
	for k, v := range initial {
		prog[k] = v
	}
	return &ImportJob{
		name:      name,
		progress:  prog,
		cancelled: make(chan struct{}),
	}
}

// GetState returns a snapshot of the current progress map.
func (j *ImportJob) GetState() map[string]any {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make(map[string]any, len(j.progress))
	for k, v := range j.progress {
		out[k] = v
	}
	return out
}

// UpdateState merges updates into progress (only updates keys that already exist).
func (j *ImportJob) UpdateState(updates map[string]any) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for k, v := range updates {
		if _, ok := j.progress[k]; ok {
			j.progress[k] = v
		}
	}
}

// Broadcast sends a JSON SSE event to all registered clients.
func (j *ImportJob) Broadcast(eventType string, data map[string]any) {
	payload := map[string]any{"type": eventType, "data": data}
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	msg := fmt.Sprintf("data: %s\n\n", b)
	j.clientsMu.Lock()
	defer j.clientsMu.Unlock()
	for _, ch := range j.clients {
		select {
		case ch <- msg:
		default: // drop if full
		}
	}
}

// InProgress returns true if the job is currently running.
func (j *ImportJob) InProgress() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.inProgress
}

// IsCancelled returns true if cancellation was requested.
func (j *ImportJob) IsCancelled() bool {
	select {
	case <-j.cancelled:
		return true
	default:
		return false
	}
}

// Cancelled returns the cancellation channel (for select in goroutines).
func (j *ImportJob) Cancelled() <-chan struct{} {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.cancelled
}

// AssertNotRunning returns an error if the job is already in progress.
func (j *ImportJob) AssertNotRunning() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.inProgress {
		return fmt.Errorf("%s is already in progress", j.name)
	}
	return nil
}

// Start marks the job as running and resets the cancellation signal.
func (j *ImportJob) Start() {
	j.mu.Lock()
	j.inProgress = true
	j.cancelled = make(chan struct{})
	j.mu.Unlock()
}

// Finish marks the job as complete.
func (j *ImportJob) Finish() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.inProgress = false
}

// Cancel signals cancellation. Returns a status message.
func (j *ImportJob) Cancel() map[string]any {
	j.mu.Lock()
	if !j.inProgress {
		j.mu.Unlock()
		return map[string]any{"message": fmt.Sprintf("No %s in progress", j.name)}
	}
	cancelled := j.cancelled
	j.mu.Unlock()
	select {
	case <-cancelled:
	default:
		close(cancelled)
	}
	return map[string]any{"message": fmt.Sprintf("%s cancellation requested", j.name)}
}

// Status returns the full status including in_progress and cancelled flags.
func (j *ImportJob) Status() map[string]any {
	state := j.GetState()
	j.mu.Lock()
	inProgress := j.inProgress
	j.mu.Unlock()
	state["in_progress"] = inProgress
	state["cancelled"] = j.IsCancelled()
	return state
}

// ServeSSE registers the HTTP connection as an SSE client and blocks until disconnect.
func (j *ImportJob) ServeSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Register client
	ch := make(chan string, 64)
	j.clientsMu.Lock()
	j.clients = append(j.clients, ch)
	j.clientsMu.Unlock()

	defer func() {
		j.clientsMu.Lock()
		for i, c := range j.clients {
			if c == ch {
				j.clients = append(j.clients[:i], j.clients[i+1:]...)
				break
			}
		}
		j.clientsMu.Unlock()
	}()

	// Send current state immediately
	initial := j.GetState()
	payload, _ := json.Marshal(map[string]any{"type": "progress", "data": initial})
	fmt.Fprintf(w, "data: %s\n\n", payload)
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprint(w, msg)
			flusher.Flush()
		case <-ticker.C:
			// keepalive comment
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
