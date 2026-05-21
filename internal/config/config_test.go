package config

import "testing"

func TestStripInlineEnvComment(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"tmp/tus_uploads  # where partial uploads are stored", "tmp/tus_uploads"},
		{"  tmp/foo  # comment  ", "tmp/foo"},
		{"no-comment", "no-comment"},
		{"foo#bar", "foo#bar"},
		{`'quoted # here' still`, `'quoted # here' still`},
		{`"double # here" tail`, `"double # here" tail`},
		{`x "a # b"  # real`, `x "a # b"`},
	}
	for _, tt := range tests {
		if got := stripInlineEnvComment(tt.in); got != tt.want {
			t.Errorf("stripInlineEnvComment(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
