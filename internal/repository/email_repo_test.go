package repository

import (
	"testing"
)

// TestSplitTrim verifies the address-splitting helper used in Search.
func TestSplitTrim(t *testing.T) {
	cases := []struct {
		in   string
		sep  rune
		want []string
	}{
		{"a, b, c", ',', []string{"a", "b", "c"}},
		{" alice@example.com , bob@example.com ", ',', []string{"alice@example.com", "bob@example.com"}},
		{"", ',', nil},
		{"single", ',', []string{"single"}},
		{",,,", ',', nil},
	}
	for _, tc := range cases {
		got := splitTrim(tc.in, tc.sep)
		if len(got) != len(tc.want) {
			t.Errorf("splitTrim(%q) = %v; want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitTrim(%q)[%d] = %q; want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

// TestIsNoRows verifies the no-rows error check.
func TestIsNoRows(t *testing.T) {
	if isNoRows(nil) {
		t.Error("isNoRows(nil) should be false")
	}
}
