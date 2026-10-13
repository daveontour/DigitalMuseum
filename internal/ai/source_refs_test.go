package ai

import (
	"encoding/json"
	"testing"
)

// decodeLikeMCP round-trips a result through JSON, matching what the MCP client hands back.
func decodeLikeMCP(t *testing.T, v map[string]any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSourceCollector_ExtractsKnownToolShapes(t *testing.T) {
	c := newSourceCollector()
	c.addToolResult("search_emails_by_similarity", decodeLikeMCP(t, map[string]any{
		"emails": []map[string]any{
			{"id": 7, "subject": "Moving house", "from_address": "a@b.com", "date": "2014-05-02T10:00:00Z"},
		},
	}))
	c.addToolResult("search_chat_messages_globally", decodeLikeMCP(t, map[string]any{
		"matches": []map[string]any{{"message_id": 11, "chat_session_id": "Sarah", "content": "see you   there"}},
	}))
	c.addToolResult("find_photos_of_person", map[string]any{ // in-process shape (int64, []map)
		"photos": []map[string]any{{"media_item_id": int64(3), "title": "", "year": 2010, "month": 6, "day": 0}},
	})
	c.addToolResult("search_facebook_posts", decodeLikeMCP(t, map[string]any{
		"posts": []map[string]any{{"id": 5, "title": "Holiday"}},
	}))

	got := c.list()
	if len(got) != 4 {
		t.Fatalf("want 4 refs, got %d: %#v", len(got), got)
	}
	want := []struct {
		kind, label, detail string
		id                  int64
	}{
		{SourceKindEmail, "Moving house", "a@b.com · 2014-05-02", 7},
		{SourceKindMessage, "see you there", "Sarah", 11},
		{SourceKindPhoto, "Photo", "2010-06", 3},
		{SourceKindFacebookPost, "Holiday", "", 5},
	}
	for i, w := range want {
		r := got[i]
		if r["kind"] != w.kind || r["id"] != w.id || r["label"] != w.label {
			t.Errorf("ref %d = %#v, want kind=%s id=%d label=%q", i, r, w.kind, w.id, w.label)
		}
		d, _ := r["detail"].(string)
		if d != w.detail {
			t.Errorf("ref %d detail = %q, want %q", i, d, w.detail)
		}
	}
}

func TestSourceCollector_DedupesCapsAndIgnoresUnknown(t *testing.T) {
	c := newSourceCollector()
	msgs := make([]any, 0, maxSourcesPerKind+10)
	for i := 1; i <= maxSourcesPerKind+10; i++ {
		msgs = append(msgs, map[string]any{"id": float64(i), "text": "hi"})
	}
	c.addToolResult("get_imessages_by_chat_session", map[string]any{"messages": msgs})
	c.addToolResult("get_messages_around_in_chat", map[string]any{"messages": msgs[:3]}) // duplicates
	c.addToolResult("someserver:search_emails_by_similarity", map[string]any{
		"emails": []any{map[string]any{"id": float64(1), "subject": "x"}},
	})
	c.addToolResult("get_current_time", map[string]any{"time": "now"})
	c.addToolResult("search_emails_by_similarity", map[string]any{"error": "boom", "emails": []any{}})

	got := c.list()
	if len(got) != maxSourcesPerKind {
		t.Fatalf("want %d refs, got %d", maxSourcesPerKind, len(got))
	}
	for _, r := range got {
		if r["kind"] != SourceKindMessage {
			t.Fatalf("unexpected kind in %#v", r)
		}
	}
}

func TestSourceCollector_EmptyListIsNonNil(t *testing.T) {
	b, _ := json.Marshal(map[string]any{"sources": newSourceCollector().list()})
	if string(b) != `{"sources":[]}` {
		t.Fatalf("got %s", b)
	}
}
