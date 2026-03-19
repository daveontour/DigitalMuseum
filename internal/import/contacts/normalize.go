package contacts

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	parenRe   = regexp.MustCompile(`\([^)]*\)`)
	nonCharRe = regexp.MustCompile(`[^a-z\s]+`)
	spaceRe   = regexp.MustCompile(`\s+`)
)

func normalizeName(name string) string {
	name = strings.ToLower(name)
	name = parenRe.ReplaceAllString(name, "")
	name = nonCharRe.ReplaceAllString(name, " ")
	name = spaceRe.ReplaceAllString(name, " ")
	return strings.TrimSpace(name)
}

// NormalizeEmailForMatching normalizes an email for comparison purposes
func NormalizeEmailForMatching(email string) string {
	email = strings.ToLower(email)
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return email
	}
	local, domain := parts[0], parts[1]
	local = strings.ReplaceAll(local, "_", "")
	local = strings.ReplaceAll(local, ".", "")
	local = strings.ReplaceAll(local, "-", "")
	local = strings.ReplaceAll(local, "'", "")
	local = strings.ReplaceAll(local, "`", "")
	local = strings.ReplaceAll(local, "´", "")
	return local + "@" + domain
}

func tokenize(name string) map[string]struct{} {
	tokens := map[string]struct{}{}
	for _, t := range strings.Split(name, " ") {
		if t != "" {
			tokens[t] = struct{}{}
		}
	}
	return tokens
}

func isEmailAddress(s string) bool {
	return strings.Contains(s, "@") && strings.Contains(s, ".")
}

func capitalizeName(name string) string {
	if isEmailAddress(name) {
		return name
	}
	name = strings.ReplaceAll(name, "`", "'")
	name = strings.ReplaceAll(name, "´", "'")
	parts := strings.Fields(name)
	if len(parts) < 2 || len(parts) > 3 {
		return name
	}
	var result []string
	for _, part := range parts {
		if len(part) > 0 {
			var capitalized strings.Builder
			prevWasApostrophe := false
			for i, r := range part {
				if i == 0 {
					capitalized.WriteRune(unicode.ToUpper(r))
				} else if prevWasApostrophe {
					capitalized.WriteRune(unicode.ToUpper(r))
					prevWasApostrophe = false
				} else if r == '\'' {
					capitalized.WriteRune(r)
					prevWasApostrophe = true
				} else {
					capitalized.WriteRune(unicode.ToLower(r))
					prevWasApostrophe = false
				}
			}
			result = append(result, capitalized.String())
		} else {
			result = append(result, part)
		}
	}
	return strings.Join(result, " ")
}
