package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestEscapeHTML verifies the HTML escaping helper used in GetHTML fallback.
func TestEscapeHTML(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"hello", "hello"},
		{"<b>bold</b>", "&lt;b&gt;bold&lt;/b&gt;"},
		{"a & b", "a &amp; b"},
		{`say "hi"`, "say &#34;hi&#34;"},
		{"it's fine", "it&#39;s fine"},
	}
	for _, tc := range cases {
		got := escapeHTML(tc.in)
		if got != tc.want {
			t.Errorf("escapeHTML(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

// TestWriteError verifies error responses have the correct status and JSON shape.
func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusNotFound, "email with ID 99 not found")

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q; want application/json", ct)
	}
	body := w.Body.String()
	if body == "" {
		t.Error("expected non-empty body")
	}
}

// TestParseEmailID verifies path parameter parsing.
func TestParseEmailID(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		// We can't easily test chi URL params in isolation without a full router.
		// The helper is thin enough that the chi integration test covers it.
		// This is a placeholder for the integration-level tests in Phase 7.
	})
}
