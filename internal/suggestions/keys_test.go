package suggestions

import "testing"

func TestBuildKeyAndSplitKey(t *testing.T) {
	key, err := BuildKey("Getting started", "Early life overview")
	if err != nil {
		t.Fatal(err)
	}
	if key != "Getting started::Early life overview" {
		t.Fatalf("key = %q", key)
	}
	category, title, err := SplitKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if category != "Getting started" || title != "Early life overview" {
		t.Fatalf("split = %q / %q", category, title)
	}
}

func TestSplitKeyWithDelimiterInTitle(t *testing.T) {
	key := "Cat::Part::two"
	category, title, err := SplitKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if category != "Cat" || title != "Part::two" {
		t.Fatalf("split = %q / %q", category, title)
	}
}
