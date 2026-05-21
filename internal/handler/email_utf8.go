package handler

import (
	"bytes"
	"mime"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/htmlindex"
)

// decodeMIMEPartToUTF8 interprets body bytes using the charset from Content-Type
// (e.g. text/plain; charset=windows-1252) and returns a valid UTF-8 string.
// When charset is missing or wrong, invalid UTF-8 is repaired (common case:
// Windows-1252 / Latin-1 bytes mislabeled as UTF-8).
func decodeMIMEPartToUTF8(body []byte, contentTypeHeader string) string {
	_, params, err := mime.ParseMediaType(contentTypeHeader)
	charset := ""
	if err == nil {
		charset = strings.TrimSpace(params["charset"])
	}
	return bytesToUTF8(body, charset)
}

// bytesToUTF8 converts arbitrary bytes to a UTF-8 string for PostgreSQL text columns.
func bytesToUTF8(body []byte, charset string) string {
	body = bytes.TrimPrefix(body, []byte{0xEF, 0xBB, 0xBF})

	charset = strings.Trim(strings.TrimSpace(charset), `"'`)
	if charset != "" {
		lc := strings.ToLower(charset)
		if lc == "utf-8" || lc == "utf8" {
			if utf8.Valid(body) {
				return string(body)
			}
			return legacyBytesToUTF8(body)
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
	return legacyBytesToUTF8(body)
}

// legacyBytesToUTF8 maps bytes that are not valid UTF-8 through Windows-1252,
// which matches many Western email bodies and headers (0xA0 NBSP, 0x96 en dash, etc.).
// UTF-8 encoding of U+FFFD replacement character
var replUTF8 = []byte("\uFFFD")

func legacyBytesToUTF8(body []byte) string {
	out, err := charmap.Windows1252.NewDecoder().Bytes(body)
	if err != nil {
		return string(bytes.ToValidUTF8(body, replUTF8))
	}
	return string(bytes.ToValidUTF8(out, replUTF8))
}

// ensureUTF8String repairs strings that are not valid UTF-8 (e.g. envelope fields).
func ensureUTF8String(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return bytesToUTF8([]byte(s), "")
}

func ptrEnsureUTF8(p *string) *string {
	if p == nil {
		return nil
	}
	v := ensureUTF8String(*p)
	return &v
}

// truncateUTF8Runes shortens s to at most maxRunes runes without splitting UTF-8 sequences.
func truncateUTF8Runes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes])
}
