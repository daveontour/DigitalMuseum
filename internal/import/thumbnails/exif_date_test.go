package thumbnails

import (
	"testing"
	"time"
)

func TestParseExifDateTaken(t *testing.T) {
	year, month, day, takenAt := ParseExifDateTaken("2024:01:15 12:30:45")
	if year == nil || *year != 2024 {
		t.Fatalf("year = %v", year)
	}
	if month == nil || *month != 1 {
		t.Fatalf("month = %v", month)
	}
	if day == nil || *day != 15 {
		t.Fatalf("day = %v", day)
	}
	if takenAt == nil {
		t.Fatal("expected takenAt")
	}
	if takenAt.Hour() != 12 || takenAt.Minute() != 30 || takenAt.Second() != 45 {
		t.Fatalf("unexpected time: %v", takenAt)
	}

	y2, m2, d2, at2 := ParseExifDateTaken("2020:07:04")
	if y2 == nil || *y2 != 2020 || m2 == nil || *m2 != 7 || d2 == nil || *d2 != 4 {
		t.Fatalf("partial date parse failed: %v %v %v", y2, m2, d2)
	}
	if at2 == nil {
		t.Fatal("expected takenAt for date-only value")
	}
	if at2.Location() != time.Local {
		t.Fatalf("location = %v", at2.Location())
	}
}
