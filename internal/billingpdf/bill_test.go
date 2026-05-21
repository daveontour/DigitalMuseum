package billingpdf

import (
	"bytes"
	"testing"
	"time"

	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/sqlutil"
)

func TestRenderLLMUsageBill_Minimal(t *testing.T) {
	meta := BillMeta{
		UserID:       1,
		Email:        "a@example.com",
		DisplayName:  "Test User",
		GeneratedAt:  time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC),
		MaxEventRows: repository.DefaultListEventsAllMax,
	}
	sum := repository.LLMUsageSummary{TotalInputTokens: 10, TotalOutputTokens: 20, EventCount: 1}
	events := []repository.LLMUsageEvent{
		{
			ID:           1,
			CreatedAt:    sqlutil.DBTime{Time: meta.GeneratedAt},
			Provider:     "gemini",
			IsVisitor:    false,
			InputTokens:  10,
			OutputTokens: 20,
			Succeeded:    true,
		},
	}
	buckets := []repository.TimeseriesBucket{
		{BucketStart: meta.GeneratedAt, InputTokens: 10, OutputTokens: 20},
	}
	pdf, err := RenderLLMUsageBill(meta, sum, nil, nil, events, buckets)
	if err != nil {
		t.Fatal(err)
	}
	if len(pdf) < 500 {
		t.Fatalf("expected non-trivial PDF size, got %d", len(pdf))
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF")) {
		n := 8
		if len(pdf) < n {
			n = len(pdf)
		}
		t.Fatalf("expected PDF header, got %q", pdf[:n])
	}
}
