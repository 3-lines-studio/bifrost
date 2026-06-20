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

	props, ok := data.Props.(map[string]any)
	if !ok {
		t.Fatal("expected map props")
	}
	if props["title"] != "Test Post" {
		t.Errorf("Expected title prop, got %v", props["title"])
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
	var matchedProps any
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

	m, ok := matchedProps.(map[string]any)
	if !ok {
		t.Fatal("expected map props")
	}
	if m["title"] != "Hello" {
		t.Errorf("Expected title 'Hello', got %v", m["title"])
	}
}

func TestStaticPathData_StructProps(t *testing.T) {
	type blogProps struct {
		Title string `json:"title"`
	}

	data := StaticPathData{
		Path:  "/blog/typed",
		Props: blogProps{Title: "Typed Post"},
	}

	if data.Path != "/blog/typed" {
		t.Errorf("Expected Path /blog/typed, got %s", data.Path)
	}

	p, ok := data.Props.(blogProps)
	if !ok {
		t.Fatalf("expected original struct props, got %T", data.Props)
	}
	if p.Title != "Typed Post" {
		t.Errorf("Expected title 'Typed Post', got %v", p.Title)
	}
}
