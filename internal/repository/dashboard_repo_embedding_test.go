package repository

import "testing"

func TestPendingFromEmbeddedCount(t *testing.T) {
	tests := []struct {
		total, embedded, want int64
	}{
		{10, 4, 6},
		{10, 10, 0},
		{10, 12, 0},
		{0, 0, 0},
		{0, 3, 0},
	}
	for _, tc := range tests {
		if got := pendingFromEmbeddedCount(tc.total, tc.embedded); got != tc.want {
			t.Fatalf("pendingFromEmbeddedCount(%d, %d) = %d, want %d", tc.total, tc.embedded, got, tc.want)
		}
	}
}
