package service

import (
	"reflect"
	"sort"
	"testing"
)

func TestNormalizeTagsForEmbedding(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"   ", ""},
		{"cat, dog", "cat, dog"},
		{"Dog, cat", "cat, dog"},
		{"a, A, a", "a"},
		{"  beach , Beach , sun ", "beach, sun"},
	}
	for _, tt := range tests {
		got := NormalizeTagsForEmbedding(tt.in)
		if got != tt.want {
			t.Errorf("NormalizeTagsForEmbedding(%q) = %q; want %q", tt.in, got, tt.want)
		}
	}
}

func TestKeywordsForTagSearch(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"  ", nil},
		{"a", nil},
		{"beach sunset", []string{"beach sunset", "beach", "sunset"}},
		{"Sunset, beach", []string{"beach", "sunset"}},
		{"cat, dog park", []string{"cat", "dog park", "dog", "park"}},
		{"the beach", []string{"beach", "the beach"}},
		{"sunset at the beach", []string{"beach", "sunset", "sunset at the beach"}},
		{"and, or", []string{"and", "or"}},
	}
	for _, tt := range tests {
		got := KeywordsForTagSearch(tt.in)
		gc, wc := append([]string(nil), got...), append([]string(nil), tt.want...)
		sort.Strings(gc)
		sort.Strings(wc)
		if !reflect.DeepEqual(gc, wc) {
			t.Errorf("KeywordsForTagSearch(%q) = %#v; want (same set) %#v", tt.in, got, tt.want)
		}
	}
}

func TestTagEmbeddingSignature(t *testing.T) {
	a := TagEmbeddingSignature("cat, dog")
	b := TagEmbeddingSignature("cat, dog")
	if len(a) != 64 {
		t.Fatalf("TagEmbeddingSignature length = %d; want 64 hex chars", len(a))
	}
	if a != b {
		t.Fatalf("TagEmbeddingSignature not stable: %q vs %q", a, b)
	}
	if TagEmbeddingSignature("cat") == a {
		t.Fatal("TagEmbeddingSignature should differ for different normalized strings")
	}
}
