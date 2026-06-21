package service

import (
	"strings"
	"testing"

	"github.com/daveontour/aimuseum/internal/model"
)

func TestFormatArchiveInventoryPromptBlock(t *testing.T) {
	inv := &model.ArchiveDataInventory{
		MessagesByService: map[string]int64{"WhatsApp": 42},
		EmailsTotal:       10,
		ImagesTotal:       5,
	}
	block, err := formatArchiveInventoryPromptBlock(inv)
	if err != nil {
		t.Fatalf("formatArchiveInventoryPromptBlock: %v", err)
	}
	if !strings.Contains(block, "**Archive data inventory (JSON):**") {
		t.Fatalf("missing header: %q", block)
	}
	if !strings.Contains(block, `"WhatsApp": 42`) {
		t.Fatalf("missing message count: %q", block)
	}
	if !strings.Contains(block, "Data Import") {
		t.Fatalf("missing instruction: %q", block)
	}
}

func TestFormatArchiveInventoryPromptBlock_nil(t *testing.T) {
	block, err := formatArchiveInventoryPromptBlock(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if block != "" {
		t.Fatalf("expected empty block, got %q", block)
	}
}
