// Package imap provides IMAP email import functionality.
package imap

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"
	"time"
	"unicode/utf8"

	goImap "github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/htmlindex"
)

// ConnParams holds IMAP server connection parameters.
type ConnParams struct {
	Host     string
	Port     int
	Username string
	Password string
	UseSSL   bool
}

// ProgressFunc is called with progress updates during folder import.
type ProgressFunc func(folder string, folderIdx, totalFolders, emailsProcessed int)

// Connect opens an authenticated IMAP connection.
func Connect(p ConnParams) (*client.Client, error) {
	addr := fmt.Sprintf("%s:%d", p.Host, p.Port)
	var (
		c   *client.Client
		err error
	)
	if p.UseSSL {
		c, err = client.DialTLS(addr, &tls.Config{ServerName: p.Host})
	} else {
		c, err = client.Dial(addr)
	}
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	if err := c.Login(p.Username, p.Password); err != nil {
		_ = c.Logout()
		return nil, fmt.Errorf("login: %w", err)
	}
	return c, nil
}

// ListFolders connects, lists all mailboxes, and disconnects.
func ListFolders(p ConnParams) ([]string, error) {
	c, err := Connect(p)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Logout() }()

	mailboxes := make(chan *goImap.MailboxInfo, 64)
	done := make(chan error, 1)
	go func() {
		done <- c.List("", "*", mailboxes)
	}()

	var folders []string
	for m := range mailboxes {
		folders = append(folders, m.Name)
	}
	if err := <-done; err != nil {
		return nil, err
	}
	return folders, nil
}

// ImportFolders imports emails from the given folders into the database.
// cancelFn is polled for cancellation; progressFn is called with progress updates.
// Returns the total number of emails stored.
func ImportFolders(ctx context.Context, pool *pgxpool.Pool, p ConnParams, folders []string, newOnly bool, cancelFn func() bool, progressFn ProgressFunc) (int, error) {
	c, err := Connect(p)
	if err != nil {
		return 0, err
	}
	defer func() { _ = c.Logout() }()

	totalProcessed := 0
	for idx, folder := range folders {
		if cancelFn != nil && cancelFn() {
			return totalProcessed, nil
		}
		if progressFn != nil {
			progressFn(folder, idx+1, len(folders), totalProcessed)
		}
		count, err := importFolder(ctx, c, pool, folder, newOnly, cancelFn)
		if err != nil {
			return totalProcessed, fmt.Errorf("folder %q: %w", folder, err)
		}
		totalProcessed += count
		if progressFn != nil {
			progressFn(folder, idx+1, len(folders), totalProcessed)
		}
	}
	return totalProcessed, nil
}

// importFolder fetches and stores all messages in one IMAP folder.
func importFolder(ctx context.Context, c *client.Client, pool *pgxpool.Pool, folder string, newOnly bool, cancelFn func() bool) (int, error) {
	mbox, err := c.Select(folder, true /* readonly */)
	if err != nil {
		return 0, fmt.Errorf("select: %w", err)
	}
	if mbox.Messages == 0 {
		return 0, nil
	}

	seqset := new(goImap.SeqSet)
	seqset.AddRange(1, mbox.Messages)

	items := []goImap.FetchItem{
		goImap.FetchEnvelope,
		goImap.FetchFlags,
		goImap.FetchRFC822,
	}

	messages := make(chan *goImap.Message, 64)
	fetchDone := make(chan error, 1)
	go func() {
		fetchDone <- c.Fetch(seqset, items, messages)
	}()

	count := 0
	for msg := range messages {
		if cancelFn != nil && cancelFn() {
			break
		}
		if err := storeEmail(ctx, pool, folder, msg, newOnly); err != nil {
			fmt.Printf("[IMAP] warning storing email %d: %s\n", msg.SeqNum, err)
			continue
		}
		count++
	}
	if err := <-fetchDone; err != nil {
		return count, fmt.Errorf("fetch: %w", err)
	}
	return count, nil
}

// storeEmail persists one IMAP message to the database.
func storeEmail(ctx context.Context, pool *pgxpool.Pool, folder string, msg *goImap.Message, newOnly bool) error {
	if msg.Envelope == nil {
		return nil
	}
	env := msg.Envelope
	uid := fmt.Sprintf("%d", msg.SeqNum)

	if newOnly {
		var exists bool
		_ = pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM emails WHERE uid=$1 AND folder=$2)`,
			uid, folder,
		).Scan(&exists)
		if exists {
			return nil
		}
	}

	from := addrList(env.From)
	to := addrList(env.To)
	cc := addrList(env.Cc)
	bcc := addrList(env.Bcc)

	var rawMsg *string
	var plainText *string
	var snippet *string
	var attachments []attachmentPart

	section, secErr := goImap.ParseBodySectionName(goImap.FetchRFC822)
	if secErr == nil {
		if body := msg.GetBody(section); body != nil {
			raw, rerr := io.ReadAll(body)
			if rerr == nil && len(raw) > 0 {
				parsed := parseMIMEBody(raw)
				rawMsg, plainText, snippet = emailStoredFields(parsed)
				attachments = parsed.Attachments
			}
		}
	}

	date := env.Date
	if date.IsZero() {
		date = time.Now()
	}

	hasAttach := false
	for _, att := range attachments {
		if len(att.Data) > 0 {
			hasAttach = true
			break
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var emailID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO emails (uid, folder, subject, from_address, to_addresses, cc_addresses, bcc_addresses,
		                    date, raw_message, plain_text, snippet, has_attachments,
		                    user_deleted, is_personal, is_business, is_social, is_promotional,
		                    is_spam, is_important, use_by_ai)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,FALSE,FALSE,FALSE,FALSE,FALSE,FALSE,FALSE,FALSE)
		ON CONFLICT (uid, folder) DO UPDATE
		SET subject=$3, from_address=$4, to_addresses=$5, cc_addresses=$6, bcc_addresses=$7,
		    date=$8, raw_message=$9, plain_text=$10, snippet=$11, has_attachments=$12,
		    updated_at=NOW()
		RETURNING id`,
		uid, folder, ensureUTF8(env.Subject), ensureUTF8(from), ensureUTF8(to), ensureUTF8(cc), ensureUTF8(bcc),
		date, ptrEnsureUTF8(rawMsg), ptrEnsureUTF8(plainText), ptrEnsureUTF8(snippet), hasAttach,
	).Scan(&emailID)
	if err != nil {
		return err
	}

	ref := fmt.Sprintf("%d", emailID)
	if _, err = tx.Exec(ctx, `DELETE FROM media_items WHERE source = 'email_attachment' AND source_reference = $1`, ref); err != nil {
		return err
	}

	for _, att := range attachments {
		if len(att.Data) == 0 {
			continue
		}
		title := ensureUTF8(att.Filename)
		if title == "" {
			title = "attachment"
		}
		title = truncateRunes(title, 1000)
		mt := att.MediaType
		if len(mt) > 255 {
			mt = mt[:255]
		}
		var blobID int64
		if err = tx.QueryRow(ctx, `INSERT INTO media_blobs (image_data, thumbnail_data) VALUES ($1, NULL) RETURNING id`, att.Data).Scan(&blobID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `
			INSERT INTO media_items (
				media_blob_id, title, media_type, source, source_reference,
				processed, available_for_task, rating, has_gps, is_referenced,
				is_personal, is_business, is_social, is_promotional, is_spam, is_important
			) VALUES ($1, $2, $3, 'email_attachment', $4,
				FALSE, FALSE, 5, FALSE, FALSE,
				FALSE, FALSE, FALSE, FALSE, FALSE, FALSE)`,
			blobID, title, mt, ref); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func addrList(addrs []*goImap.Address) string {
	parts := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if a == nil {
			continue
		}
		if a.PersonalName != "" {
			parts = append(parts, fmt.Sprintf("%s <%s@%s>", a.PersonalName, a.MailboxName, a.HostName))
		} else {
			parts = append(parts, fmt.Sprintf("%s@%s", a.MailboxName, a.HostName))
		}
	}
	return strings.Join(parts, ", ")
}

// ── MIME parsing ──────────────────────────────────────────────────────────────

type mimeHeader interface {
	Get(key string) string
}

type attachmentPart struct {
	Filename  string
	MediaType string
	Data      []byte
}

type parsedMIME struct {
	BodyPlain   string
	BodyHTML    string
	Attachments []attachmentPart
	HasAttach   bool
}

func parseMIMEBody(raw []byte) parsedMIME {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return parsedMIME{}
	}

	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		if st, ok := parseMIMEHeaderBody(raw); ok {
			out := parsedMIME{
				BodyPlain:   st.plain,
				BodyHTML:    st.html,
				Attachments: st.attachments,
			}
			out.HasAttach = len(out.Attachments) > 0
			return out
		}
		return parsedMIME{BodyPlain: strings.TrimSpace(decodeMIMEPart(bytes.TrimSpace(raw), ""))}
	}

	body, err := io.ReadAll(msg.Body)
	if err != nil {
		return parsedMIME{}
	}

	st := &parseState{}
	walkMIME(msg.Header, body, st)

	out := parsedMIME{
		BodyPlain:   st.plain,
		BodyHTML:    st.html,
		Attachments: st.attachments,
	}
	out.HasAttach = len(out.Attachments) > 0
	return out
}

type parseState struct {
	plain       string
	html        string
	attachments []attachmentPart
}

func parseMIMEHeaderBody(raw []byte) (*parseState, bool) {
	tp := textproto.NewReader(bufio.NewReader(bytes.NewReader(raw)))
	hdr, err := tp.ReadMIMEHeader()
	if err != nil || len(hdr) == 0 {
		return nil, false
	}
	body, err := io.ReadAll(tp.R)
	if err != nil {
		return nil, false
	}
	st := &parseState{}
	walkMIME(hdr, body, st)
	if st.plain == "" && st.html == "" && len(st.attachments) == 0 {
		return nil, false
	}
	return st, true
}

func walkMIME(h mimeHeader, body []byte, st *parseState) {
	body = decodeTransfer(h.Get("Content-Transfer-Encoding"), body)

	ct := h.Get("Content-Type")
	mt, params, err := mime.ParseMediaType(ct)
	if err != nil {
		mt = strings.TrimSpace(strings.Split(ct, ";")[0])
	}

	switch {
	case strings.HasPrefix(strings.ToLower(mt), "multipart/"):
		boundary := params["boundary"]
		if boundary == "" {
			return
		}
		mr := multipart.NewReader(bytes.NewReader(body), boundary)
		for {
			p, err := mr.NextPart()
			if err != nil {
				break
			}
			sub, err := io.ReadAll(p)
			if err != nil {
				continue
			}
			walkMIME(p.Header, sub, st)
		}

	case strings.HasPrefix(strings.ToLower(mt), "text/plain"):
		if st.plain == "" {
			st.plain = decodeMIMEPart(body, ct)
		}

	case strings.HasPrefix(strings.ToLower(mt), "text/html"):
		if st.html == "" {
			st.html = decodeMIMEPart(body, ct)
		}

	case strings.EqualFold(mt, "message/rfc822"):
		sub, err := mail.ReadMessage(bytes.NewReader(body))
		if err != nil {
			return
		}
		nested, err := io.ReadAll(sub.Body)
		if err != nil {
			return
		}
		walkMIME(sub.Header, nested, st)

	default:
		if shouldSaveAttachment(h, mt) {
			fn := attachFilename(h)
			st.attachments = append(st.attachments, attachmentPart{
				Filename:  fn,
				MediaType: mt,
				Data:      append([]byte(nil), body...),
			})
		}
	}
}

func shouldSaveAttachment(h mimeHeader, mt string) bool {
	mtLower := strings.ToLower(mt)
	if strings.HasPrefix(mtLower, "multipart/") {
		return false
	}
	disp := h.Get("Content-Disposition")
	dispType, dispParams, _ := mime.ParseMediaType(disp)
	dispType = strings.ToLower(dispType)
	if dispType == "attachment" {
		return true
	}
	if dispType == "inline" {
		return false
	}
	fn := dispParams["filename"]
	if fn == "" {
		fn = attachFilename(h)
	}
	if fn != "" && !strings.HasPrefix(mtLower, "text/plain") && !strings.HasPrefix(mtLower, "text/html") {
		return true
	}
	if mt != "" && !strings.HasPrefix(mtLower, "text/") {
		return true
	}
	return false
}

func attachFilename(h mimeHeader) string {
	_, params, err := mime.ParseMediaType(h.Get("Content-Disposition"))
	if err == nil {
		if fn := params["filename"]; fn != "" {
			return mimeDecodeWord(fn)
		}
	}
	_, params, err = mime.ParseMediaType(h.Get("Content-Type"))
	if err == nil {
		if fn := params["name"]; fn != "" {
			return mimeDecodeWord(fn)
		}
	}
	return ""
}

func mimeDecodeWord(s string) string {
	dec := new(mime.WordDecoder)
	out, err := dec.DecodeHeader(s)
	if err != nil {
		return strings.Trim(s, `"`)
	}
	return out
}

func decodeTransfer(enc string, data []byte) []byte {
	switch strings.ToLower(strings.TrimSpace(enc)) {
	case "quoted-printable":
		r := quotedprintable.NewReader(bytes.NewReader(data))
		out, err := io.ReadAll(r)
		if err != nil {
			return data
		}
		return out
	case "base64", "b":
		s := strings.NewReplacer("\r\n", "", "\n", "", " ", "").Replace(string(data))
		out, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return data
		}
		return out
	default:
		return data
	}
}

func emailStoredFields(parsed parsedMIME) (rawMsg, plainText, snippet *string) {
	if parsed.BodyPlain != "" {
		p := ensureUTF8(parsed.BodyPlain)
		plainText = &p
	}
	var raw string
	switch {
	case parsed.BodyHTML != "":
		raw = ensureUTF8(parsed.BodyHTML)
	case parsed.BodyPlain != "":
		raw = ensureUTF8(parsed.BodyPlain)
	}
	if raw != "" {
		rawMsg = &raw
	}

	var snip string
	if parsed.BodyPlain != "" {
		snip = ensureUTF8(parsed.BodyPlain)
	} else if parsed.BodyHTML != "" {
		snip = stripHTML(ensureUTF8(parsed.BodyHTML))
	}
	snip = strings.TrimSpace(snip)
	if snip != "" {
		snip = truncateRunes(snip, 200)
		snip = ensureUTF8(snip)
		snippet = &snip
	}
	return rawMsg, plainText, snippet
}

func stripHTML(html string) string {
	var b strings.Builder
	inTag := false
	for _, r := range html {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
			b.WriteByte(' ')
		default:
			if !inTag {
				b.WriteRune(r)
			}
		}
	}
	return strings.TrimSpace(strings.Join(strings.Fields(b.String()), " "))
}

// ── UTF-8 helpers ─────────────────────────────────────────────────────────────

var replUTF8 = []byte("\uFFFD")

func decodeMIMEPart(body []byte, contentTypeHeader string) string {
	_, params, err := mime.ParseMediaType(contentTypeHeader)
	charset := ""
	if err == nil {
		charset = strings.TrimSpace(params["charset"])
	}
	return bytesToUTF8(body, charset)
}

func bytesToUTF8(body []byte, charset string) string {
	body = bytes.TrimPrefix(body, []byte{0xEF, 0xBB, 0xBF})
	charset = strings.Trim(strings.TrimSpace(charset), `"'`)
	if charset != "" {
		lc := strings.ToLower(charset)
		if lc == "utf-8" || lc == "utf8" {
			if utf8.Valid(body) {
				return string(body)
			}
			return legacyToUTF8(body)
		}
		if enc, err := htmlindex.Get(charset); err == nil {
			out, err2 := enc.NewDecoder().Bytes(body)
			if err2 == nil {
				return string(bytes.ToValidUTF8(out, replUTF8))
			}
		}
	}
	if utf8.Valid(body) {
		return string(body)
	}
	return legacyToUTF8(body)
}

func legacyToUTF8(body []byte) string {
	out, err := charmap.Windows1252.NewDecoder().Bytes(body)
	if err != nil {
		return string(bytes.ToValidUTF8(body, replUTF8))
	}
	return string(bytes.ToValidUTF8(out, replUTF8))
}

func ensureUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return bytesToUTF8([]byte(s), "")
}

func ptrEnsureUTF8(p *string) *string {
	if p == nil {
		return nil
	}
	v := ensureUTF8(*p)
	return &v
}

func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes])
}
