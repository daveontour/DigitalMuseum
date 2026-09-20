package filesystem

import (
	"path/filepath"
	"strings"

	"github.com/daveontour/aimuseum/internal/importstorage"
)

// maxExpandedPathEquivalents caps how many variants a single path can expand
// to, guarding against pathological blow-up from many overlapping/chained
// user-defined rules — the rule list itself is expected to stay small (a
// handful of drive/folder moves).
const maxExpandedPathEquivalents = 64

// CleanPathEquivalenceRules returns a copy of rules with both sides
// filepath.Clean'd, so ExpandEquivalentPaths never has to re-clean a rule on
// every call. Call this once when rules are loaded, not per file.
func CleanPathEquivalenceRules(rules []importstorage.PathEquivalenceRule) []importstorage.PathEquivalenceRule {
	out := make([]importstorage.PathEquivalenceRule, len(rules))
	for i, r := range rules {
		out[i] = importstorage.PathEquivalenceRule{
			PathA: filepath.Clean(r.PathA),
			PathB: filepath.Clean(r.PathB),
		}
	}
	return out
}

// ExpandEquivalentPaths returns path plus every path reachable by rewriting
// it (in either direction, chained to a fixed point) via the given
// equivalence rules — e.g. rule{"C:\Photos","D:\Photos"} makes
// "C:\Photos\2020\a.jpg" equivalent to "D:\Photos\2020\a.jpg", and a rule can
// apply at any depth in the tree, not just whole drive roots. Matching is
// case-insensitive (Windows) and anchored to whole path segments — a rule
// for "C:\Foo" never matches "C:\FooBar". The result always includes the
// original path and contains no duplicates (case-insensitive).
//
// Callers should pass rules through CleanPathEquivalenceRules once (not
// per-file) before calling this in a loop.
func ExpandEquivalentPaths(path string, rules []importstorage.PathEquivalenceRule) []string {
	seen := make(map[string]string, 4) // lower-case key -> original-case value
	addPath := func(p string) bool {
		key := strings.ToLower(p)
		if _, ok := seen[key]; ok {
			return false
		}
		seen[key] = p
		return true
	}
	addPath(path)

	frontier := []string{path}
	for len(frontier) > 0 && len(seen) < maxExpandedPathEquivalents {
		var next []string
		for _, p := range frontier {
			for _, rule := range rules {
				if rewritten, ok := rewriteViaEquivalentPrefix(p, rule.PathA, rule.PathB); ok && addPath(rewritten) {
					next = append(next, rewritten)
				}
				if rewritten, ok := rewriteViaEquivalentPrefix(p, rule.PathB, rule.PathA); ok && addPath(rewritten) {
					next = append(next, rewritten)
				}
			}
		}
		frontier = next
	}

	out := make([]string, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	return out
}

// rewriteViaEquivalentPrefix rewrites path's "from" prefix to "to" when path
// equals from, or is a descendant of it (case-insensitive, whole-segment
// match — "C:\Foo" must not match "C:\FooBar"). from and to are assumed
// already filepath.Clean'd (see CleanPathEquivalenceRules).
func rewriteViaEquivalentPrefix(path, from, to string) (string, bool) {
	if len(path) < len(from) || !strings.EqualFold(path[:len(from)], from) {
		return "", false
	}
	rest := path[len(from):]
	if rest != "" && rest[0] != filepath.Separator {
		return "", false
	}
	return to + rest, true
}
