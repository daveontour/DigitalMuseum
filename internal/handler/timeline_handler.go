package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/repository"
)

// TimelineHandler serves the cross-source Timeline and the Ctrl+K quick search.
//
// Messages and emails are included only when the session may see them (visitor keys can be
// restricted — see visitor_feature_guard.go); photos and Facebook posts follow the same
// visibility as their sidebar galleries.
type TimelineHandler struct {
	repo *repository.TimelineRepo
}

// NewTimelineHandler creates a TimelineHandler.
func NewTimelineHandler(repo *repository.TimelineRepo) *TimelineHandler {
	return &TimelineHandler{repo: repo}
}

// RegisterRoutes mounts the timeline and quick-search routes.
func (h *TimelineHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/timeline", h.Summary)
	r.Get("/api/timeline/{year}/{month}", h.Month)
	r.Get("/api/quick-search", h.QuickSearch)
}

func timelineSourcesFor(r *http.Request) repository.TimelineSources {
	va := appctx.VisitorAccessFromCtx(r.Context())
	return repository.TimelineSources{
		Messages: va.AllowMessagesChat(),
		Emails:   va.AllowEmails(),
	}
}

// Summary handles GET /api/timeline → {months: [...]}.
func (h *TimelineHandler) Summary(w http.ResponseWriter, r *http.Request) {
	months, err := h.repo.MonthlyCounts(r.Context(), timelineSourcesFor(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error loading timeline: %s", err))
		return
	}
	writeJSON(w, map[string]any{"months": months})
}

// Month handles GET /api/timeline/{year}/{month}.
func (h *TimelineHandler) Month(w http.ResponseWriter, r *http.Request) {
	year, err1 := strconv.Atoi(chi.URLParam(r, "year"))
	month, err2 := strconv.Atoi(chi.URLParam(r, "month"))
	if err1 != nil || err2 != nil || year < 1900 || year > 2100 || month < 1 || month > 12 {
		writeError(w, http.StatusBadRequest, "invalid year or month")
		return
	}
	detail, err := h.repo.MonthDetail(r.Context(), year, month, timelineSourcesFor(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error loading month: %s", err))
		return
	}
	writeJSON(w, detail)
}

// QuickSearch handles GET /api/quick-search?q=...
func (h *TimelineHandler) QuickSearch(w http.ResponseWriter, r *http.Request) {
	res, err := h.repo.QuickSearch(r.Context(), r.URL.Query().Get("q"), timelineSourcesFor(r))
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(r.Context().Err(), context.Canceled) {
			return // the client typed more and abandoned this request
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("search failed: %s", err))
		return
	}
	writeJSON(w, res)
}
