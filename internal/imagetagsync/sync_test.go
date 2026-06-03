package imagetagsync

import (
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/daveontour/aimuseum/internal/model"
)

func TestSplitTags(t *testing.T) {
	raw := "Playhouse Company, vacation , ,family"
	got := splitTags(&raw)
	if len(got) != 3 || got[0] != "Playhouse Company" || got[1] != "vacation" || got[2] != "family" {
		t.Fatalf("splitTags: %#v", got)
	}
	if splitTags(nil) != nil {
		t.Fatal("nil tags should be nil")
	}
	empty := ""
	if splitTags(&empty) != nil {
		t.Fatal("empty tags should be nil")
	}
}

func TestTagsForRecord(t *testing.T) {
	src := "whatsapp"
	got := tagsForRecord(model.ImageMetadataJSONRecord{
		Source: &src,
		Tags:   strPtr("Playhouse Company, vacation"),
	})
	if len(got) != 3 || got[2] != "whatsapp" {
		t.Fatalf("tagsForRecord append source: %#v", got)
	}

	dup := tagsForRecord(model.ImageMetadataJSONRecord{
		Source: &src,
		Tags:   strPtr("whatsapp, alpha"),
	})
	if len(dup) != 2 {
		t.Fatalf("tagsForRecord dedupe source: %#v", dup)
	}

	onlySource := tagsForRecord(model.ImageMetadataJSONRecord{Source: &src})
	if len(onlySource) != 1 || onlySource[0] != "whatsapp" {
		t.Fatalf("tagsForRecord source only: %#v", onlySource)
	}
}

func strPtr(s string) *string { return &s }

func TestLoadIndex(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "export.json")
	const body = `[
		{"id": 1, "source_reference": "ref-a", "source": "whatsapp", "tags": "alpha"},
		{"id": 2, "source_reference": "ref-a", "source": "email", "tags": "beta"}
	]`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	idx, dupes, err := loadIndex(path, log.New(os.Stderr, "", 0))
	if err != nil {
		t.Fatalf("loadIndex: %v", err)
	}
	if dupes != 1 {
		t.Fatalf("dupes = %d; want 1", dupes)
	}
	if _, ok := idx["1"]; !ok {
		t.Fatal("missing key 1")
	}
	if _, ok := idx["2"]; !ok {
		t.Fatal("missing key 2")
	}
	if rec, ok := idx["ref-a"]; !ok || rec.ID != 1 {
		t.Fatalf("ref-a should map to first record, got %+v", rec)
	}
}
