package core

import (
	"context"
	"testing"
)

func TestStaticPathDataStructure(t *testing.T) {
	data := StaticPathData{
		Path: "/blog/test",
		Props: map[string]any{
			"title": "Test Post",
			"slug":  "test",
		},
	}

	if data.Path != "/blog/test" {
		t.Errorf("Expected Path to be /blog/test, got %s", data.Path)
	}

	if data.Props["title"] != "Test Post" {
		t.Errorf("Expected title prop, got %v", data.Props["title"])
	}
}

func TestStaticDataLoaderPathMatching(t *testing.T) {
	loader := func(ctx context.Context) ([]StaticPathData, error) {
		return []StaticPathData{
			{Path: "/blog/hello", Props: map[string]any{"title": "Hello"}},
			{Path: "/blog/world", Props: map[string]any{"title": "World"}},
		}, nil
	}

	entries, err := loader(context.Background())
	if err != nil {
		t.Fatalf("Loader failed: %v", err)
	}

	targetPath := "/blog/hello"
	var matchedProps map[string]any
	found := false

	for _, entry := range entries {
		if entry.Path == targetPath {
			matchedProps = entry.Props
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected to find path /blog/hello")
	}

	if matchedProps["title"] != "Hello" {
		t.Errorf("Expected title 'Hello', got %v", matchedProps["title"])
	}
}
