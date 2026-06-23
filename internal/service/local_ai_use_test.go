package service

import "testing"

func TestParseLocalAIUseEnabled(t *testing.T) {
	trueVal := "true"
	falseVal := "false"
	tests := []struct {
		raw  *string
		want bool
	}{
		{nil, true},
		{&trueVal, true},
		{&falseVal, false},
	}
	for _, tc := range tests {
		if got := parseLocalAIUseEnabled(tc.raw); got != tc.want {
			t.Fatalf("parseLocalAIUseEnabled(%v) = %v want %v", tc.raw, got, tc.want)
		}
	}
	off := "off"
	if parseLocalAIUseEnabled(&off) {
		t.Fatal("off should be false")
	}
}
