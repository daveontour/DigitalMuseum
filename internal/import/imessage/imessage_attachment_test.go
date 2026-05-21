package imessage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsGenericImazingAttachmentName(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"Attachment.jpg", true},
		{"attachment.JPEG", true},
		{"subdir/Attachment.png", true},
		{"  Attachment.heic  ", true},
		{"IMG_0001.jpg", false},
		{"attachment", false},
		{"NotAttachment.jpg", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsGenericImazingAttachmentName(tt.in); got != tt.want {
			t.Errorf("IsGenericImazingAttachmentName(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestListImageBasenames(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.jpg"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "b.PNG"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "c.txt"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "nested"), []byte("x"), 0o644)

	got, err := ListImageBasenames(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.jpg", "b.PNG"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestPrepareIMessageData_GenericPlaceholderAndUsedBasenames(t *testing.T) {
	dir := t.TempDir()
	realName := "realphoto.jpg"
	if err := os.WriteFile(filepath.Join(dir, realName), []byte("fakejpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	// On-disk Attachment.jpg must NOT be consumed by generic CSV row (always placeholder).
	if err := os.WriteFile(filepath.Join(dir, "Attachment.jpg"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}

	stats := &ImportStats{}
	used := make(map[string]struct{})

	msg := IMessageMessage{
		ChatSession: "Test Chat",
		Service:     "SMS",
		Type:        "Incoming",
		SenderID:    "Bob",
		SenderName:  "Bob",
		Status:      "Received",
		Attachment:  "Attachment.jpg",
	}

	md, data, fn, mt := prepareIMessageData(msg, dir, "conv", stats, nil, used)
	if md.ChatSession == nil || *md.ChatSession == "" {
		t.Fatal("expected chat session")
	}
	if len(data) == 0 {
		t.Fatal("expected placeholder image bytes")
	}
	if fn != "imazing-placeholder.png" || mt != "image/png" {
		t.Fatalf("got filename %q type %q", fn, mt)
	}
	if !equalBytes(data, ImazingPlaceholderPNG()) {
		t.Fatal("placeholder bytes mismatch")
	}
	if stats.PlaceholdersUsed != 1 || stats.AttachmentsFound != 1 {
		t.Fatalf("stats PlaceholdersUsed=%d AttachmentsFound=%d", stats.PlaceholdersUsed, stats.AttachmentsFound)
	}
	if _, ok := used["Attachment.jpg"]; ok {
		t.Fatal("generic row must not mark Attachment.jpg as used")
	}

	msg2 := IMessageMessage{
		ChatSession: "Test Chat",
		Service:     "SMS",
		Type:        "Incoming",
		SenderID:    "Bob",
		SenderName:  "Bob",
		Status:      "Received",
		Attachment:  realName,
	}
	_, _, _, _ = prepareIMessageData(msg2, dir, "conv", stats, nil, used)
	if _, ok := used[realName]; !ok {
		t.Fatal("expected realphoto.jpg in usedBasenames")
	}
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
