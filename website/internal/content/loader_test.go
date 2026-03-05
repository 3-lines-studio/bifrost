package content

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestContent(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	write("getting-started.md", `---
title: Getting Started
description: Learn how to use Bifrost
order: 1
---

# Getting Started

Install Bifrost:

`+"```"+`bash
go get github.com/3-lines-studio/bifrost
`+"```"+`
`)

	write("api.md", `---
title: API Reference
description: Full API documentation
order: 2
---

## Creating an App

Use `+"`bifrost.New()`"+` to create an app.
`)

	write("ignore.txt", "not a markdown file")

	return dir
}

func TestLoaderLoadAll(t *testing.T) {
	dir := setupTestContent(t)
	loader := NewLoader(dir)

	pages, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}

	if pages[0].Slug != "getting-started" {
		t.Errorf("first page slug = %q, want %q", pages[0].Slug, "getting-started")
	}
	if pages[0].Title != "Getting Started" {
		t.Errorf("first page title = %q, want %q", pages[0].Title, "Getting Started")
	}
	if pages[0].Order != 1 {
		t.Errorf("first page order = %d, want 1", pages[0].Order)
	}
	if pages[0].HTML == "" {
		t.Error("first page HTML is empty")
	}

	if pages[1].Slug != "api" {
		t.Errorf("second page slug = %q, want %q", pages[1].Slug, "api")
	}
	if pages[1].Order != 2 {
		t.Errorf("second page order = %d, want 2", pages[1].Order)
	}
}

func TestLoaderLoadAllOrdering(t *testing.T) {
	dir := t.TempDir()

	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	write("z-last.md", "---\ntitle: Last\norder: 3\n---\nLast page")
	write("a-first.md", "---\ntitle: First\norder: 1\n---\nFirst page")
	write("m-middle.md", "---\ntitle: Middle\norder: 2\n---\nMiddle page")

	loader := NewLoader(dir)
	pages, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	expected := []string{"a-first", "m-middle", "z-last"}
	for i, want := range expected {
		if pages[i].Slug != want {
			t.Errorf("pages[%d].Slug = %q, want %q", i, pages[i].Slug, want)
		}
	}
}

func TestBuildNav(t *testing.T) {
	pages := []Page{
		{Slug: "intro", Title: "Introduction"},
		{Slug: "api", Title: "API Reference"},
	}
	nav := BuildNav(pages)

	if len(nav) != 2 {
		t.Fatalf("expected 2 nav items, got %d", len(nav))
	}
	if nav[0].Slug != "intro" || nav[0].Title != "Introduction" {
		t.Errorf("nav[0] = %+v, unexpected", nav[0])
	}
}

func TestLoaderMissingDir(t *testing.T) {
	loader := NewLoader("/nonexistent/path")
	_, err := loader.LoadAll()
	if err == nil {
		t.Error("expected error for missing directory")
	}
}

func TestLoaderNoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plain.md"), []byte("# Just a heading\n\nSome text."), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(dir)
	pages, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	if len(pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(pages))
	}
	if pages[0].Title != "plain" {
		t.Errorf("title = %q, want %q (filename fallback)", pages[0].Title, "plain")
	}
	if pages[0].Order != 999 {
		t.Errorf("order = %d, want 999 (default)", pages[0].Order)
	}
}
