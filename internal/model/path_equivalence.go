package model

// PathEquivalence is a row from filesystem_path_equivalences — a user-defined
// rule that two directory paths (at any depth, not just drive roots) should
// be treated as the same content for filesystem-import duplicate detection,
// e.g. after moving a photo tree from one drive letter to another.
type PathEquivalence struct {
	ID    int64  `json:"id"`
	PathA string `json:"path_a"`
	PathB string `json:"path_b"`
}
