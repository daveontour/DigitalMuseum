package sqlutil

import "testing"

func TestOrderByAscNullsLast(t *testing.T) {
	got := OrderByAscNullsLast("e.date")
	want := "e.date IS NULL, e.date ASC"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestOrderByDescNullsLast_withTieBreaker(t *testing.T) {
	got := OrderByDescNullsLast("fp.timestamp")
	want := "fp.timestamp IS NULL, fp.timestamp DESC"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	got = OrderByAscNullsLast("message_date", "id ASC")
	want = "message_date IS NULL, message_date ASC, id ASC"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
