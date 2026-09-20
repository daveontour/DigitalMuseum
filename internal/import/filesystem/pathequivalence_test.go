package filesystem

import (
	"sort"
	"strings"
	"testing"

	"github.com/daveontour/aimuseum/internal/importstorage"
)

func sortedLower(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = strings.ToLower(p)
	}
	sort.Strings(out)
	return out
}

func rules(pairs ...string) []importstorage.PathEquivalenceRule {
	var out []importstorage.PathEquivalenceRule
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, importstorage.PathEquivalenceRule{PathA: pairs[i], PathB: pairs[i+1]})
	}
	return CleanPathEquivalenceRules(out)
}

func assertExpanded(t *testing.T, path string, r []importstorage.PathEquivalenceRule, want []string) {
	t.Helper()
	got := ExpandEquivalentPaths(path, r)
	gotSorted := sortedLower(got)
	wantSorted := sortedLower(want)
	if len(gotSorted) != len(wantSorted) {
		t.Fatalf("ExpandEquivalentPaths(%q): got %v, want %v", path, got, want)
	}
	for i := range gotSorted {
		if gotSorted[i] != wantSorted[i] {
			t.Fatalf("ExpandEquivalentPaths(%q): got %v, want %v", path, got, want)
		}
	}
}

func TestExpandEquivalentPaths_NoRules_Passthrough(t *testing.T) {
	assertExpanded(t, `C:\Photos\2020\a.jpg`, nil, []string{`C:\Photos\2020\a.jpg`})
}

func TestExpandEquivalentPaths_ExactRootMatch(t *testing.T) {
	r := rules(`C:\NonOneDrive\iCloud Sorted`, `D:\NonOneDrive\iCloud Sorted`)
	assertExpanded(t, `C:\NonOneDrive\iCloud Sorted\2020\a.jpg`, r, []string{
		`C:\NonOneDrive\iCloud Sorted\2020\a.jpg`,
		`D:\NonOneDrive\iCloud Sorted\2020\a.jpg`,
	})
}

func TestExpandEquivalentPaths_SubdirectoryDepth(t *testing.T) {
	// Rule defined deep in the tree, not at the drive root — "anywhere up
	// the directory tree" per the feature request.
	r := rules(`C:\Users\Dave\Photos\2020`, `D:\Archive\2020`)
	assertExpanded(t, `C:\Users\Dave\Photos\2020\Summer\a.jpg`, r, []string{
		`C:\Users\Dave\Photos\2020\Summer\a.jpg`,
		`D:\Archive\2020\Summer\a.jpg`,
	})
}

func TestExpandEquivalentPaths_BidirectionalMatch(t *testing.T) {
	r := rules(`C:\Old`, `D:\New`)
	// Matching the "B" side of the rule should rewrite back to the "A" side.
	assertExpanded(t, `D:\New\a.jpg`, r, []string{
		`D:\New\a.jpg`,
		`C:\Old\a.jpg`,
	})
}

func TestExpandEquivalentPaths_SiblingDirectoryNotMatched(t *testing.T) {
	// A rule for "C:\Foo" must never match "C:\FooBar" — whole-segment match only.
	r := rules(`C:\Foo`, `D:\Foo`)
	assertExpanded(t, `C:\FooBar\a.jpg`, r, []string{`C:\FooBar\a.jpg`})
}

func TestExpandEquivalentPaths_CaseInsensitive(t *testing.T) {
	r := rules(`c:\photos`, `D:\Photos`)
	assertExpanded(t, `C:\PHOTOS\a.jpg`, r, []string{
		`C:\PHOTOS\a.jpg`,
		`D:\Photos\a.jpg`,
	})
}

func TestExpandEquivalentPaths_TransitiveChaining(t *testing.T) {
	// A≡B and A≡C should imply B≡C for matching purposes (a 3-way drive move
	// without the user having to enumerate every pairwise combination).
	r := rules(
		`C:\Photos`, `D:\Photos`,
		`C:\Photos`, `E:\Photos`,
	)
	assertExpanded(t, `D:\Photos\a.jpg`, r, []string{
		`D:\Photos\a.jpg`,
		`C:\Photos\a.jpg`,
		`E:\Photos\a.jpg`,
	})
}

func TestExpandEquivalentPaths_ExactPathEqualsRule(t *testing.T) {
	// The candidate path is exactly the rule's root (no trailing suffix).
	r := rules(`C:\Photos`, `D:\Photos`)
	assertExpanded(t, `C:\Photos`, r, []string{`C:\Photos`, `D:\Photos`})
}

func TestExpandEquivalentPaths_NoDuplicatesWhenRuleIrrelevant(t *testing.T) {
	r := rules(`C:\Unrelated`, `D:\Unrelated`)
	assertExpanded(t, `C:\Other\a.jpg`, r, []string{`C:\Other\a.jpg`})
}
