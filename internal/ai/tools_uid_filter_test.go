package ai

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/daveontour/aimuseum/internal/appctx"
)

func TestToolsUIDFilter_withNewlineBeforeWhere(t *testing.T) {
	q := `
		SELECT DISTINCT chat_session
		FROM messages
		WHERE chat_session IS NOT NULL AND chat_session <> ''
		ORDER BY chat_session
	`
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, int64(42))
	out, args := toolsUIDFilter(ctx, q, nil)
	if len(args) != 1 || args[0] != int64(42) {
		t.Fatalf("args = %v; want [42]", args)
	}
	whereCount := len(regexp.MustCompile(`(?i)\bwhere\b`).FindAllString(out, -1))
	if whereCount != 1 {
		t.Fatalf("expected single WHERE clause, got %d in:\n%s", whereCount, out)
	}
	if !strings.Contains(out, "AND user_id = ?") {
		t.Fatalf("expected AND user_id filter, got:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "order by chat_session") {
		t.Fatalf("ORDER BY should remain after filter, got:\n%s", out)
	}
}

func TestToolsUIDFilter_addsWhereWhenMissing(t *testing.T) {
	q := `SELECT id FROM messages ORDER BY id`
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, int64(1))
	out, _ := toolsUIDFilter(ctx, q, nil)
	if !strings.Contains(out, " WHERE user_id = ?") {
		t.Fatalf("expected WHERE user_id, got:\n%s", out)
	}
}

func TestToolsUIDFilter_skipsWhenUnauthenticated(t *testing.T) {
	q := `SELECT id FROM messages WHERE id = 1`
	out, args := toolsUIDFilter(context.Background(), q, nil)
	if out != q || args != nil {
		t.Fatalf("expected unchanged query for uid 0")
	}
}

func TestToolsUIDFilter_beforeOrderByWithNewline(t *testing.T) {
	q := fmt.Sprintf(`
		SELECT id, chat_session, text, subject
		FROM messages
		WHERE chat_session IS NOT NULL AND TRIM(chat_session) <> ''
		  AND (COALESCE(text, '') LIKE ? OR COALESCE(subject, '') LIKE ?)
		ORDER BY message_date IS NULL, message_date ASC, id ASC
		LIMIT %d`, 200)
	ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, int64(42))
	out, args := toolsUIDFilter(ctx, q, []any{"%kw%", "%kw%"})
	if len(args) != 3 {
		t.Fatalf("args = %v; want 3 (2 patterns + uid)", args)
	}
	idxUser := strings.Index(out, "AND user_id = ?")
	idxOrder := strings.Index(strings.ToLower(out), "order by")
	idxLimit := strings.Index(strings.ToLower(out), "limit")
	if idxUser < 0 || idxOrder < 0 || idxLimit < 0 {
		t.Fatalf("expected user_id before ORDER BY and LIMIT, got:\n%s", out)
	}
	if idxUser > idxOrder || idxOrder > idxLimit {
		t.Fatalf("expected AND user_id = ? before ORDER BY before LIMIT, got:\n%s", out)
	}
}
