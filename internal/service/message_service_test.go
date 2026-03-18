package service

import (
	"testing"
	"time"

	"github.com/daveontour/digitalmuseum/internal/model"
)

// TestIsPhoneNumber mirrors the Python is_phone_number() test cases.
func TestIsPhoneNumber(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		// International format
		{"+61412345678", true},
		{"+1 (555) 123-4567", true},
		{"+447911123456", true},
		// Plain digits
		{"0412345678", true},
		{"1234567", true}, // exactly 7 digits
		// Too short
		{"123456", false},
		// Names / chat sessions
		{"John Smith", false},
		{"Family Group", false},
		{"Work", false},
		// Edge cases
		{"", false},
		{"+", false},
		{"+12345", false}, // only 5 digits after +
		// Long string with digits (> 20 chars cleaned)
		{"12345678901234567890123", false},
	}

	for _, tc := range cases {
		got := isPhoneNumber(tc.input)
		if got != tc.want {
			t.Errorf("isPhoneNumber(%q) = %v; want %v", tc.input, got, tc.want)
		}
	}
}

// TestDetermineMessageType verifies the service labelling logic.
func TestDetermineMessageType(t *testing.T) {
	cases := []struct {
		row  model.ChatSessionRow
		want string
	}{
		{model.ChatSessionRow{IMessageCount: 5}, "imessage"},
		{model.ChatSessionRow{SMSCount: 3}, "sms"},
		{model.ChatSessionRow{WhatsAppCount: 10}, "whatsapp"},
		{model.ChatSessionRow{FacebookCount: 2}, "facebook"},
		{model.ChatSessionRow{InstagramCount: 1}, "instagram"},
		{model.ChatSessionRow{IMessageCount: 3, SMSCount: 1}, "mixed"},
		{model.ChatSessionRow{WhatsAppCount: 2, FacebookCount: 1}, "mixed"},
		{model.ChatSessionRow{}, "mixed"}, // all zero → non_zero_counts == 0, not 1
	}

	for _, tc := range cases {
		got := determineMessageType(tc.row)
		if got != tc.want {
			t.Errorf("determineMessageType(%+v) = %q; want %q", tc.row, got, tc.want)
		}
	}
}

// TestIsoString verifies the Python-compatible date formatting.
func TestIsoString(t *testing.T) {
	cases := []struct {
		t    *time.Time
		want *string
	}{
		{nil, nil},
	}

	// Non-nil with whole seconds (no fractional)
	ts := time.Date(2023, 6, 15, 10, 30, 0, 0, time.UTC)
	want := "2023-06-15T10:30:00"
	cases = append(cases, struct {
		t    *time.Time
		want *string
	}{&ts, &want})

	// With fractional seconds
	ts2 := time.Date(2023, 6, 15, 10, 30, 0, 500000000, time.UTC) // 0.5s
	want2 := "2023-06-15T10:30:00.5"
	cases = append(cases, struct {
		t    *time.Time
		want *string
	}{&ts2, &want2})

	for _, tc := range cases {
		got := isoString(tc.t)
		if tc.want == nil {
			if got != nil {
				t.Errorf("isoString(nil) = %q; want nil", *got)
			}
			continue
		}
		if got == nil {
			t.Errorf("isoString(%v) = nil; want %q", *tc.t, *tc.want)
			continue
		}
		if *got != *tc.want {
			t.Errorf("isoString(%v) = %q; want %q", *tc.t, *got, *tc.want)
		}
	}
}
