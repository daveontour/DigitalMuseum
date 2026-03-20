package handler

import (
	"net/http"
	"strings"

	appcrypto "github.com/daveontour/digitalmuseum/internal/crypto"
	"github.com/daveontour/digitalmuseum/internal/keystore"
)

// resolveMasterPassword returns primary if non-empty; otherwise the session-stored
// master key for this HTTP request, if any.
func resolveMasterPassword(primary string, r *http.Request, store *keystore.SessionMasterStore) string {
	if strings.TrimSpace(primary) != "" {
		return appcrypto.NormalizeKeyringPassword(primary)
	}
	if store != nil && r != nil {
		if p, ok := store.Get(r); ok {
			return p
		}
	}
	return ""
}
