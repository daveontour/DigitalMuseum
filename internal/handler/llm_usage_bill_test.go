package handler

import (
	"testing"
	"time"
)

func TestUTCRangeCurrentMonth(t *testing.T) {
	ref := time.Date(2026, 3, 15, 14, 30, 0, 0, time.UTC)
	from, to := UTCRangeCurrentMonth(ref)
	if !from.Equal(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("from: got %v", from)
	}
	if !to.Equal(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("to: got %v", to)
	}
}

func TestUTCRangePreviousMonth(t *testing.T) {
	ref := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	from, to := UTCRangePreviousMonth(ref)
	if !from.Equal(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("from: got %v", from)
	}
	if !to.Equal(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("to: got %v", to)
	}
}
