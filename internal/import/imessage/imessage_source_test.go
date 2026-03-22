package imessage

import "testing"

func TestMediaItemSource(t *testing.T) {
	tests := []struct {
		service string
		want    string
	}{
		{"", "imessage"},
		{"iMessage", "imessage"},
		{"SMS", "sms"},
		{"sms", "sms"},
		{"MMS", "sms"},
		{"mms", "sms"},
		{"RCS", "imessage"},
	}
	for _, tt := range tests {
		got := mediaItemSource(IMessageMessage{Service: tt.service})
		if got != tt.want {
			t.Errorf("mediaItemSource(Service=%q) = %q, want %q", tt.service, got, tt.want)
		}
	}
}
