package bifrost

import (
	"context"
	"testing"
)

func TestDevModeWithStaticData(t *testing.T) {
	t.Setenv("BIFROST_DEV", "1")

	loader := func(ctx context.Context) ([]StaticPathData, error) {
		return []StaticPathData{
			{
				Path: "/blog/hello",
				Props: map[string]any{
					"title": "Hello Post",
					"body":  "Hello content",
				},
			},
			{
				Path: "/blog/world",
				Props: map[string]any{
					"title": "World Post",
					"body":  "World content",
				},
			},
		}, nil
	}

	route := Page("/blog", "./blog.tsx", WithStaticData(loader))

	app := New(testFS, route)
	defer app.Stop()

	config := app.pageConfigs["./blog.tsx"]
	if config == nil {
		t.Fatal("Config not stored")
	}

	if config.StaticDataLoader == nil {
		t.Error("StaticDataLoader not set")
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
