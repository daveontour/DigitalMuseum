package facebook

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// FacebookExport represents the top-level structure of a Facebook message JSON file
type FacebookExport struct {
	Title        string                `json:"title"`
	Participants []FacebookParticipant `json:"participants"`
	Messages     []FacebookMessage     `json:"messages"`
}

// FacebookParticipant represents a participant in a conversation
type FacebookParticipant struct {
	Name string `json:"name"`
}

// FacebookMessage represents a single message in the export
type FacebookMessage struct {
	TimestampMs *int64               `json:"timestamp_ms"`
	SenderName  string               `json:"sender_name"`
	Content     string               `json:"content"`
	Photos      []FacebookAttachment `json:"photos"`
	Videos      []FacebookAttachment `json:"videos"`
	Files       []FacebookAttachment `json:"files"`
	Share       *FacebookShare       `json:"share"`
	Sticker     interface{}          `json:"sticker"`
}

// FacebookAttachment represents a photo, video, or file attachment
type FacebookAttachment struct {
	URI string `json:"uri"`
}

// FacebookShare represents shared link content
type FacebookShare struct {
	Link      string `json:"link"`
	ShareText string `json:"share_text"`
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
func DetermineMessageType(senderName, userName string, participants []FacebookParticipant) string {
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

// ExtractSubject builds subject from share link and text
func ExtractSubject(share *FacebookShare) *string {
	if share == nil {
		return nil
	}
	parts := []string{}
	if share.ShareText != "" {
		parts = append(parts, share.ShareText)
	}
	if share.Link != "" {
		parts = append(parts, share.Link)
	}
	if len(parts) == 0 {
		return nil
	}
	s := strings.TrimSpace(strings.Join(parts, " "))
	return &s
}

func parseFacebookJSON(data []byte, export *FacebookExport) error {
	return json.Unmarshal(data, export)
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
