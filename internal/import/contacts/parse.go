package contacts

import (
	"regexp"
	"strings"
)

// ParseEmailEntry parses an email entry and extracts email and name.
// Supports formats: "Name <email@example.com>", "email@example.com (Name)", "Name email@example.com", "email@example.com"
func ParseEmailEntry(entry string) (email, name string) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return "", ""
	}
	if idx := strings.LastIndex(entry, "<"); idx != -1 {
		if idx2 := strings.Index(entry[idx:], ">"); idx2 != -1 {
			email = strings.TrimSpace(entry[idx+1 : idx+idx2])
			name = strings.TrimSpace(entry[:idx])
			email = strings.ReplaceAll(email, `"`, "")
			name = strings.ReplaceAll(name, `"`, "")
			email = strings.ReplaceAll(email, "<", "")
			email = strings.ReplaceAll(email, ">", "")
			name = strings.ReplaceAll(name, "<", "")
			name = strings.ReplaceAll(name, ">", "")
			name = strings.ReplaceAll(name, "`", "'")
			name = strings.ReplaceAll(name, "´", "'")
			name = strings.Trim(name, "'")
			name = strings.TrimSpace(name)
			email = strings.ToLower(email)
			return email, name
		}
	}
	if idx := strings.Index(entry, "("); idx != -1 {
		if idx2 := strings.Index(entry[idx:], ")"); idx2 != -1 {
			email = strings.TrimSpace(entry[:idx])
			name = strings.TrimSpace(entry[idx+1 : idx+idx2])
			email = strings.ReplaceAll(email, `"`, "")
			name = strings.ReplaceAll(name, `"`, "")
			email = strings.ReplaceAll(email, "<", "")
			email = strings.ReplaceAll(email, ">", "")
			name = strings.ReplaceAll(name, "<", "")
			name = strings.ReplaceAll(name, ">", "")
			name = strings.ReplaceAll(name, "`", "'")
			name = strings.ReplaceAll(name, "´", "'")
			name = strings.Trim(name, "'")
			name = strings.TrimSpace(name)
			email = strings.ToLower(email)
			return email, name
		}
	}
	emailRegex := regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	emailMatch := emailRegex.FindString(entry)
	if emailMatch != "" {
		email = emailMatch
		name = strings.TrimSpace(strings.Replace(entry, emailMatch, "", -1))
		name = strings.Trim(name, " ()")
		email = strings.ReplaceAll(email, `"`, "")
		name = strings.ReplaceAll(name, `"`, "")
		email = strings.ReplaceAll(email, "<", "")
		email = strings.ReplaceAll(email, ">", "")
		name = strings.ReplaceAll(name, "<", "")
		name = strings.ReplaceAll(name, ">", "")
		name = strings.ReplaceAll(name, "`", "'")
		name = strings.ReplaceAll(name, "´", "'")
		name = strings.Trim(name, "'")
		name = strings.TrimSpace(name)
		email = strings.ToLower(email)
		return email, name
	}
	if strings.Contains(entry, "@") {
		email = strings.ReplaceAll(entry, `"`, "")
		email = strings.ReplaceAll(email, "<", "")
		email = strings.ReplaceAll(email, ">", "")
		email = strings.ToLower(email)
		return email, ""
	}
	return "", ""
}
