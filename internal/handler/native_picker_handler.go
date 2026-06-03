package handler

import (
	"net/http"

	"github.com/daveontour/aimuseum/internal/filedupe/browse"
	"github.com/go-chi/chi/v5"
)

// NativePickerHandler exposes OS-native file/folder pickers for the SPA.
type NativePickerHandler struct{}

// NewNativePickerHandler creates a NativePickerHandler.
func NewNativePickerHandler() *NativePickerHandler {
	return &NativePickerHandler{}
}

// RegisterRoutes mounts native picker routes (no master-key gate).
func (h *NativePickerHandler) RegisterRoutes(r chi.Router) {
	r.Post("/api/pick-file", h.PickFile)
	r.Post("/api/pick-folder", h.PickFolder)
}

func (h *NativePickerHandler) PickFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path, err := browse.PickFile()
	if err != nil {
		fileDupeWriteJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	fileDupeWriteJSON(w, http.StatusOK, map[string]string{"path": path})
}

func (h *NativePickerHandler) PickFolder(w http.ResponseWriter, r *http.Request) {
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
