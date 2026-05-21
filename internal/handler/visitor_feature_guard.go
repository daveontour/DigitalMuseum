package handler

import (
	"net/http"

	"github.com/daveontour/aimuseum/internal/appctx"
)

const visitorFeatureDenied = "This area is not enabled for your visitor key. Ask the archive owner to grant access under Manage Keys."

// requireVisitorMessagesChat gates the Messages & Chats (iMessage/SMS) APIs only, not /chat/generate or the main AI pane.
func requireVisitorMessagesChat(w http.ResponseWriter, r *http.Request) bool {
	if appctx.VisitorAccessFromCtx(r.Context()).AllowMessagesChat() {
		return true
	}
	writeError(w, http.StatusForbidden, visitorFeatureDenied)
	return false
}

func requireVisitorEmails(w http.ResponseWriter, r *http.Request) bool {
	if appctx.VisitorAccessFromCtx(r.Context()).AllowEmails() {
		return true
	}
	writeError(w, http.StatusForbidden, visitorFeatureDenied)
	return false
}

func requireVisitorContacts(w http.ResponseWriter, r *http.Request) bool {
	if appctx.VisitorAccessFromCtx(r.Context()).AllowContacts() {
		return true
	}
	writeError(w, http.StatusForbidden, visitorFeatureDenied)
	return false
}

func requireVisitorRelationships(w http.ResponseWriter, r *http.Request) bool {
	if appctx.VisitorAccessFromCtx(r.Context()).AllowRelationships() {
		return true
	}
	writeError(w, http.StatusForbidden, visitorFeatureDenied)
	return false
}

func requireVisitorSensitivePrivate(w http.ResponseWriter, r *http.Request) bool {
	if appctx.VisitorAccessFromCtx(r.Context()).AllowSensitivePrivate() {
		return true
	}
	writeError(w, http.StatusForbidden, visitorFeatureDenied)
	return false
}
