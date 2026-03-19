package handler

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"
)

// imapMIMEHeader is implemented by mail.Header and textproto.MIMEHeader.
type imapMIMEHeader interface {
	Get(key string) string
}

// imapAttachmentPart is a non-body MIME part to store as email_attachment.
type imapAttachmentPart struct {
	Filename  string
	MediaType string
	Data      []byte
}

// imapParsedMIME holds body text and attachments after walking a MIME tree (Gmail-style).
type imapParsedMIME struct {
	BodyPlain    string
	BodyHTML     string
	Attachments  []imapAttachmentPart
	HasAttach    bool
}

// parseIMAPMIMEBody parses RFC822.TEXT bytes: extracts first text/plain and first text/html,
// and collects attachment parts (similar to Gmail's parseParts, plus saving binaries).
func parseIMAPMIMEBody(raw []byte) imapParsedMIME {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return imapParsedMIME{}
	}

	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		// Some servers return a MIME entity without a full RFC822 wrapper; try header/body split.
		if st, ok := parseIMAPMIMEHeaderBody(raw); ok {
			out := imapParsedMIME{
				BodyPlain:   st.plain,
				BodyHTML:    st.html,
				Attachments: st.attachments,
			}
			out.HasAttach = len(out.Attachments) > 0
			return out
		}
		// Last resort: single unlabeled body — store as plain only (not as raw MIME).
		return imapParsedMIME{BodyPlain: strings.TrimSpace(string(raw))}
	}

	body, err := io.ReadAll(msg.Body)
	if err != nil {
		return imapParsedMIME{}
	}

	st := &parseState{}
	walkMIME(msg.Header, body, st)

	out := imapParsedMIME{
		BodyPlain:   st.plain,
		BodyHTML:    st.html,
		Attachments: st.attachments,
	}
	out.HasAttach = len(out.Attachments) > 0
	return out
}

type parseState struct {
	plain        string
	html         string
	attachments  []imapAttachmentPart
}

// parseIMAPMIMEHeaderBody parses leading MIME headers + body (when mail.ReadMessage fails).
func parseIMAPMIMEHeaderBody(raw []byte) (*parseState, bool) {
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

func walkMIME(h imapMIMEHeader, body []byte, st *parseState) {
	body = decodeContentTransfer(h.Get("Content-Transfer-Encoding"), body)

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
			st.plain = string(body)
		}

	case strings.HasPrefix(strings.ToLower(mt), "text/html"):
		if st.html == "" {
			st.html = string(body)
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
		if shouldSaveAsAttachment(h, mt) {
			fn := attachmentFilename(h)
			st.attachments = append(st.attachments, imapAttachmentPart{
				Filename:  fn,
				MediaType: mt,
				Data:      append([]byte(nil), body...),
			})
		}
	}
}

func shouldSaveAsAttachment(h imapMIMEHeader, mt string) bool {
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
		// Keep inline resources (e.g. cid: images) out of the attachment list, like typical Gmail body handling.
		return false
	}

	fn := dispParams["filename"]
	if fn == "" {
		fn = attachmentFilename(h)
	}
	if fn != "" && !strings.HasPrefix(mtLower, "text/plain") && !strings.HasPrefix(mtLower, "text/html") {
		return true
	}

	// Binary / non-text parts without explicit inline disposition (e.g. application/pdf).
	if mt != "" && !strings.HasPrefix(mtLower, "text/") {
		return true
	}
	return false
}

func attachmentFilename(h imapMIMEHeader) string {
	_, params, err := mime.ParseMediaType(h.Get("Content-Disposition"))
	if err == nil {
		if fn := params["filename"]; fn != "" {
			return mimeDecodeHeader(fn)
		}
	}
	_, params, err = mime.ParseMediaType(h.Get("Content-Type"))
	if err == nil {
		if fn := params["name"]; fn != "" {
			return mimeDecodeHeader(fn)
		}
	}
	return ""
}

func mimeDecodeHeader(s string) string {
	dec := new(mime.WordDecoder)
	out, err := dec.DecodeHeader(s)
	if err != nil {
		return strings.Trim(s, `"`)
	}
	return out
}

func decodeContentTransfer(enc string, data []byte) []byte {
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

// imapEmailStoredFields maps parsed MIME parts to DB columns for IMAP import.
// raw_message is exactly one body: the first text/html part if any, otherwise the first text/plain
// part — never MIME boundaries, multipart wrappers, or both HTML and plain concatenated.
// plain_text is the first text/plain part when present (may coexist with HTML in raw_message).
func imapEmailStoredFields(parsed imapParsedMIME) (rawMsg, plainText, snippet *string) {
	if parsed.BodyPlain != "" {
		p := parsed.BodyPlain
		plainText = &p
	}
	var raw string
	switch {
	case parsed.BodyHTML != "":
		raw = parsed.BodyHTML
	case parsed.BodyPlain != "":
		raw = parsed.BodyPlain
	}
	if raw != "" {
		rawMsg = &raw
	}

	var snip string
	if parsed.BodyPlain != "" {
		snip = parsed.BodyPlain
	} else if parsed.BodyHTML != "" {
		snip = stripHTMLForSnippet(parsed.BodyHTML)
	}
	snip = strings.TrimSpace(snip)
	if snip != "" {
		if len(snip) > 200 {
			snip = snip[:200]
		}
		snippet = &snip
	}
	return rawMsg, plainText, snippet
}

func stripHTMLForSnippet(html string) string {
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
