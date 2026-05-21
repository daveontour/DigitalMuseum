package gmail

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	gmailv1 "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

const pageSize = 500

// ErrFetchCancelled is returned when cancelFn passed to FetchMessages reports true before the next message fetch.
var ErrFetchCancelled = errors.New("gmail: fetch cancelled")

// MailAttachment is a non-body MIME part to store as gmail_attachment (same shape as IMAP).
type MailAttachment struct {
	Filename  string
	MediaType string
	Data      []byte
}

// Message holds the parsed content of a single Gmail message.
type Message struct {
	UID         string
	ThreadID    string
	Folder      string // comma-joined label names
	Snippet     string
	Subject     string
	FromAddress string
	ToAddress   string
	Date        time.Time
	BodyText    string
	BodyHTML    string
	Attachments []MailAttachment
}

// Client wraps the Gmail API service.
type Client struct {
	svc *gmailv1.Service
}

// NewClient creates a Client authenticated with the given token.
func NewClient(ctx context.Context, cfg *oauth2.Config, tok *oauth2.Token) (*Client, error) {
	ts := cfg.TokenSource(ctx, tok)
	httpClient := oauth2.NewClient(ctx, ts)
	svc, err := gmailv1.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("gmail service: %w", err)
	}
	return &Client{svc: svc}, nil
}

// NewClientFromHTTP creates a Client from a pre-configured http.Client.
// Useful when the caller manages token refresh externally.
func NewClientFromHTTP(ctx context.Context, httpClient *http.Client) (*Client, error) {
	svc, err := gmailv1.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("gmail service: %w", err)
	}
	return &Client{svc: svc}, nil
}

// ListLabels returns user labels as a map of id → name.
func (c *Client) ListLabels(ctx context.Context) (map[string]string, error) {
	resp, err := c.svc.Users.Labels.List("me").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("list labels: %w", err)
	}
	out := make(map[string]string, len(resp.Labels))
	for _, l := range resp.Labels {
		out[l.Id] = l.Name
	}
	return out, nil
}

// FetchMessages fetches all messages for the given label IDs, returning parsed
// Message structs. progressFn(fetched, estimated) is called after each page.
// Pass newOnlyUIDs as a set of already-imported UIDs to skip full message fetch.
// If cancelFn is non-nil, it is invoked before each message fetch (list stub → full get);
// when it returns true, FetchMessages stops and returns ErrFetchCancelled with messages
// collected so far (caller may discard them).
func (c *Client) FetchMessages(
	ctx context.Context,
	labelIDs []string,
	newOnly bool,
	existingUIDs map[string]struct{},
	progressFn func(fetched, estimated int),
	cancelFn func() bool,
) ([]*Message, error) {
	labelNames, err := c.ListLabels(ctx)
	if err != nil {
		return nil, err
	}

	var messages []*Message
	fetched := 0

	req := c.svc.Users.Messages.List("me").
		LabelIds(labelIDs...).
		MaxResults(int64(pageSize)).
		Context(ctx)

	for {
		if cancelFn != nil && cancelFn() {
			return messages, ErrFetchCancelled
		}
		resp, err := req.Do()
		if err != nil {
			return nil, fmt.Errorf("list messages: %w", err)
		}

		for _, stub := range resp.Messages {
			if cancelFn != nil && cancelFn() {
				return messages, ErrFetchCancelled
			}
			if newOnly {
				if _, exists := existingUIDs[stub.Id]; exists {
					continue
				}
			}

			msg, err := c.fetchOne(ctx, stub.Id, labelNames)
			if err != nil {
				// Skip messages that cannot be fetched rather than aborting.
				continue
			}
			messages = append(messages, msg)
			fetched++
			if progressFn != nil {
				estimated := fetched
				if resp.ResultSizeEstimate > 0 {
					estimated = int(resp.ResultSizeEstimate)
				}
				progressFn(fetched, estimated)
			}
		}

		if resp.NextPageToken == "" {
			break
		}
		req = req.PageToken(resp.NextPageToken)
	}

	return messages, nil
}

// fetchOne retrieves a single message by ID and parses it.
func (c *Client) fetchOne(ctx context.Context, id string, labelNames map[string]string) (*Message, error) {
	raw, err := c.svc.Users.Messages.Get("me", id).
		Format("full").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("get message %s: %w", id, err)
	}

	msg := &Message{
		UID:      raw.Id,
		ThreadID: raw.ThreadId,
		Snippet:  raw.Snippet,
	}

	// Resolve label IDs → names, join with comma.
	var labelNameSlice []string
	for _, lid := range raw.LabelIds {
		if name, ok := labelNames[lid]; ok {
			labelNameSlice = append(labelNameSlice, name)
		}
	}
	msg.Folder = strings.Join(labelNameSlice, ",")

	// Parse headers.
	if raw.Payload != nil {
		for _, h := range raw.Payload.Headers {
			switch strings.ToLower(h.Name) {
			case "subject":
				msg.Subject = h.Value
			case "from":
				msg.FromAddress = h.Value
			case "to":
				msg.ToAddress = h.Value
			case "date":
				if t, err := parseEmailDate(h.Value); err == nil {
					msg.Date = t
				}
			}
		}
		parseParts(raw.Payload, msg)
		var attachments []MailAttachment
		// Never fail the whole message if attachment collection errors; email body still saves.
		if err := c.collectAttachments(ctx, id, raw.Payload, &attachments); err != nil {
			msg.Attachments = nil
		} else {
			msg.Attachments = attachments
		}
	}

	return msg, nil
}

// parseParts recursively walks message payload parts to extract text/plain and text/html.
func parseParts(part *gmailv1.MessagePart, msg *Message) {
	if part == nil {
		return
	}

	ct := strings.ToLower(part.MimeType)

	if part.Body != nil && part.Body.Data != "" {
		data, err := base64.URLEncoding.DecodeString(part.Body.Data)
		if err != nil {
			// Try standard encoding as fallback.
			data, err = base64.StdEncoding.DecodeString(part.Body.Data)
		}
		if err == nil {
			switch {
			case ct == "text/plain" && msg.BodyText == "":
				msg.BodyText = string(data)
			case ct == "text/html" && msg.BodyHTML == "":
				msg.BodyHTML = string(data)
			}
		}
	}

	for _, sub := range part.Parts {
		parseParts(sub, msg)
	}
}

func partHeader(part *gmailv1.MessagePart, name string) string {
	if part == nil {
		return ""
	}
	for _, h := range part.Headers {
		if h == nil {
			continue
		}
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

// gmailPartFilename returns a display filename from the part Filename field or MIME headers.
func gmailPartFilename(part *gmailv1.MessagePart) string {
	if part == nil {
		return ""
	}
	if fn := strings.TrimSpace(part.Filename); fn != "" {
		return mimeDecodeFilename(fn)
	}
	_, params, err := mime.ParseMediaType(partHeader(part, "Content-Disposition"))
	if err == nil {
		if fn := params["filename"]; fn != "" {
			return mimeDecodeFilename(fn)
		}
	}
	_, params, err = mime.ParseMediaType(partHeader(part, "Content-Type"))
	if err == nil {
		if fn := params["name"]; fn != "" {
			return mimeDecodeFilename(fn)
		}
	}
	return ""
}

func mimeDecodeFilename(s string) string {
	dec := new(mime.WordDecoder)
	out, err := dec.DecodeHeader(s)
	if err != nil {
		return strings.Trim(s, `"`)
	}
	return out
}

// ShouldSaveGmailPartAsAttachment mirrors IMAP shouldSaveAsAttachment for Gmail API MessageParts
// (inline images excluded; attachment disposition and non-text parts included).
func ShouldSaveGmailPartAsAttachment(part *gmailv1.MessagePart) bool {
	if part == nil {
		return false
	}
	mt := strings.TrimSpace(part.MimeType)
	mtLower := strings.ToLower(mt)
	if strings.HasPrefix(mtLower, "multipart/") {
		return false
	}
	if strings.EqualFold(mt, "message/rfc822") {
		return false
	}

	disp := partHeader(part, "Content-Disposition")
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
		fn = gmailPartFilename(part)
	}
	if fn != "" && !strings.HasPrefix(mtLower, "text/plain") && !strings.HasPrefix(mtLower, "text/html") {
		return true
	}

	if mt != "" && !strings.HasPrefix(mtLower, "text/") {
		return true
	}
	return false
}

func decodeGmailBodyData(s string) ([]byte, error) {
	if s == "" {
		return nil, nil
	}
	data, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		data, err = base64.StdEncoding.DecodeString(s)
	}
	return data, err
}

func (c *Client) fetchAttachmentAPI(ctx context.Context, messageID, attachmentID string) ([]byte, error) {
	att, err := c.svc.Users.Messages.Attachments.Get("me", messageID, attachmentID).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return decodeGmailBodyData(att.Data)
}

func (c *Client) partAttachmentBytes(ctx context.Context, messageID string, part *gmailv1.MessagePart) ([]byte, error) {
	if part == nil || part.Body == nil {
		return nil, nil
	}
	if part.Body.AttachmentId != "" {
		return c.fetchAttachmentAPI(ctx, messageID, part.Body.AttachmentId)
	}
	return decodeGmailBodyData(part.Body.Data)
}

// collectAttachments walks the payload tree and appends saved parts to *out.
func (c *Client) collectAttachments(ctx context.Context, messageID string, part *gmailv1.MessagePart, out *[]MailAttachment) error {
	if part == nil {
		return nil
	}
	mt := strings.ToLower(strings.TrimSpace(part.MimeType))

	if strings.HasPrefix(mt, "multipart/") {
		for _, sub := range part.Parts {
			if err := c.collectAttachments(ctx, messageID, sub, out); err != nil {
				return err
			}
		}
		return nil
	}

	if mt == "message/rfc822" {
		for _, sub := range part.Parts {
			if err := c.collectAttachments(ctx, messageID, sub, out); err != nil {
				return err
			}
		}
		return nil
	}

	if mt == "text/plain" || mt == "text/html" {
		return nil
	}

	if !ShouldSaveGmailPartAsAttachment(part) {
		return nil
	}

	data, err := c.partAttachmentBytes(ctx, messageID, part)
	if err != nil || len(data) == 0 {
		return nil
	}

	fn := gmailPartFilename(part)
	mtClean := strings.TrimSpace(part.MimeType)
	if mtClean == "" {
		mtClean = "application/octet-stream"
	}

	*out = append(*out, MailAttachment{
		Filename:  fn,
		MediaType: mtClean,
		Data:      data,
	})
	return nil
}

// parseEmailDate attempts to parse common email date formats.
func parseEmailDate(s string) (time.Time, error) {
	formats := []string{
		time.RFC1123Z,
		time.RFC1123,
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"Mon, 2 Jan 2006 15:04:05 MST",
		"2 Jan 2006 15:04:05 -0700",
		"Mon, _2 Jan 2006 15:04:05 -0700",
	}
	s = strings.TrimSpace(s)
	// Strip parenthetical timezone suffixes like "(UTC)"
	if idx := strings.LastIndex(s, "("); idx > 0 {
		s = strings.TrimSpace(s[:idx])
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse date: %s", s)
}
