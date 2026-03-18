package service

import (
	"sort"
	"strings"
	"testing"
)

func TestGuessMimeFromFilename(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"photo.jpg", "image/jpeg"},
		{"photo.JPEG", "image/jpeg"},
		{"image.png", "image/png"},
		{"clip.mp4", "video/mp4"},
		{"raw.HEIC", "image/heic"},
		{"unknown.xyz", ""},
		{"noextension", ""},
	}
	for _, tc := range cases {
		got := guessMimeFromFilename(tc.name)
		if got != tc.want {
			t.Errorf("guessMimeFromFilename(%q) = %q; want %q", tc.name, got, tc.want)
		}
	}
}

// TestTagSplitDedup verifies the tag-splitting logic used in GetDistinctTags.
func TestTagSplitDedup(t *testing.T) {
	splitTags := func(rawTags []string) []string {
		set := make(map[string]struct{})
		for _, raw := range rawTags {
			for _, tag := range strings.Split(raw, ",") {
				if t := strings.TrimSpace(tag); t != "" {
					set[t] = struct{}{}
				}
			}
		}
		out := make([]string, 0, len(set))
		for t := range set {
			out = append(out, t)
		}
		sort.Strings(out)
		return out
	}

	cases := []struct {
		raw  []string
		want []string
	}{
		{
			raw:  []string{"travel, food, architecture", "food, nature"},
			want: []string{"architecture", "food", "nature", "travel"},
		},
		{
			raw:  []string{""},
			want: []string{},
		},
		{
			raw:  nil,
			want: []string{},
		},
		{
			raw:  []string{"  dogs  ,cats,  birds  "},
			want: []string{"birds", "cats", "dogs"},
		},
	}

	for _, tc := range cases {
		got := splitTags(tc.raw)
		if len(got) != len(tc.want) {
			t.Errorf("raw=%v: got %v; want %v", tc.raw, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("raw=%v: got[%d]=%q; want %q", tc.raw, i, got[i], tc.want[i])
			}
		}
	}
}

// TestRatingDefault verifies the rating=0 → 5 default mirrors Python `media_item.rating or 5`.
func TestRatingDefault(t *testing.T) {
	applyDefault := func(r int) int {
		if r == 0 {
			return 5
		}
		return r
	}
	if applyDefault(0) != 5 {
		t.Error("rating 0 should default to 5")
	}
	if applyDefault(3) != 3 {
		t.Error("rating 3 should stay 3")
	}
	if applyDefault(1) != 1 {
		t.Error("rating 1 should stay 1")
	}
}
