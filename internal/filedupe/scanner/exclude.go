package scanner

import (
	"fmt"
	"regexp"
	"strings"
)

type excludePatterns struct {
	patterns []*regexp.Regexp
}

func compileExcludePatterns(raw []string) (excludePatterns, error) {
	cleaned := make([]string, 0, len(raw))
	for _, pattern := range raw {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		cleaned = append(cleaned, pattern)
	}

	compiled := excludePatterns{
		patterns: make([]*regexp.Regexp, 0, len(cleaned)),
	}

	for _, pattern := range cleaned {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return excludePatterns{}, fmt.Errorf("invalid exclude pattern %q: %w", pattern, err)
		}
		compiled.patterns = append(compiled.patterns, re)
	}

	return compiled, nil
}

func (e excludePatterns) matchesFolder(name string) bool {
	for _, pattern := range e.patterns {
		if pattern.MatchString(name) {
			return true
		}
	}
	return false
}

func mergeExcludePatterns(a, b excludePatterns) excludePatterns {
	return excludePatterns{
		patterns: append(append([]*regexp.Regexp{}, a.patterns...), b.patterns...),
	}
}
