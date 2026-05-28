package service

import "testing"

func TestBuildArtefactEmbeddingInput(t *testing.T) {
	desc := "  A   fine   object  "
	tags := " bronze, Silver , bronze "
	story := "Found in\n the attic"
	got := BuildArtefactEmbeddingInput("Teapot", &desc, &tags, &story)
	want := "Name: Teapot\nDescription: A fine object\nTags: bronze, silver\nStory: Found in the attic"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBuildArtefactEmbeddingInputEmpty(t *testing.T) {
	if got := BuildArtefactEmbeddingInput("  ", nil, nil, nil); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}
