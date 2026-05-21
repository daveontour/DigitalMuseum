package handler

import (
	"net/http"
	"time"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/go-chi/chi/v5"
)

// BillingExportHandler serves the archive owner's LLM usage PDF (session auth, not admin).
type BillingExportHandler struct {
	userRepo *repository.UserRepo
	billing  *repository.BillingRepo
}

// NewBillingExportHandler creates a BillingExportHandler.
func NewBillingExportHandler(userRepo *repository.UserRepo, billing *repository.BillingRepo) *BillingExportHandler {
	return &BillingExportHandler{userRepo: userRepo, billing: billing}
}

// RegisterRoutes mounts GET /api/llm-usage/me/bill.pdf
func (h *BillingExportHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/llm-usage/me/bill.pdf", h.GetMyBillPDF)
}

// GET /api/llm-usage/me/bill.pdf?period=current|previous
// Calendar months are in UTC. Only the archive owner may download (not visitor sessions).
func (h *BillingExportHandler) GetMyBillPDF(w http.ResponseWriter, r *http.Request) {
	if h.billing == nil || h.billing.PgxPool() == nil {
		writeError(w, http.StatusServiceUnavailable, "billing database not configured")
		return
	}
	ctx := r.Context()
	uid := appctx.UserIDFromCtx(ctx)
	if uid == 0 {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if appctx.IsVisitorFromCtx(ctx) {
		writeError(w, http.StatusForbidden, "billing export is only available to the archive owner")
		return
	}
	period := r.URL.Query().Get("period")
	var fromPtr, toPtr *time.Time
	switch period {
	case "current":
		from, to := UTCRangeCurrentMonth(time.Now())
		fromPtr, toPtr = &from, &to
	case "previous":
		from, to := UTCRangePreviousMonth(time.Now())
		fromPtr, toPtr = &from, &to
	default:
		writeError(w, http.StatusBadRequest, "period must be current or previous")
		return
	}
	WriteLLMUsageBillPDF(w, r, h.userRepo, h.billing, uid, fromPtr, toPtr)
}
