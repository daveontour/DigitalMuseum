package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/daveontour/aimuseum/internal/billingpdf"
	"github.com/daveontour/aimuseum/internal/repository"
)

// WriteLLMUsageBillPDF writes a PDF usage statement for userID and optional time range [from, to) (to exclusive in queries).
func WriteLLMUsageBillPDF(w http.ResponseWriter, r *http.Request, userRepo *repository.UserRepo, billing *repository.BillingRepo, userID int64, from, to *time.Time) {
	ctx := r.Context()
	u, err := userRepo.FindByID(ctx, userID)
	if err != nil {
		slog.Error("bill pdf user lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to load user")
		return
	}
	if u == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	sum, byProv, byVis, err := billing.SummaryByUser(ctx, userID, from, to)
	if err != nil {
		slog.Error("bill pdf summary", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to load usage summary")
		return
	}
	events, truncated, err := billing.ListEventsByUserAll(ctx, userID, from, to, repository.DefaultListEventsAllMax)
	if err != nil {
		slog.Error("bill pdf events", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to load usage events")
		return
	}
	buckets, err := billing.TimeseriesByUser5Min(ctx, userID, from, to)
	if err != nil {
		slog.Error("bill pdf timeseries", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to load timeseries")
		return
	}
	if buckets == nil {
		buckets = []repository.TimeseriesBucket{}
	}
	meta := billingpdf.BillMeta{
		UserID:          u.ID,
		Email:           u.Email,
		DisplayName:     u.DisplayName,
		FirstName:       u.FirstName,
		FamilyName:      u.FamilyName,
		PeriodFrom:      from,
		PeriodTo:        to,
		GeneratedAt:     time.Now().UTC(),
		EventsTruncated: truncated,
		MaxEventRows:    repository.DefaultListEventsAllMax,
	}
	pdfBytes, err := billingpdf.RenderLLMUsageBill(meta, sum, byProv, byVis, events, buckets)
	if err != nil {
		slog.Error("bill pdf render", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to generate PDF")
		return
	}
	fname := billPDFFilename(userID, from)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fname))
	w.Header().Set("Content-Length", strconv.Itoa(len(pdfBytes)))
	_, _ = w.Write(pdfBytes)
}

func billPDFFilename(userID int64, from *time.Time) string {
	ts := time.Now().UTC().Format("20060102-150405")
	if from != nil {
		return fmt.Sprintf("llm-usage-user-%d-%s.pdf", userID, from.UTC().Format("2006-01"))
	}
	return fmt.Sprintf("llm-usage-user-%d-%s.pdf", userID, ts)
}

// UTCRangeForCalendarMonth returns [start, end) for the given year/month in UTC (end is exclusive).
func UTCRangeForCalendarMonth(year int, month time.Month) (from, to time.Time) {
	from = time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	to = from.AddDate(0, 1, 0)
	return from, to
}

// UTCRangeCurrentMonth returns [first instant of current UTC month, first instant of next UTC month).
func UTCRangeCurrentMonth(now time.Time) (from, to time.Time) {
	n := now.UTC()
	return UTCRangeForCalendarMonth(n.Year(), n.Month())
}

// UTCRangePreviousMonth returns the UTC calendar month immediately before now's UTC month.
func UTCRangePreviousMonth(now time.Time) (from, to time.Time) {
	n := now.UTC()
	firstThis := time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, time.UTC)
	to = firstThis
	from = firstThis.AddDate(0, -1, 0)
	return from, to
}
