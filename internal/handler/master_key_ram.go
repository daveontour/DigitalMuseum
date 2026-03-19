package handler

import (
	"strings"

	"github.com/daveontour/digitalmuseum/internal/keystore"
)

// resolvePasswordWithRAMMaster returns primary if non-empty; otherwise the in-RAM master key if set.
func resolvePasswordWithRAMMaster(primary string, ram *keystore.MemoryMasterKey) string {
	if s := strings.TrimSpace(primary); s != "" {
		return s
	}
	if ram != nil {
		if p, ok := ram.Get(); ok {
			return p
		}
	}
	return ""
}
