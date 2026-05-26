package georegion

import "strings"

const (
	// KeyDefaultRegion stores the fallback region code when no bbox matches.
	KeyDefaultRegion = "__default_region__"
	// KeyDefaultLabel stores the display label for the default / unknown region.
	KeyDefaultLabel = "__default_label__"
)

// IsReservedKey reports whether key is a reserved settings row (not a bbox region).
func IsReservedKey(key string) bool {
	k := strings.TrimSpace(key)
	return k == KeyDefaultRegion || k == KeyDefaultLabel
}
