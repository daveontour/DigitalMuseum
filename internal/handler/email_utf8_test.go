package handler

import (
	"testing"
	"unicode/utf8"
)

func TestDecodeMIMEPartToUTF8_declaredWindows1252(t *testing.T) {
	// NBSP (U+00A0) as single byte 0xA0 in windows-1252
	body := []byte{0x48, 0x65, 0x6C, 0x6C, 0x6F, 0xA0, 0x77, 0x6F, 0x72, 0x6C, 0x64}
	ct := "text/plain; charset=windows-1252"
	s := decodeMIMEPartToUTF8(body, ct)
	if !utf8.ValidString(s) {
		t.Fatalf("not valid UTF-8: %q", s)
	}
	if want := "Hello\u00a0world"; s != want {
		t.Fatalf("got %q want %q", s, want)
	}
}

func TestBytesToUTF8_missingCharset_invalidUTF8UsesLegacy(t *testing.T) {
	body := []byte{0x48, 0x65, 0x6C, 0x6C, 0x6F, 0xA0} // "Hello" + CP1252 NBSP
	s := bytesToUTF8(body, "")
	if !utf8.ValidString(s) {
		t.Fatalf("not valid UTF-8: %q", s)
	}
}

func TestEnsureUTF8String_invalidInput(t *testing.T) {
	// Lone 0xA0 is invalid UTF-8
	s := ensureUTF8String(string([]byte{0xA0}))
	if !utf8.ValidString(s) {
		t.Fatalf("still invalid: %q", s)
	}
}

func TestTruncateUTF8Runes(t *testing.T) {
	s := "a" + string([]rune{0x4E2D}) + "b" // multi-byte rune in middle
	out := truncateUTF8Runes(s, 2)
	if out != "a"+string([]rune{0x4E2D}) {
		t.Fatalf("got %q", out)
	}
	if !utf8.ValidString(out) {
		t.Fatal("truncation broke UTF-8")
	}
}
