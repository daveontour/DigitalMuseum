package importstorage

import "strings"

// TagsFromSourceRef splits source_reference path segments into a comma-separated string for media_items.tags.
// Slashes (including backslashes, normalized) separate parts; empty and "." / ".." segments are skipped.
func TagsFromSourceRef(sourceRef string) string {
	s := strings.TrimSpace(strings.ReplaceAll(sourceRef, `\`, `/`))
	s = strings.Trim(s, "/")
	if s == "" {
		return ""
	}
	var parts []string
	for _, seg := range strings.Split(s, "/") {
		seg = strings.TrimSpace(seg)
		if seg == "" || seg == "." || seg == ".." {
			continue
		}
		parts = append(parts, seg)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
}
