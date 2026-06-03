package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/daveontour/aimuseum/internal/filedupe/browse"
	"github.com/daveontour/aimuseum/internal/filedupe/delete"
	"github.com/daveontour/aimuseum/internal/filedupe/paths"
	"github.com/daveontour/aimuseum/internal/filedupe/preview"
	"github.com/daveontour/aimuseum/internal/filedupe/scanner"
	"github.com/go-chi/chi/v5"
)

// FileDupeHandler exposes duplicate-file comparison tools under /api/filedupe/*.
type FileDupeHandler struct {
	mu          sync.Mutex
	lastEventAt time.Time
}

// NewFileDupeHandler creates a FileDupeHandler.
func NewFileDupeHandler() *FileDupeHandler {
	return &FileDupeHandler{}
}

// RegisterRoutes mounts duplicate-file tool routes (no master-key gate).
func (h *FileDupeHandler) RegisterRoutes(r chi.Router) {
	r.Post("/api/filedupe/browse", h.BrowseFolder)
	r.Post("/api/filedupe/scan", h.Scan)
	r.Post("/api/filedupe/delete", h.DeleteFiles)
	r.Get("/api/filedupe/preview", h.PreviewImage)
	r.Post("/api/filedupe/open-path", h.OpenPath)
}

type fileDupeScanRequest struct {
	Dir1        string   `json:"dir1"`
	Dir2        string   `json:"dir2"`
	Dir1Exclude []string `json:"dir1Exclude"`
	Dir2Exclude []string `json:"dir2Exclude"`
}

type fileDupeDeleteRequest struct {
	Dir1  string   `json:"dir1"`
	Dir2  string   `json:"dir2"`
	Paths []string `json:"paths"`
}

type fileDupeOpenPathRequest struct {
	Path string `json:"path"`
	Dir1 string `json:"dir1"`
	Dir2 string `json:"dir2"`
}

type fileDupeSSEEvent struct {
	Type         string          `json:"type"`
	Phase        string          `json:"phase,omitempty"`
	FilesScanned int             `json:"filesScanned,omitempty"`
	CurrentPath  string          `json:"currentPath,omitempty"`
	Message      string          `json:"message,omitempty"`
	Result       *scanner.Result `json:"result,omitempty"`
	Error        string          `json:"error,omitempty"`
}

func (h *FileDupeHandler) BrowseFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path, err := browse.PickFolder()
	if err != nil {
		fileDupeWriteJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	fileDupeWriteJSON(w, http.StatusOK, map[string]string{"path": path})
}

func (h *FileDupeHandler) OpenPath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req fileDupeOpenPathRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fileDupeWriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := paths.ValidateUnderRoots(req.Path, []string{req.Dir1, req.Dir2}); err != nil {
		fileDupeWriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := browse.OpenInExplorer(req.Path); err != nil {
		fileDupeWriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	fileDupeWriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *FileDupeHandler) DeleteFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req fileDupeDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fileDupeWriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	result, err := delete.Files([]string{req.Dir1, req.Dir2}, req.Paths)
	if err != nil && len(result.Deleted) == 0 {
		payload := map[string]any{
			"error":  err.Error(),
			"failed": result.Failed,
		}
		if result.Hint != "" {
			payload["hint"] = result.Hint
		}
		fileDupeWriteJSON(w, http.StatusBadRequest, payload)
		return
	}

	status := http.StatusOK
	payload := map[string]any{
		"deleted": result.Deleted,
	}
	if len(result.Failed) > 0 {
		payload["failed"] = result.Failed
		payload["warning"] = "some files could not be deleted"
	}
	if result.Hint != "" {
		payload["hint"] = result.Hint
	}
	if err != nil {
		payload["warning"] = err.Error()
	}

	fileDupeWriteJSON(w, status, payload)
}

func (h *FileDupeHandler) PreviewImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := r.URL.Query().Get("path")
	dir1 := r.URL.Query().Get("dir1")
	dir2 := r.URL.Query().Get("dir2")

	result, err := preview.Open([]string{dir1, dir2}, path)
	if err != nil {
		fileDupeWriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	defer result.Cleanup()

	w.Header().Set("Content-Type", result.ContentType)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = io.Copy(w, result.Reader)
}

func (h *FileDupeHandler) Scan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req fileDupeScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fileDupeWriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	h.resetProgressThrottle()
	start := time.Now()
	h.sendSSE(w, flusher, fileDupeSSEEvent{
		Type:    "progress",
		Phase:   "starting",
		Message: "Starting scan",
	})

	var lastPhase string
	result, err := scanner.Scan(req.Dir1, req.Dir2, scanner.ScanOptions{
		Dir1ExcludePatterns: req.Dir1Exclude,
		Dir2ExcludePatterns: req.Dir2Exclude,
	}, func(update scanner.ProgressUpdate) {
		if update.Message == "" && update.Phase == lastPhase && !h.shouldEmitProgress() {
			return
		}
		lastPhase = update.Phase
		h.sendSSE(w, flusher, fileDupeSSEEvent{
			Type:         "progress",
			Phase:        update.Phase,
			FilesScanned: update.FilesScanned,
			CurrentPath:  update.CurrentPath,
			Message:      update.Message,
		})
	})

	if err != nil {
		h.sendSSE(w, flusher, fileDupeSSEEvent{
			Type:  "error",
			Error: err.Error(),
		})
		return
	}

	result.ScanDuration = time.Since(start).Round(time.Millisecond).String()
	h.sendSSE(w, flusher, fileDupeSSEEvent{
		Type:    "complete",
		Message: fmt.Sprintf("Scan complete in %s", result.ScanDuration),
		Result:  &result,
	})
}

func (h *FileDupeHandler) resetProgressThrottle() {
	h.mu.Lock()
	h.lastEventAt = time.Time{}
	h.mu.Unlock()
}

func (h *FileDupeHandler) shouldEmitProgress() bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	if now.Sub(h.lastEventAt) < 100*time.Millisecond {
		return false
	}
	h.lastEventAt = now
	return true
}

func (h *FileDupeHandler) sendSSE(w http.ResponseWriter, flusher http.Flusher, event fileDupeSSEEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

func fileDupeWriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
