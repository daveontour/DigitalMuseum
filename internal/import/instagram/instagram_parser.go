package instagram

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var cleanStringRegex = regexp.MustCompile(`[^\w\s]`)

// InstagramExport represents the top-level structure of an Instagram message JSON file
type InstagramExport struct {
	Title        string                 `json:"title"`
	Participants []InstagramParticipant `json:"participants"`
	Messages     []InstagramMessage     `json:"messages"`
}

// InstagramParticipant represents a participant in a conversation
type InstagramParticipant struct {
	Name string `json:"name"`
}

// InstagramMessage represents a single message in the export
type InstagramMessage struct {
	TimestampMs *int64                `json:"timestamp_ms"`
	SenderName  string                `json:"sender_name"`
	Content     string                `json:"content"`
	Photos      []InstagramAttachment `json:"photos"`
}

// InstagramAttachment represents a photo attachment
type InstagramAttachment struct {
	URI string `json:"uri"`
}

// ParseTimestampMs converts Unix timestamp in milliseconds to time.Time
func ParseTimestampMs(ts *int64) (*time.Time, error) {
	if ts == nil {
		return nil, nil
	}
	t := time.UnixMilli(*ts)
	return &t, nil
}

// DetermineMessageType returns "Incoming" or "Outgoing" based on sender
func DetermineMessageType(senderName, userName string, participants []InstagramParticipant) string {
	if userName != "" {
		if senderName == userName {
			return "Outgoing"
		}
		return "Incoming"
	}
	if len(participants) > 0 {
		first := participants[0].Name
		if senderName == first {
			return "Outgoing"
		}
		return "Incoming"
	}
	return "Incoming"
}

// CleanString strips non-word characters (except spaces) from a string
func CleanString(s string) string {
	return strings.TrimSpace(cleanStringRegex.ReplaceAllString(s, ""))
}

// FilenameFromURI extracts filename from a URI/path
func FilenameFromURI(uri string) string {
	return filepath.Base(uri)
}

func extractMessageNumber(path string) int {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	parts := strings.Split(base, "_")
	if len(parts) >= 2 {
		var n int
		fmt.Sscanf(parts[1], "%d", &n)
		return n
	}
	return 0
}
