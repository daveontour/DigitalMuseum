package ai

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/daveontour/aimuseum/internal/appctx"
	appcrypto "github.com/daveontour/aimuseum/internal/crypto"
	"github.com/daveontour/aimuseum/internal/sqlutil"
)

// RAMMasterGetter returns the session master password when the browser has unlocked the keyring.
type RAMMasterGetter func() (password string, ok bool)

// chatMessageNeighborCount is how many messages to include before and after the anchor (inclusive window is up to 2*N+1).
const chatMessageNeighborCount = 20

// chatMessageKeywordSearchLimit caps rows returned by global / per-session message content search tools.
const chatMessageKeywordSearchLimit = 200

// NewToolExecutor creates a ToolExecutor backed by the provided pool.
// getRAM returns the sensitive keyring password for this HTTP request's session; used to decrypt encrypted reference documents for AI tools.
func NewToolExecutor(pool *sql.DB, subjectName, tavilyKey, pepper string, getRAM RAMMasterGetter) ToolExecutor {
	if getRAM == nil {
		getRAM = func() (string, bool) { return "", false }
	}
	return func(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
		switch name {
		case "get_current_time":
			return map[string]any{
				"current_time": time.Now().UTC().Format(time.RFC3339),
				"timezone":     "UTC",
			}, nil
		case "get_imessages_by_chat_session":
			chatSession, _ := args["chat_session"].(string)
			return getMessagesByChatSession(ctx, pool, chatSession)
		case "get_messages_around_in_chat":
			chatSession, _ := args["chat_session"].(string)
			var messageID int64
			switch v := args["message_id"].(type) {
			case float64:
				messageID = int64(v)
			case int64:
				messageID = v
			}
			return getMessagesAroundInChat(ctx, pool, chatSession, messageID)

		case "get_emails_by_contact":
			n, _ := args["name"].(string)
			return getEmailsByContact(ctx, pool, n)
		case "get_subject_writing_examples":
			return getSubjectWritingExamples(ctx, pool, subjectName)
		case "search_tavily":
			query, _ := args["query"].(string)
			return searchTavily(tavilyKey, query)
		case "get_all_messages_by_contact":
			n, _ := args["name"].(string)
			return getAllMessagesByContact(ctx, pool, n)
		case "get_unique_tags_count":
			return getUniqueTagsCount(ctx, pool)
		case "search_facebook_albums":
			keyword, _ := args["keyword"].(string)
			return searchFacebookAlbums(ctx, pool, keyword)
		case "get_album_images":
			var albumID int64
			switch v := args["album_id"].(type) {
			case float64:
				albumID = int64(v)
			case int64:
				albumID = v
			}
			return getAlbumImagesTool(ctx, pool, albumID)
		case "search_facebook_posts":
			desc, _ := args["description"].(string)
			return searchFacebookPosts(ctx, pool, desc)
		case "get_all_facebook_posts":
			return getAllFacebookPosts(ctx, pool)
		case "get_user_interests":
			return getUserInterests(ctx, pool)
		case "get_available_reference_documents":
			return getAvailableReferenceDocuments(ctx, pool)
		case "get_available_sensitive_reference_documents":
			return getAvailableSensitiveReferenceDocuments(ctx, pool)
		case "get_reference_document":
			idsRaw, _ := args["document_ids"].([]any)
			var ids []int64
			for _, v := range idsRaw {
				switch x := v.(type) {
				case float64:
					ids = append(ids, int64(x))
				case int64:
					ids = append(ids, x)
				}
			}
			return getReferenceDocuments(ctx, pool, ids, pepper, getRAM)
		case "get_sensitive_reference_document":
			idsRaw, _ := args["document_ids"].([]any)
			var ids []int64
			for _, v := range idsRaw {
				switch x := v.(type) {
				case float64:
					ids = append(ids, int64(x))
				case int64:
					ids = append(ids, x)
				}
			}
			return getSensitiveReferenceDocuments(ctx, pool, ids, pepper, getRAM)
		case "list_interviews":
			stateFilter, _ := args["state"].(string)
			return listInterviewsTool(ctx, pool, stateFilter)
		case "get_interview":
			var iid int64
			switch v := args["interview_id"].(type) {
			case float64:
				iid = int64(v)
			case int64:
				iid = v
			}
			return getInterviewTool(ctx, pool, iid)
		case "list_available_chat_sessions":
			return listAvailableChatSessions(ctx, pool)
		case "search_chat_messages_globally":
			kw, _ := args["keyword"].(string)
			return searchChatMessagesGlobally(ctx, pool, kw)
		case "search_chat_messages_in_session":
			ch, _ := args["chat_session"].(string)
			kw, _ := args["keyword"].(string)
			return searchChatMessagesInSession(ctx, pool, ch, kw)
		case "search_messages_by_similarity":
			text, _ := args["text"].(string)
			return searchMessagesBySimilarity(ctx, pool, text)
		case "search_emails_by_similarity":
			text, _ := args["text"].(string)
			return searchEmailsBySimilarity(ctx, pool, text)
		case "list_complete_profiles":
			return listCompleteProfilesTool(ctx, pool)
		case "get_complete_profile":
			n, _ := args["name"].(string)
			return getCompleteProfileTool(ctx, pool, n)
		default:
			return nil, fmt.Errorf("unknown tool: %s", name)
		}
	}
}

// toolsUIDFilterInsertPoint returns the byte index in q before which a user_id
// predicate must be inserted. Appending "AND user_id" after ORDER BY/LIMIT
// produces invalid SQL, so we splice before the first trailing clause.
func toolsUIDFilterInsertPoint(q string) int {
	ql := strings.ToLower(q)
	markers := []string{" order by", " group by", " having ", " limit ", " offset ", " fetch "}
	best := len(q)
	for _, m := range markers {
		if i := strings.Index(ql, m); i >= 0 && i < best {
			best = i
		}
	}
	return best
}

// toolsUIDFilter appends user_id = $N to the WHERE clause (or adds WHERE) when
// the context carries a non-zero userID. When userID == 0 (unauthenticated /
// single-tenant mode) no filter is added, preserving backward-compatible behaviour.
func toolsUIDFilter(ctx context.Context, q string, args []any) (string, []any) {
	uid := appctx.UserIDFromCtx(ctx)
	if uid == 0 {
		return q, args
	}
	args = append(args, uid)
	insertAt := toolsUIDFilterInsertPoint(q)
	prefix := strings.TrimRight(q[:insertAt], " \t\n\r")
	suffix := q[insertAt:]
	qlPrefix := strings.ToLower(prefix)
	hasWhere := strings.Contains(qlPrefix, " where ")
	joiner := " AND user_id = "
	if !hasWhere {
		joiner = " WHERE user_id = "
	}
	return prefix + joiner + "?" + suffix, args
}

func visitorKeyReferenceDocsRestricted(ctx context.Context) bool {
	return appctx.VisitorAccessFromCtx(ctx).Restricted
}

func visitorKeyMayReadReferenceDoc(ctx context.Context, pool *sql.DB, docID int64) (bool, error) {
	_, _, _ = ctx, pool, docID
	// Visitor key hint junction tables are not present in the SQLite build.
	return false, nil
}

func visitorKeySensitiveLLMAccessDenied(ctx context.Context) bool {
	va := appctx.VisitorAccessFromCtx(ctx)
	return va.Restricted && !va.AllowSensitivePrivate()
}

func toolEmbedText(ctx context.Context, text string) ([]float32, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("text is required")
	}
	baseURL := strings.TrimSpace(os.Getenv("LOCALAI_BASE_URL"))
	apiKey := strings.TrimSpace(os.Getenv("LOCALAI_API_KEY"))
	model := strings.TrimSpace(os.Getenv("LOCALAI_MODEL_NAME"))
	embeddingModel := strings.TrimSpace(os.Getenv("LOCALAI_EMBEDDING_MODEL"))
	if embeddingModel == "" {
		embeddingModel = model
	}
	// num_ctx is irrelevant for the embed endpoint, so pass 0 here.
	provider := NewLocalAIProvider(baseURL, apiKey, model, 0)
	if provider == nil || !provider.IsAvailable() {
		return nil, fmt.Errorf("embedding service unavailable: set LOCALAI_BASE_URL and LOCALAI_EMBEDDING_MODEL")
	}
	return provider.Embed(ctx, text, embeddingModel)
}

func toolVectorLiteral(values []float32) string {
	if len(values) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.Grow(len(values) * 10)
	b.WriteByte('[')
	for i, v := range values {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(v), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

func searchMessagesBySimilarity(ctx context.Context, pool *sql.DB, text string) (map[string]any, error) {
	queryText := strings.TrimSpace(text)
	if queryText == "" {
		return map[string]any{"error": "text is required", "messages": []any{}}, nil
	}
	if pool == nil {
		return map[string]any{"error": "message similarity search not configured", "messages": []any{}}, nil
	}

	vec, err := toolEmbedText(ctx, queryText)
	if err != nil {
		return map[string]any{"error": fmt.Sprintf("embedding failed: %v", err), "messages": []any{}}, nil
	}
	vecBlob, err := sqlite_vec.SerializeFloat32(vec)
	if err != nil {
		return map[string]any{"error": fmt.Sprintf("failed to serialize embedding: %v", err), "messages": []any{}}, nil
	}

	const topN = 15
	rows, err := pool.QueryContext(ctx, `
		SELECT rowid, int_ids
		FROM message_embeddings
		WHERE embedding MATCH ? AND k = ?
		ORDER BY distance ASC
	`, vecBlob, topN)
	if err != nil {
		return map[string]any{"error": fmt.Sprintf("vector search failed: %v", err), "messages": []any{}}, nil
	}
	defer rows.Close()

	combinedIDs := make([]int64, 0, topN*11)
	seen := make(map[int64]struct{}, topN*11)
	for rows.Next() {
		var (
			rowID    int64
			intIDsJS string
			ids      []int64
		)
		if err := rows.Scan(&rowID, &intIDsJS); err != nil {
			return map[string]any{"error": fmt.Sprintf("scan match row: %v", err), "messages": []any{}}, nil
		}
		if strings.TrimSpace(intIDsJS) == "" {
			continue
		}
		if err := json.Unmarshal([]byte(intIDsJS), &ids); err != nil {
			return map[string]any{"error": fmt.Sprintf("parse int_ids for row %d: %v", rowID, err), "messages": []any{}}, nil
		}
		for _, id := range ids {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			combinedIDs = append(combinedIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		return map[string]any{"error": fmt.Sprintf("iterate vector results: %v", err), "messages": []any{}}, nil
	}
	_ = rows.Close()

	if len(combinedIDs) == 0 {
		return map[string]any{
			"query_text":      queryText,
			"match_count":     0,
			"messages":        []any{},
			"unique_messages": []any{},
		}, nil
	}

	uid := appctx.UserIDFromCtx(ctx)
	placeholders := make([]string, len(combinedIDs))
	args := make([]any, 0, len(combinedIDs)+1)
	for i, id := range combinedIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, uid)

	q := fmt.Sprintf(`
		SELECT id, COALESCE(chat_session, ''), COALESCE(text, '')
		FROM messages
		WHERE id IN (%s)
		  AND COALESCE(user_id, 0) = ?
	`, strings.Join(placeholders, ","))
	msgRows, err := pool.QueryContext(ctx, q, args...)
	if err != nil {
		return map[string]any{"error": fmt.Sprintf("load unique messages failed: %v", err), "messages": []any{}}, nil
	}
	defer msgRows.Close()

	byID := make(map[int64]map[string]any, len(combinedIDs))
	for msgRows.Next() {
		var (
			id          int64
			chatSession string
			msgText     string
		)
		if err := msgRows.Scan(&id, &chatSession, &msgText); err != nil {
			return map[string]any{"error": fmt.Sprintf("scan message row failed: %v", err), "messages": []any{}}, nil
		}
		byID[id] = map[string]any{
			"id":           id,
			"chat_session": chatSession,
			"text":         msgText,
		}
	}
	if err := msgRows.Err(); err != nil {
		return map[string]any{"error": fmt.Sprintf("iterate message rows failed: %v", err), "messages": []any{}}, nil
	}

	uniqueMessages := make([]any, 0, len(combinedIDs))
	for _, id := range combinedIDs {
		if m, ok := byID[id]; ok {
			uniqueMessages = append(uniqueMessages, m)
		}
	}
	return map[string]any{
		"query_text":      queryText,
		"match_count":     len(uniqueMessages),
		"messages":        uniqueMessages,
		"unique_messages": uniqueMessages,
	}, nil
}

func searchEmailsBySimilarity(ctx context.Context, pool *sql.DB, text string) (map[string]any, error) {
	queryText := strings.TrimSpace(text)
	if queryText == "" {
		return map[string]any{"error": "text is required", "emails": []any{}}, nil
	}
	if pool == nil {
		return map[string]any{"error": "email similarity search not configured", "emails": []any{}}, nil
	}

	vec, err := toolEmbedText(ctx, queryText)
	if err != nil {
		return map[string]any{"error": fmt.Sprintf("embedding failed: %v", err), "emails": []any{}}, nil
	}
	vecBlob, err := sqlite_vec.SerializeFloat32(vec)
	if err != nil {
		return map[string]any{"error": fmt.Sprintf("failed to serialize embedding: %v", err), "emails": []any{}}, nil
	}

	const topN = 20
	rows, err := pool.QueryContext(ctx, `
		SELECT rowid, int_ids
		FROM email_embeddings
		WHERE embedding MATCH ? AND k = ?
		ORDER BY distance ASC
	`, vecBlob, topN)
	if err != nil {
		return map[string]any{"error": fmt.Sprintf("vector search failed: %v", err), "emails": []any{}}, nil
	}

	combinedIDs := make([]int64, 0, topN)
	seen := make(map[int64]struct{}, topN)
	for rows.Next() {
		var (
			rowID    int64
			intIDsJS string
			ids      []int64
		)
		if err := rows.Scan(&rowID, &intIDsJS); err != nil {
			_ = rows.Close()
			return map[string]any{"error": fmt.Sprintf("scan match row: %v", err), "emails": []any{}}, nil
		}
		if rowID > 0 {
			if _, ok := seen[rowID]; !ok {
				seen[rowID] = struct{}{}
				combinedIDs = append(combinedIDs, rowID)
			}
		}
		if strings.TrimSpace(intIDsJS) == "" {
			continue
		}
		if err := json.Unmarshal([]byte(intIDsJS), &ids); err != nil {
			_ = rows.Close()
			return map[string]any{"error": fmt.Sprintf("parse int_ids for row %d: %v", rowID, err), "emails": []any{}}, nil
		}
		for _, id := range ids {
			if id <= 0 {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			combinedIDs = append(combinedIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return map[string]any{"error": fmt.Sprintf("iterate vector results: %v", err), "emails": []any{}}, nil
	}
	_ = rows.Close()

	if len(combinedIDs) == 0 {
		return map[string]any{
			"query_text":  queryText,
			"match_count": 0,
			"emails":      []any{},
		}, nil
	}

	uid := appctx.UserIDFromCtx(ctx)
	placeholders := make([]string, len(combinedIDs))
	args := make([]any, 0, len(combinedIDs)+1)
	for i, id := range combinedIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, uid)

	q := fmt.Sprintf(`
		SELECT id, date, from_address, to_addresses, subject, plain_text, snippet, has_attachments
		FROM emails
		WHERE id IN (%s)
		  AND user_deleted = FALSE
		  AND COALESCE(user_id, 0) = ?
	`, strings.Join(placeholders, ","))
	emailRows, err := pool.QueryContext(ctx, q, args...)
	if err != nil {
		return map[string]any{"error": fmt.Sprintf("load emails failed: %v", err), "emails": []any{}}, nil
	}
	defer emailRows.Close()

	byID := make(map[int64]map[string]any, len(combinedIDs))
	for emailRows.Next() {
		var (
			id             int64
			date           sqlutil.NullDBTime
			fromAddr       *string
			toAddrs        *string
			subject        *string
			plainText      *string
			snippet        *string
			hasAttachments bool
		)
		if err := emailRows.Scan(&id, &date, &fromAddr, &toAddrs, &subject, &plainText, &snippet, &hasAttachments); err != nil {
			return map[string]any{"error": fmt.Sprintf("scan email row failed: %v", err), "emails": []any{}}, nil
		}
		textBody := ""
		if plainText != nil && *plainText != "" {
			textBody = *plainText
		} else if snippet != nil {
			textBody = *snippet
		}
		e := map[string]any{
			"id":              id,
			"date":            nil,
			"from_address":    strVal(fromAddr, ""),
			"to_addresses":    strVal(toAddrs, ""),
			"subject":         strVal(subject, ""),
			"plain_text":      textBody,
			"has_attachments": hasAttachments,
		}
		if date.Valid {
			e["date"] = date.Time.Format(time.RFC3339)
		}
		byID[id] = e
	}
	if err := emailRows.Err(); err != nil {
		return map[string]any{"error": fmt.Sprintf("iterate email rows failed: %v", err), "emails": []any{}}, nil
	}

	emails := make([]any, 0, len(combinedIDs))
	for _, id := range combinedIDs {
		if e, ok := byID[id]; ok {
			emails = append(emails, e)
		}
	}
	return map[string]any{
		"query_text":  queryText,
		"match_count": len(emails),
		"emails":      emails,
	}, nil
}

// visitorKeyMayReadSensitiveReferenceDoc is true for non-restricted sessions (owner or share visitor).
// For restricted visitor-key sessions with sensitive/private access, the document must appear
// in visitor_key_hint_sensitive_reference_documents for the session hint.
func visitorKeyMayReadSensitiveReferenceDoc(ctx context.Context, pool *sql.DB, docID int64) (bool, error) {
	va := appctx.VisitorAccessFromCtx(ctx)
	if !va.Restricted {
		return true, nil
	}
	if !va.AllowSensitivePrivate() {
		return false, nil
	}
	_, _, _ = pool, docID, ctx
	// Visitor key hint junction tables are not present in the SQLite build.
	return false, nil
}

// tool to get the title, description, id and tags of all reference documents
func getAvailableReferenceDocuments(ctx context.Context, pool *sql.DB) (map[string]any, error) {
	if visitorKeyReferenceDocsRestricted(ctx) {
		// Visitor key hint junction tables are not present in the SQLite build.
		return map[string]any{"documents": []map[string]any{}}, nil
	}
	q, args := toolsUIDFilter(ctx, `SELECT id, title, description, tags FROM reference_documents WHERE available_for_task = TRUE AND NOT include_in_system_prompt AND is_sensitive = FALSE`, nil)
	rows, err := pool.QueryContext(ctx, q, args...)
	if err != nil {
		return map[string]any{"error": err.Error(), "documents": []any{}}, nil
	}
	defer rows.Close()
	var documents []map[string]any

	for rows.Next() {
		var id int64
		var title, description, tags *string
		if err := rows.Scan(&id, &title, &description, &tags); err != nil {
			continue
		}
		documents = append(documents, map[string]any{
			"id":          id,
			"title":       title,
			"description": description,
			"tags":        tags,
		})
	}
	if documents == nil {
		documents = []map[string]any{}
	}
	return map[string]any{"documents": documents}, nil
}

// scanStandardMessageRows reads rows from SELECT id, message_date, sender_name, sender_id, type, text, service, subject.
func scanStandardMessageRows(rows *sql.Rows) ([]map[string]any, error) {
	var msgs []map[string]any
	for rows.Next() {
		var id int64
		var msgDate sqlutil.NullDBTime
		var senderName, senderID, typ, text, service, subject *string
		if err := rows.Scan(&id, &msgDate, &senderName, &senderID, &typ, &text, &service, &subject); err != nil {
			continue
		}
		m := map[string]any{
			"id":           id,
			"message_date": nil,
			"sender_name":  strVal(senderName, "Unknown"),
			"sender_id":    strVal(senderID, ""),
			"type":         strVal(typ, ""),
			"text":         strVal(text, ""),
			"service":      strVal(service, ""),
			"subject":      strVal(subject, ""),
		}
		if msgDate.Valid {
			m["message_date"] = msgDate.Time.Format(time.RFC3339)
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if msgs == nil {
		msgs = []map[string]any{}
	}
	return msgs, nil
}

func getMessagesByChatSession(ctx context.Context, pool *sql.DB, chatSession string) (map[string]any, error) {
	q, args := toolsUIDFilter(ctx,
		`SELECT id, message_date, sender_name, sender_id, type, text, service, subject FROM messages WHERE chat_session LIKE ? ORDER BY message_date ASC LIMIT 500`,
		[]any{"%" + chatSession + "%"})
	rows, err := pool.QueryContext(ctx, q, args...)
	if err != nil {
		return map[string]any{"error": err.Error(), "chat_session": chatSession, "message_count": 0, "messages": []any{}}, nil
	}
	defer rows.Close()
	msgs, scanErr := scanStandardMessageRows(rows)
	if scanErr != nil {
		return map[string]any{"error": scanErr.Error(), "chat_session": chatSession, "message_count": 0, "messages": []any{}}, nil
	}
	return map[string]any{
		"chat_session":  chatSession,
		"message_count": len(msgs),
		"messages":      msgs,
	}, nil
}

// GetMessagesForContactProfile returns messages for relationship / complete-profile generation.
// It matches chat_session, sender_name, and sender_id against the display name, and — when a
// contacts row exists with that exact name — also matches email, messenger IDs, and alternative_names.
// This fixes the case where get_imessages_by_chat_session alone returned nothing because thread
// titles do not contain the contact's display name.
func GetMessagesForContactProfile(ctx context.Context, pool *sql.DB, name string) (map[string]any, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return map[string]any{"contact_name": name, "message_count": 0, "messages": []any{}}, nil
	}
	pat := "%" + name + "%"
	orParts := []string{"chat_session LIKE ?", "sender_name LIKE ?", "sender_id LIKE ?"}
	args := []any{pat, pat, pat}

	cq, cargs := toolsUIDFilter(ctx,
		`SELECT email, facebookid, whatsappid, imessageid, smsid, instagramid, alternative_names FROM contacts WHERE name = ?`,
		[]any{name})
	var email, fb, wa, im, sms, ig, altNames sql.NullString
	if err := pool.QueryRowContext(ctx, cq, cargs...).Scan(&email, &fb, &wa, &im, &sms, &ig, &altNames); err == nil {
		addIDPatterns := func(s string) {
			s = strings.TrimSpace(s)
			if s == "" {
				return
			}
			p := "%" + s + "%"
			orParts = append(orParts, "sender_id LIKE ?", "chat_session LIKE ?")
			args = append(args, p, p)
		}
		addIDPatterns(name)
		if email.Valid {
			addIDPatterns(email.String)
		}
		if fb.Valid {
			addIDPatterns(fb.String)
		}
		if wa.Valid {
			addIDPatterns(wa.String)
		}
		if im.Valid {
			addIDPatterns(im.String)
		}
		if sms.Valid {
			addIDPatterns(sms.String)
		}
		if ig.Valid {
			addIDPatterns(ig.String)
		}
		if altNames.Valid {
			for _, alt := range strings.Split(altNames.String, ",") {
				alt = strings.TrimSpace(alt)
				if alt == "" {
					continue
				}
				ap := "%" + alt + "%"
				orParts = append(orParts, "sender_name LIKE ?", "sender_id LIKE ?", "chat_session LIKE ?")
				args = append(args, ap, ap, ap)
			}
		}
	}

	where := "(" + strings.Join(orParts, " OR ") + ")"
	baseQ := `SELECT id, message_date, sender_name, sender_id, type, text, service, subject FROM messages WHERE ` + where + ` ORDER BY message_date ASC`
	q, args := toolsUIDFilter(ctx, baseQ, args)

	// create the fully expanded sql query string
	slog.Info("getMessagesForContactProfile: query", "q", q, "args", args)
	rows, err := pool.QueryContext(ctx, q, args...)
	if err != nil {
		return map[string]any{"error": err.Error(), "contact_name": name, "message_count": 0, "messages": []any{}}, nil
	}
	defer rows.Close()
	msgs, scanErr := scanStandardMessageRows(rows)
	if scanErr != nil {
		return map[string]any{"error": scanErr.Error(), "contact_name": name, "message_count": 0, "messages": []any{}}, nil
	}
	return map[string]any{
		"contact_name":  name,
		"message_count": len(msgs),
		"messages":      msgs,
	}, nil
}

// getMessagesAroundInChat returns up to chatMessageNeighborCount messages before and after
// the anchor row in one chat session, ordered by message_date then id.
// chat_session is matched the same way as getMessagesByChatSession (LIKE with wildcards).
func getMessagesAroundInChat(ctx context.Context, pool *sql.DB, chatSession string, messageID int64) (map[string]any, error) {
	if strings.TrimSpace(chatSession) == "" || messageID <= 0 {
		return map[string]any{
			"error":         "chat_session and positive message_id are required",
			"chat_session":  chatSession,
			"message_id":    messageID,
			"message_count": 0,
			"anchor_rn":     nil,
			"messages":      []any{},
		}, nil
	}
	uid := appctx.UserIDFromCtx(ctx)
	pattern := "%" + chatSession + "%"
	innerWhere := "WHERE chat_session LIKE ?"
	args := []any{pattern}
	if uid > 0 {
		innerWhere += " AND user_id = ?"
		args = append(args, uid)
	}
	args = append(args, messageID)

	q := fmt.Sprintf(`WITH session_msgs AS (
		SELECT id, message_date, sender_name, sender_id, type, text, service, subject,
			ROW_NUMBER() OVER (ORDER BY message_date ASC NULLS LAST, id ASC) AS rn
		FROM messages
		%s
	),
	anchor AS (
		SELECT rn FROM session_msgs WHERE id = ? LIMIT 1
	)
	SELECT sm.id, sm.message_date, sm.sender_name, sm.sender_id, sm.type, sm.text, sm.service, sm.subject, sm.rn, a.rn AS anchor_rn
	FROM session_msgs sm
	CROSS JOIN anchor a
	WHERE sm.rn BETWEEN GREATEST(1, a.rn - %d) AND a.rn + %d
	ORDER BY sm.rn`,
		innerWhere, chatMessageNeighborCount, chatMessageNeighborCount)

	rows, err := pool.QueryContext(ctx, q, args...)
	if err != nil {
		return map[string]any{
			"error":         err.Error(),
			"chat_session":  chatSession,
			"message_id":    messageID,
			"message_count": 0,
			"anchor_rn":     nil,
			"messages":      []any{},
		}, nil
	}
	defer rows.Close()
	var msgs []map[string]any
	var anchorRN *int64
	for rows.Next() {
		var id int64
		var rn int64
		var anchorR int64
		var msgDate sqlutil.NullDBTime
		var senderName, senderID, typ, text, service, subject *string
		if err := rows.Scan(&id, &msgDate, &senderName, &senderID, &typ, &text, &service, &subject, &rn, &anchorR); err != nil {
			continue
		}
		if anchorRN == nil {
			v := anchorR
			anchorRN = &v
		}
		m := map[string]any{
			"id":           id,
			"position":     rn,
			"message_date": nil,
			"sender_name":  strVal(senderName, "Unknown"),
			"sender_id":    strVal(senderID, ""),
			"type":         strVal(typ, ""),
			"text":         strVal(text, ""),
			"service":      strVal(service, ""),
			"subject":      strVal(subject, ""),
			"is_anchor":    id == messageID,
		}
		if msgDate.Valid {
			m["message_date"] = msgDate.Time.Format(time.RFC3339)
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return map[string]any{
			"error":         err.Error(),
			"chat_session":  chatSession,
			"message_id":    messageID,
			"message_count": 0,
			"anchor_rn":     nil,
			"messages":      []any{},
		}, nil
	}
	if msgs == nil {
		msgs = []map[string]any{}
	}
	out := map[string]any{
		"chat_session":   chatSession,
		"message_id":     messageID,
		"neighbor_limit": chatMessageNeighborCount,
		"message_count":  len(msgs),
		"messages":       msgs,
		"anchor_found":   len(msgs) > 0,
	}
	if anchorRN != nil {
		out["anchor_rn"] = *anchorRN
	} else {
		out["anchor_rn"] = nil
		out["error"] = "message not found in this chat session, or session has no matching messages"
	}
	return out, nil
}

func getEmailsByContact(ctx context.Context, pool *sql.DB, name string) (map[string]any, error) {
	pattern := "%" + name + "%"
	q, args := toolsUIDFilter(ctx,
		`SELECT id, date, from_address, to_addresses, subject, plain_text, snippet, has_attachments
		 FROM emails WHERE (from_address LIKE ? OR to_addresses LIKE ?) ORDER BY date ASC LIMIT 500`,
		[]any{pattern})
	rows, err := pool.QueryContext(ctx, q, args...)
	if err != nil {
		return map[string]any{"error": err.Error(), "contact_name": name, "email_count": 0, "emails": []any{}}, nil
	}
	defer rows.Close()
	var emails []map[string]any
	for rows.Next() {
		var id int64
		var date sqlutil.NullDBTime
		var from, to, subject, plainText, snippet *string
		var hasAttachments bool
		if err := rows.Scan(&id, &date, &from, &to, &subject, &plainText, &snippet, &hasAttachments); err != nil {
			continue
		}
		text := ""
		if plainText != nil && *plainText != "" {
			text = *plainText
		} else if snippet != nil {
			text = *snippet
		}
		e := map[string]any{
			"id":              id,
			"date":            nil,
			"from_address":    strVal(from, ""),
			"to_addresses":    strVal(to, ""),
			"subject":         strVal(subject, ""),
			"plain_text":      text,
			"has_attachments": hasAttachments,
		}
		if date.Valid {
			e["date"] = date.Time.Format(time.RFC3339)
		}
		emails = append(emails, e)
	}
	if emails == nil {
		emails = []map[string]any{}
	}
	return map[string]any{
		"contact_name": name,
		"email_count":  len(emails),
		"emails":       emails,
	}, nil
}

func getSubjectWritingExamples(ctx context.Context, pool *sql.DB, subjectName string) (map[string]any, error) {
	q, args := toolsUIDFilter(ctx,
		`SELECT id, message_date, sender_name, sender_id, type, text, service, subject
		 FROM messages WHERE sender_id = ? AND type = 'text' ORDER BY RANDOM() LIMIT 200`,
		[]any{subjectName})
	rows, err := pool.QueryContext(ctx, q, args...)
	if err != nil {
		return map[string]any{"error": err.Error()}, nil
	}
	defer rows.Close()
	var msgs []map[string]any
	for rows.Next() {
		var id int64
		var msgDate sqlutil.NullDBTime
		var senderName, senderID, typ, text, service, subject *string
		if err := rows.Scan(&id, &msgDate, &senderName, &senderID, &typ, &text, &service, &subject); err != nil {
			continue
		}
		m := map[string]any{
			"id":           id,
			"message_date": nil,
			"sender_name":  strVal(senderName, "Unknown"),
			"sender_id":    strVal(senderID, ""),
			"type":         strVal(typ, ""),
			"text":         strVal(text, ""),
			"service":      strVal(service, ""),
			"subject":      strVal(subject, ""),
		}
		if msgDate.Valid {
			m["message_date"] = msgDate.Time.Format(time.RFC3339)
		}
		msgs = append(msgs, m)
	}
	if msgs == nil {
		msgs = []map[string]any{}
	}
	return map[string]any{"subject_writing_examples": msgs}, nil
}

func getAllMessagesByContact(ctx context.Context, pool *sql.DB, name string) (map[string]any, error) {
	messages, _ := GetMessagesForContactProfile(ctx, pool, name)
	emails, _ := getEmailsByContact(ctx, pool, name)
	messagesJSON, _ := json.Marshal(messages)
	emailsJSON, _ := json.Marshal(emails)
	return map[string]any{
		"contact_name":        name,
		"relationshipSummary": fmt.Sprintf("Messages: %s\nEmails: %s", string(messagesJSON), string(emailsJSON)),
	}, nil
}

func getUniqueTagsCount(ctx context.Context, pool *sql.DB) (map[string]any, error) {
	mediaTags := map[string]struct{}{}
	q, args := toolsUIDFilter(ctx, `SELECT tags FROM media_items WHERE tags IS NOT NULL AND tags != ''`, nil)
	rows, err := pool.QueryContext(ctx, q, args...)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var raw string
			if err := rows.Scan(&raw); err != nil {
				slog.Warn("getUniqueTagsCount: media_items scan", "err", err)
				continue
			}
			for _, t := range strings.Split(raw, ",") {
				if s := strings.TrimSpace(t); s != "" {
					mediaTags[s] = struct{}{}
				}
			}
		}
		if err := rows.Err(); err != nil {
			slog.Warn("getUniqueTagsCount: media_items rows", "err", err)
		}
	}
	artefactTags := map[string]struct{}{}
	q2, args2 := toolsUIDFilter(ctx, `SELECT tags FROM artefacts WHERE tags IS NOT NULL AND tags != ''`, nil)
	rows2, err := pool.QueryContext(ctx, q2, args2...)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var raw string
			if err := rows2.Scan(&raw); err != nil {
				slog.Warn("getUniqueTagsCount: artefacts scan", "err", err)
				continue
			}
			for _, t := range strings.Split(raw, ",") {
				if s := strings.TrimSpace(t); s != "" {
					artefactTags[s] = struct{}{}
				}
			}
		}
		if err := rows2.Err(); err != nil {
			slog.Warn("getUniqueTagsCount: artefacts rows", "err", err)
		}
	}
	combined := map[string]struct{}{}
	for k := range mediaTags {
		combined[k] = struct{}{}
	}
	for k := range artefactTags {
		combined[k] = struct{}{}
	}
	return map[string]any{
		"media_items_unique_tag_count": len(mediaTags),
		"media_items_tags":             sortedKeys(mediaTags),
		"artefacts_unique_tag_count":   len(artefactTags),
		"artefacts_tags":               sortedKeys(artefactTags),
		"combined_unique_tag_count":    len(combined),
		"combined_tags":                sortedKeys(combined),
	}, nil
}

func searchFacebookAlbums(ctx context.Context, pool *sql.DB, keyword string) (map[string]any, error) {
	pattern := "%" + keyword + "%"
	q, args := toolsUIDFilter(ctx,
		`SELECT id, name, description, cover_photo_uri, last_modified_timestamp
		 FROM facebook_albums WHERE name LIKE ? OR description LIKE ? ORDER BY name ASC`,
		[]any{pattern})
	rows, err := pool.QueryContext(ctx, q, args...)
	if err != nil {
		return map[string]any{"error": err.Error(), "albums": []any{}, "count": 0}, nil
	}
	defer rows.Close()
	var albums []map[string]any
	for rows.Next() {
		var id int64
		var name, desc, coverURI *string
		var lastMod *time.Time
		if err := rows.Scan(&id, &name, &desc, &coverURI, &lastMod); err != nil {
			continue
		}
		a := map[string]any{
			"id":              id,
			"name":            strVal(name, ""),
			"description":     strVal(desc, ""),
			"cover_photo_uri": strVal(coverURI, ""),
			"last_modified":   nil,
		}
		if lastMod != nil {
			a["last_modified"] = lastMod.Format(time.RFC3339)
		}
		albums = append(albums, a)
	}
	if albums == nil {
		albums = []map[string]any{}
	}
	return map[string]any{"keyword": keyword, "count": len(albums), "albums": albums}, nil
}

func getAlbumImagesTool(ctx context.Context, pool *sql.DB, albumID int64) (map[string]any, error) {
	uid := appctx.UserIDFromCtx(ctx)
	q := `SELECT mi.id, mi.title, mi.year, mi.month
          FROM media_items mi
          JOIN album_media am ON mi.id = am.media_item_id
          WHERE am.album_id = ?`
	args := []any{albumID}
	if uid > 0 {
		args = append(args, uid)
		q += " AND mi.user_id = ?"
	}
	q += " ORDER BY mi.created_at ASC LIMIT 5"

	rows, err := pool.QueryContext(ctx, q, args...)
	if err != nil {
		return map[string]any{"error": err.Error(), "images": []any{}, "count": 0}, nil
	}
	defer rows.Close()
	var images []map[string]any
	for rows.Next() {
		var id int64
		var title *string
		var year, month *int
		if err := rows.Scan(&id, &title, &year, &month); err != nil {
			continue
		}
		entry := map[string]any{"id": id}
		if title != nil {
			entry["title"] = *title
		}
		if year != nil {
			entry["year"] = *year
		}
		if month != nil {
			entry["month"] = *month
		}
		images = append(images, entry)
	}
	if images == nil {
		images = []map[string]any{}
	}
	return map[string]any{"album_id": albumID, "count": len(images), "images": images}, nil
}

func searchFacebookPosts(ctx context.Context, pool *sql.DB, description string) (map[string]any, error) {
	pattern := "%" + description + "%"
	q, args := toolsUIDFilter(ctx,
		`SELECT id, title, post_text FROM facebook_posts WHERE post_text LIKE ? ORDER BY timestamp DESC`,
		[]any{pattern})
	rows, err := pool.QueryContext(ctx, q, args...)
	if err != nil {
		return map[string]any{"error": err.Error(), "posts": []any{}, "count": 0}, nil
	}
	defer rows.Close()
	var posts []map[string]any
	for rows.Next() {
		var id int64
		var title, postText *string
		if err := rows.Scan(&id, &title, &postText); err != nil {
			continue
		}
		posts = append(posts, map[string]any{
			"id":          id,
			"title":       strVal(title, ""),
			"description": strVal(postText, ""),
		})
	}
	if posts == nil {
		posts = []map[string]any{}
	}
	return map[string]any{"description": description, "count": len(posts), "posts": posts}, nil
}

func getAllFacebookPosts(ctx context.Context, pool *sql.DB) (map[string]any, error) {
	q, args := toolsUIDFilter(ctx,
		`SELECT id, timestamp, title, post_text, external_url, post_type FROM facebook_posts ORDER BY timestamp DESC LIMIT 500`,
		nil)
	rows, err := pool.QueryContext(ctx, q, args...)
	if err != nil {
		return map[string]any{"error": err.Error(), "posts": []any{}, "count": 0}, nil
	}
	defer rows.Close()
	var posts []map[string]any
	for rows.Next() {
		var id int64
		var ts *time.Time
		var title, postText, extURL, postType *string
		if err := rows.Scan(&id, &ts, &title, &postText, &extURL, &postType); err != nil {
			continue
		}
		p := map[string]any{
			"id":           id,
			"timestamp":    nil,
			"title":        strVal(title, ""),
			"description":  strVal(postText, ""),
			"external_url": strVal(extURL, ""),
			"post_type":    strVal(postType, ""),
		}
		if ts != nil {
			p["timestamp"] = ts.Format(time.RFC3339)
		}
		posts = append(posts, p)
	}
	if posts == nil {
		posts = []map[string]any{}
	}
	return map[string]any{"count": len(posts), "posts": posts}, nil
}

func getUserInterests(ctx context.Context, pool *sql.DB) (map[string]any, error) {
	q, args := toolsUIDFilter(ctx, `SELECT name FROM interests ORDER BY name`, nil)
	rows, err := pool.QueryContext(ctx, q, args...)
	if err != nil {
		return map[string]any{"error": err.Error(), "interests": []any{}}, nil
	}
	defer rows.Close()
	var interests []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			slog.Warn("getUserInterests: scan", "err", err)
			continue
		}
		if name != "" {
			interests = append(interests, name)
		}
	}
	if err := rows.Err(); err != nil {
		slog.Warn("getUserInterests: rows", "err", err)
	}
	if interests == nil {
		interests = []string{}
	}
	return map[string]any{"interests": interests, "count": len(interests)}, nil
}

// listAvailableChatSessions lists all chat sessions (conversations) by name and optional ID.
// Used by the tool "get_available_chat_sessions".
func listAvailableChatSessions(ctx context.Context, pool *sql.DB) (map[string]any, error) {
	q, args := toolsUIDFilter(ctx, `
		SELECT DISTINCT chat_session
		FROM messages
		WHERE chat_session IS NOT NULL AND chat_session <> ''
		ORDER BY chat_session
	`, nil)
	rows, err := pool.QueryContext(ctx, q, args...)
	if err != nil {
		return map[string]any{"error": err.Error(), "chat_sessions": []any{}}, nil
	}
	defer rows.Close()
	var sessions []map[string]any
	for rows.Next() {
		var chatSession string
		if err := rows.Scan(&chatSession); err != nil {
			continue
		}
		sessions = append(sessions, map[string]any{
			"name": chatSession,
		})
	}
	if sessions == nil {
		sessions = []map[string]any{}
	}
	return map[string]any{"count": len(sessions), "chat_sessions": sessions}, nil
}

// searchChatMessagesGlobally finds messages in any chat whose body or subject matches keyword (LIKE).
// Each match returns chat_session_id (messages.chat_session string), message_id, and content (text, or subject if text empty).
func searchChatMessagesGlobally(ctx context.Context, pool *sql.DB, keyword string) (map[string]any, error) {
	if strings.TrimSpace(keyword) == "" {
		return map[string]any{"error": "keyword is required", "keyword": keyword, "matches": []any{}, "count": 0}, nil
	}
	pat := "%" + keyword + "%"
	q, args := toolsUIDFilter(ctx, fmt.Sprintf(`
		SELECT id, chat_session, text, subject
		FROM messages
		WHERE chat_session IS NOT NULL AND TRIM(chat_session) <> ''
		  AND (COALESCE(text, '') LIKE ? OR COALESCE(subject, '') LIKE ?)
		ORDER BY message_date ASC NULLS LAST, id ASC
		LIMIT %d`, chatMessageKeywordSearchLimit), []any{pat})
	return execChatMessageKeywordSearch(ctx, pool, q, args, keyword, "", true)
}

// searchChatMessagesInSession is like searchChatMessagesGlobally but only within one chat_session (LIKE on session name, same as get_imessages_by_chat_session).
func searchChatMessagesInSession(ctx context.Context, pool *sql.DB, chatSession, keyword string) (map[string]any, error) {
	if strings.TrimSpace(chatSession) == "" || strings.TrimSpace(keyword) == "" {
		return map[string]any{"error": "chat_session and keyword are required", "chat_session": chatSession, "keyword": keyword, "matches": []any{}, "count": 0}, nil
	}
	sessPat := "%" + chatSession + "%"
	kwPat := "%" + keyword + "%"
	q, args := toolsUIDFilter(ctx, fmt.Sprintf(`
		SELECT id, chat_session, text, subject
		FROM messages
		WHERE chat_session LIKE ?
		  AND (COALESCE(text, '') LIKE ? OR COALESCE(subject, '') LIKE ?)
		ORDER BY message_date ASC NULLS LAST, id ASC
		LIMIT %d`, chatMessageKeywordSearchLimit), []any{sessPat, kwPat})
	return execChatMessageKeywordSearch(ctx, pool, q, args, keyword, chatSession, false)
}

func execChatMessageKeywordSearch(ctx context.Context, pool *sql.DB, q string, args []any, keyword, chatSession string, global bool) (map[string]any, error) {
	rows, err := pool.QueryContext(ctx, q, args...)
	if err != nil {
		out := map[string]any{"error": err.Error(), "keyword": keyword, "matches": []any{}, "count": 0}
		if !global {
			out["chat_session"] = chatSession
		}
		return out, nil
	}
	defer rows.Close()
	var matches []map[string]any
	for rows.Next() {
		var id int64
		var cs *string
		var text, subject *string
		if err := rows.Scan(&id, &cs, &text, &subject); err != nil {
			continue
		}
		body := strVal(text, "")
		if body == "" {
			body = strVal(subject, "")
		}
		matches = append(matches, map[string]any{
			"chat_session_id": strVal(cs, ""),
			"message_id":      id,
			"content":         body,
		})
	}
	if err := rows.Err(); err != nil {
		out := map[string]any{"error": err.Error(), "keyword": keyword, "matches": []any{}, "count": 0}
		if !global {
			out["chat_session"] = chatSession
		}
		return out, nil
	}
	if matches == nil {
		matches = []map[string]any{}
	}
	out := map[string]any{
		"keyword": keyword,
		"count":   len(matches),
		"matches": matches,
	}
	if !global {
		out["chat_session"] = chatSession
	}
	return out, nil
}

func getReferenceDocuments(ctx context.Context, pool *sql.DB, ids []int64, pepper string, getRAM RAMMasterGetter) (map[string]any, error) {
	masterPassword, haveMaster := "", false
	if getRAM != nil {
		masterPassword, haveMaster = getRAM()
	}
	var results []map[string]any
	for _, id := range ids {
		if visitorKeyReferenceDocsRestricted(ctx) {
			allowed, gerr := visitorKeyMayReadReferenceDoc(ctx, pool, id)
			if gerr != nil {
				results = append(results, map[string]any{"id": id, "error": gerr.Error()})
				continue
			}
			if !allowed {
				results = append(results, map[string]any{"id": id, "error": "not authorized"})
				continue
			}
		}
		var title, filename, contentType *string
		var data []byte
		var isEncrypted bool
		q, args := toolsUIDFilter(ctx,
			`SELECT title, filename, content_type, data, is_encrypted FROM reference_documents WHERE id = ? AND is_sensitive = FALSE`,
			[]any{id})
		err := pool.QueryRowContext(ctx, q, args...).Scan(&title, &filename, &contentType, &data, &isEncrypted)
		if err != nil {
			results = append(results, map[string]any{"id": id, "error": "not found"})
			continue
		}
		ct := ""
		if contentType != nil {
			ct = *contentType
		}
		displayTitle := strVal(title, strVal(filename, ""))
		if isEncrypted {
			if !haveMaster || masterPassword == "" {
				results = append(results, map[string]any{"id": id, "title": displayTitle, "content": "[encrypted — master key not unlocked in this session]"})
				continue
			}
			plain, err := appcrypto.DecryptDocumentData(ctx, pool, masterPassword, data, pepper)
			if err != nil || len(plain) == 0 {
				results = append(results, map[string]any{"id": id, "title": displayTitle, "content": "[encrypted — decryption failed]"})
				continue
			}
			data = plain
		}
		if ct == "application/pdf" {
			results = append(results, map[string]any{
				"id":      id,
				"title":   displayTitle,
				"content": "[PDF document — not renderable as text]",
			})
		} else {
			results = append(results, map[string]any{
				"id":      id,
				"title":   displayTitle,
				"content": string(data),
			})
		}
	}
	if results == nil {
		results = []map[string]any{}
	}
	return map[string]any{"documents": results}, nil
}

func getAvailableSensitiveReferenceDocuments(ctx context.Context, pool *sql.DB) (map[string]any, error) {
	if visitorKeySensitiveLLMAccessDenied(ctx) {
		return map[string]any{"documents": []map[string]any{}}, nil
	}
	va := appctx.VisitorAccessFromCtx(ctx)
	if va.Restricted {
		// Visitor key hint junction tables are not present in the SQLite build.
		return map[string]any{"documents": []map[string]any{}}, nil
	}
	q, args := toolsUIDFilter(ctx, `SELECT id, title, description, tags FROM reference_documents WHERE is_sensitive = TRUE AND NOT include_in_system_prompt`, nil)
	rows, err := pool.QueryContext(ctx, q, args...)
	if err != nil {
		return map[string]any{"error": err.Error(), "documents": []any{}}, nil
	}
	defer rows.Close()
	var documents []map[string]any
	for rows.Next() {
		var id int64
		var title, description, tags *string
		if err := rows.Scan(&id, &title, &description, &tags); err != nil {
			continue
		}
		documents = append(documents, map[string]any{
			"id":          id,
			"title":       title,
			"description": description,
			"tags":        tags,
		})
	}
	if documents == nil {
		documents = []map[string]any{}
	}
	return map[string]any{"documents": documents}, nil
}

func getSensitiveReferenceDocuments(ctx context.Context, pool *sql.DB, ids []int64, pepper string, getRAM RAMMasterGetter) (map[string]any, error) {
	masterPassword, haveMaster := "", false
	if getRAM != nil {
		masterPassword, haveMaster = getRAM()
	}
	var results []map[string]any
	for _, id := range ids {
		if visitorKeySensitiveLLMAccessDenied(ctx) {
			results = append(results, map[string]any{"id": id, "error": "not authorized"})
			continue
		}
		allowed, gerr := visitorKeyMayReadSensitiveReferenceDoc(ctx, pool, id)
		if gerr != nil {
			results = append(results, map[string]any{"id": id, "error": gerr.Error()})
			continue
		}
		if !allowed {
			results = append(results, map[string]any{"id": id, "error": "not authorized"})
			continue
		}
		var title, filename, contentType *string
		var data []byte
		var isEncrypted bool
		q, args := toolsUIDFilter(ctx,
			`SELECT title, filename, content_type, data, is_encrypted FROM reference_documents WHERE id = ? AND is_sensitive = TRUE`,
			[]any{id})
		err := pool.QueryRowContext(ctx, q, args...).Scan(&title, &filename, &contentType, &data, &isEncrypted)
		if err != nil {
			results = append(results, map[string]any{"id": id, "error": "not found"})
			continue
		}
		ct := ""
		if contentType != nil {
			ct = *contentType
		}
		displayTitle := strVal(title, strVal(filename, ""))
		if isEncrypted {
			if !haveMaster || masterPassword == "" {
				results = append(results, map[string]any{"id": id, "title": displayTitle, "content": "[encrypted — master key not unlocked in this session]"})
				continue
			}
			plain, err := appcrypto.DecryptDocumentData(ctx, pool, masterPassword, data, pepper)
			if err != nil || len(plain) == 0 {
				results = append(results, map[string]any{"id": id, "title": displayTitle, "content": "[encrypted — decryption failed]"})
				continue
			}
			data = plain
		}
		if ct == "application/pdf" {
			results = append(results, map[string]any{
				"id":      id,
				"title":   displayTitle,
				"content": "[PDF document — not renderable as text]",
			})
		} else {
			results = append(results, map[string]any{
				"id":      id,
				"title":   displayTitle,
				"content": string(data),
			})
		}
	}
	if results == nil {
		results = []map[string]any{}
	}
	return map[string]any{"documents": results}, nil
}

func searchTavily(tavilyKey, query string) (map[string]any, error) {
	if tavilyKey == "" {
		return map[string]any{"error": "Tavily search not configured (TAVILY_API_KEY missing)"}, nil
	}
	body, _ := json.Marshal(map[string]any{
		"api_key":      tavilyKey,
		"query":        query,
		"search_depth": "advanced",
		"max_results":  5,
	})
	resp, err := http.Post("https://api.tavily.com/search", "application/json", bytes.NewReader(body))
	if err != nil {
		return map[string]any{"error": err.Error(), "query": query}, nil
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var result map[string]any
	if err := json.Unmarshal(b, &result); err != nil {
		return map[string]any{"error": "invalid response", "query": query}, nil
	}
	return result, nil
}

// strVal dereferences a *string or returns def.
func strVal(s *string, def string) string {
	if s != nil {
		return *s
	}
	return def
}

// sortedKeys returns the keys of a set map in sorted order.
func sortedKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// listInterviewsTool returns a summary list of interviews for the AI tool.
func listInterviewsTool(ctx context.Context, pool *sql.DB, stateFilter string) (map[string]any, error) {
	q := `SELECT i.id, i.title, i.style, i.purpose, i.purpose_detail, i.state,
	             (i.writeup IS NOT NULL AND LENGTH(TRIM(COALESCE(i.writeup, ''))) > 0) AS has_writeup,
	             COALESCE((SELECT COUNT(*) FROM interview_turns WHERE interview_id = i.id), 0) AS turn_count,
	             i.created_at, COALESCE(i.last_turn_at, i.created_at) AS last_activity
	      FROM interviews i WHERE TRUE`
	args := []any{}
	uid := appctx.UserIDFromCtx(ctx)
	if uid > 0 {
		args = append(args, uid)
		q += " AND i.user_id = ?"
	}
	if stateFilter != "" {
		args = append(args, stateFilter)
		q += " AND i.state = ?"
	}
	q += " ORDER BY last_activity DESC"
	rows, err := pool.QueryContext(ctx, q, args...)
	if err != nil {
		return map[string]any{"error": err.Error()}, nil
	}
	defer rows.Close()

	var interviews []map[string]any
	for rows.Next() {
		var id int64
		var title, style, purpose, state string
		var purposeDetail *string
		var hasWriteup bool
		var turnCount int
		var createdAt, lastActivity time.Time
		if err := rows.Scan(&id, &title, &style, &purpose, &purposeDetail, &state,
			&hasWriteup, &turnCount, &createdAt, &lastActivity); err != nil {
			return map[string]any{"error": err.Error()}, nil
		}
		entry := map[string]any{
			"id":            id,
			"title":         title,
			"style":         style,
			"purpose":       purpose,
			"state":         state,
			"has_writeup":   hasWriteup,
			"turn_count":    turnCount,
			"created_at":    createdAt.Format(time.RFC3339),
			"last_activity": lastActivity.Format(time.RFC3339),
		}
		if purposeDetail != nil {
			entry["purpose_detail"] = *purposeDetail
		}
		interviews = append(interviews, entry)
	}
	if interviews == nil {
		interviews = []map[string]any{}
	}
	return map[string]any{"interviews": interviews, "count": len(interviews)}, nil
}

// getInterviewTool returns full interview details including transcript and writeup.
func getInterviewTool(ctx context.Context, pool *sql.DB, interviewID int64) (map[string]any, error) {
	uid := appctx.UserIDFromCtx(ctx)
	q := `SELECT i.id, i.title, i.style, i.purpose, i.purpose_detail, i.state,
	             i.writeup, i.created_at, i.updated_at
	      FROM interviews i WHERE i.id = ?`
	args := []any{interviewID}
	if uid > 0 {
		args = append(args, uid)
		q += " AND i.user_id = ?"
	}

	var id int64
	var title, style, purpose, state string
	var purposeDetail, writeup *string
	var createdAt, updatedAt time.Time
	err := pool.QueryRowContext(ctx, q, args...).Scan(
		&id, &title, &style, &purpose, &purposeDetail, &state,
		&writeup, &createdAt, &updatedAt,
	)
	if err != nil {
		return map[string]any{"error": fmt.Sprintf("interview %d not found", interviewID)}, nil
	}

	interview := map[string]any{
		"id":         id,
		"title":      title,
		"style":      style,
		"purpose":    purpose,
		"state":      state,
		"created_at": createdAt.Format(time.RFC3339),
		"updated_at": updatedAt.Format(time.RFC3339),
	}
	if purposeDetail != nil {
		interview["purpose_detail"] = *purposeDetail
	}
	if writeup != nil {
		interview["writeup"] = *writeup
	}

	trows, err := pool.QueryContext(ctx,
		`SELECT turn_number, question, answer FROM interview_turns
		 WHERE interview_id = ? ORDER BY turn_number ASC`, interviewID)
	if err != nil {
		interview["turns_error"] = err.Error()
		return interview, nil
	}
	defer trows.Close()

	var turns []map[string]any
	for trows.Next() {
		var turnNum int
		var question string
		var answer *string
		if err := trows.Scan(&turnNum, &question, &answer); err != nil {
			continue
		}
		t := map[string]any{
			"turn_number": turnNum,
			"question":    question,
		}
		if answer != nil {
			t["answer"] = *answer
		}
		turns = append(turns, t)
	}
	if turns == nil {
		turns = []map[string]any{}
	}
	interview["turns"] = turns
	return interview, nil
}

// listCompleteProfilesTool returns name and pending flag for each complete_profiles row (user-scoped).
func listCompleteProfilesTool(ctx context.Context, pool *sql.DB) (map[string]any, error) {
	va := appctx.VisitorAccessFromCtx(ctx)
	if va.Restricted && !va.AllowRelationships() {
		return map[string]any{"profiles": []any{}, "count": 0}, nil
	}
	q := `SELECT name, generation_pending FROM complete_profiles WHERE name IS NOT NULL AND name != ''`
	args := []any{}
	q, args = toolsUIDFilter(ctx, q, args)
	q += " ORDER BY name"
	rows, err := pool.QueryContext(ctx, q, args...)
	if err != nil {
		return map[string]any{"error": err.Error()}, nil
	}
	defer rows.Close()
	var profiles []map[string]any
	for rows.Next() {
		var name string
		var pending bool
		if err := rows.Scan(&name, &pending); err != nil {
			return map[string]any{"error": err.Error()}, nil
		}
		profiles = append(profiles, map[string]any{"name": name, "pending": pending})
	}
	if profiles == nil {
		profiles = []map[string]any{}
	}
	return map[string]any{"profiles": profiles, "count": len(profiles)}, nil
}

// getCompleteProfileTool returns the profile text for one name (user-scoped).
func getCompleteProfileTool(ctx context.Context, pool *sql.DB, name string) (map[string]any, error) {
	va := appctx.VisitorAccessFromCtx(ctx)
	if va.Restricted && !va.AllowRelationships() {
		return map[string]any{"error": "not authorized for visitor key"}, nil
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return map[string]any{"error": "name is required"}, nil
	}
	q := `SELECT profile, generation_pending FROM complete_profiles WHERE name = ?`
	args := []any{name}
	q, args = toolsUIDFilter(ctx, q, args)
	var prof *string
	var pending bool
	err := pool.QueryRowContext(ctx, q, args...).Scan(&prof, &pending)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return map[string]any{"error": fmt.Sprintf("no complete profile found for %q", name)}, nil
		}
		return map[string]any{"error": err.Error()}, nil
	}
	out := map[string]any{
		"name":    name,
		"pending": pending,
	}
	if prof != nil {
		out["profile"] = *prof
	} else {
		out["profile"] = ""
	}
	return out, nil
}
