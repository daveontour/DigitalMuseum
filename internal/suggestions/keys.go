package suggestions

import (
	"fmt"
	"strings"
)

const KeyDelimiter = "::"

// BuildKey forms the unique suggestion key from category and title.
func BuildKey(category, title string) (string, error) {
	category = strings.TrimSpace(category)
	title = strings.TrimSpace(title)
	if category == "" {
		return "", fmt.Errorf("category is required")
	}
	if title == "" {
		return "", fmt.Errorf("title is required")
	}
	return category + KeyDelimiter + title, nil
}

// SplitKey parses category and title from a stored key.
func SplitKey(key string) (category, title string, err error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", fmt.Errorf("key is required")
	}
	parts := strings.SplitN(key, KeyDelimiter, 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("invalid suggestion key %q", key)
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}
