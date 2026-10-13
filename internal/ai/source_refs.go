package ai

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Source references ("sources consulted") for a chat answer.
//
// While a provider runs its tool-call loop, every tool result from the bundled MCP server is
// scanned for archive records the LLM was shown (messages, emails, photos, Facebook albums/posts).
// The collected references are returned in the response metadata under "sources" so the chat UI
// can render clickable chips that open each record in its existing viewer.
//
// Extraction is keyed on known bundled tool names and their documented result shapes (see the
// tool implementations in tools.go). Tools from additional (namespaced "<server>:<tool>") MCP
// servers are never matched, so an untrusted server cannot inject links into the UI.

// Source kinds understood by the frontend (static/js/museum/chat.js).
const (
	SourceKindMessage       = "message"
	SourceKindEmail         = "email"
	SourceKindPhoto         = "photo"
	SourceKindFacebookAlbum = "facebook_album"
	SourceKindFacebookPost  = "facebook_post"
)

// maxSourcesPerKind caps how many references of one kind are returned, so a tool that dumps a
// whole conversation does not bloat the response.
const maxSourcesPerKind = 50

const maxSourceLabelRunes = 90

// sourceExtractor describes where a tool's result keeps its records.
type sourceExtractor struct {
	kind     string
	arrayKey string
	idKey    string
}

var sourceExtractorsByTool = map[string][]sourceExtractor{
	"search_chat_messages_globally":   {{SourceKindMessage, "matches", "message_id"}},
	"search_chat_messages_in_session": {{SourceKindMessage, "matches", "message_id"}},
	"get_imessages_by_chat_session":   {{SourceKindMessage, "messages", "id"}},
	"get_messages_around_in_chat":     {{SourceKindMessage, "messages", "id"}},
	"search_messages_by_similarity":   {{SourceKindMessage, "messages", "id"}},
	"search_emails_by_similarity":     {{SourceKindEmail, "emails", "id"}},
	"get_emails_by_contact":           {{SourceKindEmail, "emails", "id"}},
	"find_photos_of_person":           {{SourceKindPhoto, "photos", "media_item_id"}},
	"get_album_images":                {{SourceKindPhoto, "images", "id"}},
	"search_facebook_albums":          {{SourceKindFacebookAlbum, "albums", "id"}},
	"search_facebook_posts":           {{SourceKindFacebookPost, "posts", "id"}},
	"get_all_facebook_posts":          {{SourceKindFacebookPost, "posts", "id"}},
}

// sourceCollector accumulates de-duplicated source references across one GenerateResponse call.
// Not safe for concurrent use (each provider call owns its own collector).
type sourceCollector struct {
	refs    []map[string]any
	seen    map[string]struct{}
	perKind map[string]int
}

func newSourceCollector() *sourceCollector {
	return &sourceCollector{
		refs:    []map[string]any{},
		seen:    map[string]struct{}{},
		perKind: map[string]int{},
	}
}

// addToolResult extracts source references from one tool result. Unknown tools are ignored.
func (c *sourceCollector) addToolResult(toolName string, result map[string]any) {
	if c == nil || result == nil {
		return
	}
	extractors, ok := sourceExtractorsByTool[toolName]
	if !ok {
		return
	}
	for _, ex := range extractors {
		for _, item := range srcItemList(result[ex.arrayKey]) {
			id, ok := srcInt64(item[ex.idKey])
			if !ok || id <= 0 {
				continue
			}
			c.add(ex.kind, id, item)
		}
	}
}

func (c *sourceCollector) add(kind string, id int64, item map[string]any) {
	key := kind + ":" + strconv.FormatInt(id, 10)
	if _, dup := c.seen[key]; dup {
		return
	}
	if c.perKind[kind] >= maxSourcesPerKind {
		return
	}
	c.seen[key] = struct{}{}
	c.perKind[kind]++

	label, detail := sourceLabel(kind, item)
	ref := map[string]any{
		"kind":  kind,
		"id":    id,
		"label": label,
	}
	if detail != "" {
		ref["detail"] = detail
	}
	c.refs = append(c.refs, ref)
}

// list returns the collected references (never nil, so JSON encodes as []).
func (c *sourceCollector) list() []map[string]any {
	if c == nil || c.refs == nil {
		return []map[string]any{}
	}
	return c.refs
}

func sourceLabel(kind string, item map[string]any) (label, detail string) {
	switch kind {
	case SourceKindMessage:
		label = srcFirstNonEmpty(srcStr(item["text"]), srcStr(item["content"]), srcStr(item["subject"]))
		who := srcStr(item["sender_name"])
		if who == "Unknown" {
			who = ""
		}
		detail = srcJoinNonEmpty(" · ", srcFirstNonEmpty(who, srcStr(item["chat_session"]), srcStr(item["chat_session_id"])), srcShortDate(srcStr(item["message_date"])))
		if label == "" {
			label = "Message"
		}
	case SourceKindEmail:
		label = srcFirstNonEmpty(srcStr(item["subject"]), "(no subject)")
		detail = srcJoinNonEmpty(" · ", srcStr(item["from_address"]), srcShortDate(srcStr(item["date"])))
	case SourceKindPhoto:
		label = srcFirstNonEmpty(srcStr(item["title"]), "Photo")
		detail = srcJoinNonEmpty(" · ", srcStr(item["contact_name"]), srcYMD(item["year"], item["month"], item["day"]))
	case SourceKindFacebookAlbum:
		label = srcFirstNonEmpty(srcStr(item["name"]), "Facebook album")
		detail = srcStr(item["description"])
	case SourceKindFacebookPost:
		label = srcFirstNonEmpty(srcStr(item["title"]), srcStr(item["description"]), "Facebook post")
		detail = srcShortDate(srcStr(item["timestamp"]))
	}
	return srcTruncate(srcCollapseSpace(label), maxSourceLabelRunes), srcTruncate(srcCollapseSpace(detail), maxSourceLabelRunes)
}

// asItemList accepts both the in-process shape ([]map[string]any) and the JSON-decoded shape
// ([]any of map[string]any) that arrives over MCP.
func srcItemList(v any) []map[string]any {
	switch t := v.(type) {
	case []map[string]any:
		return t
	case []any:
		out := make([]map[string]any, 0, len(t))
		for _, e := range t {
			if m, ok := e.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

func srcInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case int32:
		return int64(n), true
	case float64:
		return int64(n), n == float64(int64(n))
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	case string:
		i, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		return i, err == nil
	}
	return 0, false
}

func srcStr(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case nil:
		return ""
	}
	return ""
}

func srcFirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func srcJoinNonEmpty(sep string, vals ...string) string {
	parts := make([]string, 0, len(vals))
	for _, v := range vals {
		if v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, sep)
}

// shortDate keeps the YYYY-MM-DD prefix of an ISO-ish timestamp; other formats pass through.
func srcShortDate(s string) string {
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		return s[:10]
	}
	return s
}

func srcYMD(y, m, d any) string {
	yi, yok := srcInt64(y)
	if !yok || yi <= 0 {
		return ""
	}
	mi, mok := srcInt64(m)
	if !mok || mi <= 0 {
		return strconv.FormatInt(yi, 10)
	}
	di, dok := srcInt64(d)
	if !dok || di <= 0 {
		return fmt.Sprintf("%04d-%02d", yi, mi)
	}
	return fmt.Sprintf("%04d-%02d-%02d", yi, mi, di)
}

func srcCollapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func srcTruncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n-1])) + "…"
}
