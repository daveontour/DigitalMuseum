package handler

import (
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/repository"
)

// MemoriesHandler serves two nostalgia features built on plain archive queries (no LLM):
//
//   - On This Day — GET /api/on-this-day?date=YYYY-MM-DD: everything from earlier years on that
//     calendar day (date defaults to the server's today; the browser sends its own local date).
//   - Memory Challenge — GET /api/trivia?count=N&types=a,b: multiple-choice questions generated
//     from real records (see repository.TriviaRepo).
//
// Both honour visitor-key permissions the same way the Timeline does: messages/emails only when
// allowed, and face questions (which reveal contact names) only with the contacts permission.
type MemoriesHandler struct {
	timeline *repository.TimelineRepo
	trivia   *repository.TriviaRepo
}

// NewMemoriesHandler creates a MemoriesHandler.
func NewMemoriesHandler(timeline *repository.TimelineRepo, trivia *repository.TriviaRepo) *MemoriesHandler {
	return &MemoriesHandler{timeline: timeline, trivia: trivia}
}

// RegisterRoutes mounts the On This Day and Memory Challenge routes.
func (h *MemoriesHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/on-this-day", h.OnThisDay)
	r.Get("/api/trivia", h.Trivia)
}

// OnThisDay handles GET /api/on-this-day?date=YYYY-MM-DD.
func (h *MemoriesHandler) OnThisDay(w http.ResponseWriter, r *http.Request) {
	day := time.Now()
	if v := strings.TrimSpace(r.URL.Query().Get("date")); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil || t.Year() < 1901 || t.Year() > 2101 {
			writeError(w, http.StatusBadRequest, "invalid date (expected YYYY-MM-DD)")
			return
		}
		day = t
	}
	res, err := h.timeline.OnThisDay(r.Context(), day.Year(), int(day.Month()), day.Day(), timelineSourcesFor(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("error loading memories: %s", err))
		return
	}
	writeJSON(w, res)
}

// Trivia handles GET /api/trivia?count=10&types=photo_year,face_who.
func (h *MemoriesHandler) Trivia(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	va := appctx.VisitorAccessFromCtx(ctx)
	opt := repository.TriviaOptions{
		Messages: va.AllowMessagesChat(),
		Emails:   va.AllowEmails(),
		Contacts: va.AllowContacts(),
	}
	q := r.URL.Query()
	if v := strings.TrimSpace(q.Get("count")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "invalid count")
			return
		}
		opt.Count = n
	}
	if v := strings.TrimSpace(q.Get("types")); v != "" {
		opt.Types = strings.Split(v, ",")
	}
	// Visitors play about the subject in the third person; the owner plays about themself.
	if appctx.IsVisitorFromCtx(ctx) || va.Restricted {
		opt.SubjectName = h.trivia.TriviaSubjectName(ctx)
	}
	res, err := h.trivia.Generate(ctx, opt, rand.New(rand.NewSource(time.Now().UnixNano())))
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("could not build the challenge: %s", err))
		return
	}
	writeJSON(w, res)
}
