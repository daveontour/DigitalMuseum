package repository

import "testing"

func TestFilterTagsNotAlreadyPresent(t *testing.T) {
	existing := "beach, Sunset"
	tests := []struct {
		name     string
		existing *string
		newTags  string
		want     []string
	}{
		{
			name:     "skips case-insensitive duplicates",
			existing: &existing,
			newTags:  "dog, beach, SUNSET, cat",
			want:     []string{"dog", "cat"},
		},
		{
			name:     "all duplicates",
			existing: &existing,
			newTags:  "Beach, sunset",
			want:     nil,
		},
		{
			name:    "empty existing",
			newTags: "alpha, beta",
			want:    []string{"alpha", "beta"},
		},
		{
			name:     "dedupes within new batch",
			existing: nil,
			newTags:  "dog, Dog, cat",
			want:     []string{"dog", "cat"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterTagsNotAlreadyPresent(tt.existing, tt.newTags)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}
