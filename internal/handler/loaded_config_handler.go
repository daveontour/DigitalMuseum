package handler

import (
	"net/http"

	"github.com/daveontour/aimuseum/internal/config"
)

// LoadedConfigJSON serves GET /api/loaded-config — localhost only.
// Returns known configuration keys with effective values and provenance
// (.env file path, process environment, or built-in default).
func LoadedConfigJSON(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !IsLocalhost(r) {
			writeError(w, http.StatusForbidden, "localhost only")
			return
		}
		writeJSON(w, config.SnapshotLoadedConfig(cfg))
	}
}
