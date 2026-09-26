package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/service"
)

// SetOpenRouterKey supplies the archive OpenRouter key used by the personal-score job.
func (h *EmailHandler) SetOpenRouterKey(fn func(context.Context, *http.Request) string) {
	h.openRouterKey = fn
}

type personalScoreStartRequest struct {
	RescoreAll bool `json:"rescore_all"`
}

// StartPersonalScore handles POST /emails/personal-score/start.
func (h *EmailHandler) StartPersonalScore(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	var req personalScoreStartRequest
	if r.Body != nil {
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	apiKey := ""
	if h.openRouterKey != nil {
		apiKey = h.openRouterKey(r.Context(), r)
	}
	ev, err := h.svc.StartPersonalScore(appctx.UserIDFromCtx(r.Context()), apiKey, req.RescoreAll)
	if errors.Is(err, service.ErrPersonalScoreRunning) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, ev)
}

// PersonalScoreStatus handles GET /emails/personal-score/status.
func (h *EmailHandler) PersonalScoreStatus(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	writeJSON(w, h.svc.PersonalScoreStatus(appctx.UserIDFromCtx(r.Context())))
}

// PersonalScoreStream handles GET /emails/personal-score/stream.
// Closing the stream does not stop the job.
func (h *EmailHandler) PersonalScoreStream(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, unsub := h.svc.SubscribePersonalScore(appctx.UserIDFromCtx(r.Context()))
	defer unsub()
	if ch == nil {
		writePersonalScoreSSE(w, flusher, h.svc.PersonalScoreStatus(appctx.UserIDFromCtx(r.Context())))
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			writePersonalScoreSSE(w, flusher, ev)
			if ev.Type == "done" {
				return
			}
		}
	}
}

// CancelPersonalScore handles POST /emails/personal-score/cancel.
func (h *EmailHandler) CancelPersonalScore(w http.ResponseWriter, r *http.Request) {
	if !RequireOwnerMasterUnlock(w, r, h.sessionStore) {
		return
	}
	h.svc.CancelPersonalScore(appctx.UserIDFromCtx(r.Context()))
	writeJSON(w, h.svc.PersonalScoreStatus(appctx.UserIDFromCtx(r.Context())))
}

func writePersonalScoreSSE(w http.ResponseWriter, flusher http.Flusher, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", b)
	flusher.Flush()
}
