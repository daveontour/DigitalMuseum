package importstorage

import "testing"

func TestTagsFromSourceRef(t *testing.T) {
	tests := []struct {
		ref  string
		want string
	}{
		{"Vacation/beach/IMG_001.heic", "Vacation, beach, IMG_001.heic"},
		{`Album\2024\a.jpg`, "Album, 2024, a.jpg"},
		{"photo.jpg", "photo.jpg"},
		{"", ""},
		{"  ", ""},
		{"a//b/../c", "a, b, c"},
	}
	for _, tt := range tests {
		got := TagsFromSourceRef(tt.ref)
		if got != tt.want {
			t.Errorf("TagsFromSourceRef(%q) = %q; want %q", tt.ref, got, tt.want)
		}
	}
}
