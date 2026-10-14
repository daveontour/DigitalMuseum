package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

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

// timelineFilterFor reads the optional ?person=<contact id>&q=<keywords>&tag=<photo tag> filter.
// A person filter needs the same visitor permission as the Contacts API; ok=false means an error
// response has already been written.
func timelineFilterFor(w http.ResponseWriter, r *http.Request) (repository.TimelineFilter, bool) {
	q := r.URL.Query()
	f := repository.TimelineFilter{
		Keywords: repository.ParseTimelineKeywords(q.Get("q")),
		Tag:      strings.TrimSpace(q.Get("tag")),
	}
	if v := strings.TrimSpace(q.Get("person")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "invalid person")
			return f, false
		}
		if !requireVisitorContacts(w, r) {
			return f, false
		}
		f.PersonID = id
	}
	return f, true
}

func writeTimelineError(w http.ResponseWriter, what string, err error) {
	if errors.Is(err, repository.ErrTimelinePersonNotFound) {
		writeError(w, http.StatusNotFound, "That person isn't in this archive's contacts.")
		return
	}
	writeError(w, http.StatusInternalServerError, fmt.Sprintf("error loading %s: %s", what, err))
}

// Summary handles GET /api/timeline → {months: [...], filtered, excluded}.
func (h *TimelineHandler) Summary(w http.ResponseWriter, r *http.Request) {
	f, ok := timelineFilterFor(w, r)
	if !ok {
		return
	}
	months, excluded, err := h.repo.MonthlyCounts(r.Context(), timelineSourcesFor(r), f)
	if err != nil {
		writeTimelineError(w, "timeline", err)
		return
	}
	writeJSON(w, map[string]any{"months": months, "filtered": f.Active(), "excluded": excluded})
}

// Month handles GET /api/timeline/{year}/{month} (same optional filter as Summary).
func (h *TimelineHandler) Month(w http.ResponseWriter, r *http.Request) {
	year, err1 := strconv.Atoi(chi.URLParam(r, "year"))
	month, err2 := strconv.Atoi(chi.URLParam(r, "month"))
	if err1 != nil || err2 != nil || year < 1900 || year > 2100 || month < 1 || month > 12 {
		writeError(w, http.StatusBadRequest, "invalid year or month")
		return
	}
	f, ok := timelineFilterFor(w, r)
	if !ok {
		return
	}
	detail, err := h.repo.MonthDetail(r.Context(), year, month, timelineSourcesFor(r), f)
	if err != nil {
		writeTimelineError(w, "month", err)
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
