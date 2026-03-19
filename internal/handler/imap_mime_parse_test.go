package handler

import (
	"strings"
	"testing"
)

func TestParseIMAPMIMEBody_multipartPlainAndAttachment(t *testing.T) {
	raw := "" +
		"Content-Type: multipart/mixed; boundary=\"b\"\r\n" +
		"\r\n" +
		"--b\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"Hello\r\n" +
		"--b\r\n" +
		"Content-Type: application/pdf\r\n" +
		"Content-Disposition: attachment; filename=\"a.pdf\"\r\n" +
		"\r\n" +
		"%PDF-1 fake\r\n" +
		"--b--\r\n"

	p := parseIMAPMIMEBody([]byte(raw))
	if strings.TrimSpace(p.BodyPlain) != "Hello" {
		t.Fatalf("plain: %q", p.BodyPlain)
	}
	if len(p.Attachments) != 1 {
		t.Fatalf("attachments: %d", len(p.Attachments))
	}
	if p.Attachments[0].Filename != "a.pdf" {
		t.Fatalf("filename: %q", p.Attachments[0].Filename)
	}
	if !p.HasAttach {
		t.Fatal("expected HasAttach")
	}
}

func TestImapEmailStoredFields_prefersHTMLForRaw(t *testing.T) {
	p := imapParsedMIME{BodyPlain: "p", BodyHTML: "<p>h</p>"}
	raw, plain, _ := imapEmailStoredFields(p)
	if plain == nil || *plain != "p" {
		t.Fatal("plain")
	}
	if raw == nil || *raw != "<p>h</p>" {
		t.Fatalf("raw: %v", raw)
	}
}
