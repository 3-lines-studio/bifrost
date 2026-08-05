package esbuild

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	sobekrenderer "github.com/3-lines-studio/bifrost/internal/adapters/sobek"
	"github.com/3-lines-studio/bifrost/internal/core"
)

func TestBuilderBuildsReactSSRAndClient(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	component := filepath.Join(root, "page.tsx")
	ssrEntry := filepath.Join(root, "page-ssr.tsx")
	clientEntry := filepath.Join(root, "page-client.tsx")
	writeTestFile(t, component, `import React from "react"; export function Page({name}) { return <main className="flex bg-red-500">Hello {name}</main> }; export function Head(){ return <title>Test</title> }`)
	writeTestFile(t, ssrEntry, `import React from "react"; import {renderToString} from "react-dom/server"; import {Page, Head} from "./page"; export function render(props){ return {html: renderToString(<Page {...props}/>), head: renderToString(<Head/>)} }`)
	writeTestFile(t, clientEntry, `import React from "react"; import {hydrateRoot} from "react-dom/client"; import {Page} from "./page"; hydrateRoot(document.getElementById("app"), <Page name="Client"/>);`)

	// Resolve React from this repository instead of installing packages in the fixture.
	if err := os.Symlink(filepath.Join(repoRoot, "node_modules"), filepath.Join(root, "node_modules")); err != nil {
		t.Fatal(err)
	}

	builder := NewBuilder(core.ModeProd)
	ssrOut := filepath.Join(root, "ssr")
	if err := builder.BuildSSR([]string{ssrEntry}, ssrOut); err != nil {
		t.Fatal(err)
	}
	ssrCode, err := os.ReadFile(filepath.Join(ssrOut, "page-ssr.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ssrCode), "render") {
		t.Fatal("SSR output has no render export")
	}

	clientOut := filepath.Join(root, "dist")
	built, err := builder.Build([]string{clientEntry}, clientOut, []string{"page"})
	if err != nil {
		t.Fatal(err)
	}
	if built["page"].Script == "" {
		t.Fatal("client output has no script")
	}
}

func TestBuilderSSRRegistryLazilyIsolatesImportFailures(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	goodEntry := filepath.Join(root, "good.ts")
	badEntry := filepath.Join(root, "bad.ts")
	writeTestFile(t, goodEntry, `export function render(props) { return {head: "good", html: props.value}; }`)
	writeTestFile(t, badEntry, `throw new Error("broken import"); export function render() { return {head: "", html: "bad"}; }`)
	if err := os.Symlink(filepath.Join(repoRoot, "node_modules"), filepath.Join(root, "node_modules")); err != nil {
		t.Fatal(err)
	}

	bundle, exports, err := NewBuilder(core.ModeProd).BuildSSRRegistry([]string{goodEntry, badEntry}, filepath.Join(root, "ssr"))
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := sobekrenderer.NewRenderer(core.ModeProd, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	goodTarget := bundle + "#" + exports[goodEntry]
	badTarget := bundle + "#" + exports[badEntry]
	good, err := renderer.Render(goodTarget, map[string]any{"value": "first"})
	if err != nil {
		t.Fatal(err)
	}
	if good.Head != "good" || good.Body != "first" {
		t.Fatalf("unexpected good page: %+v", good)
	}
	if _, err := renderer.Render(badTarget, nil); err == nil || !strings.Contains(err.Error(), "broken import") {
		t.Fatalf("bad registry page error = %v", err)
	}
	good, err = renderer.Render(goodTarget, map[string]any{"value": "after"})
	if err != nil {
		t.Fatal(err)
	}
	if good.Body != "after" {
		t.Fatalf("good page did not recover after sibling failure: %+v", good)
	}
}

func TestBuilderCompilesTailwindInSobek(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "style.css"), `@import "tailwindcss";`)
	writeTestFile(t, filepath.Join(root, "page.tsx"), `import "./style.css"; export const classes = "flex bg-red-500 dark:bg-neutral-950";`)
	if err := os.Symlink(filepath.Join(repoRoot, "example", "node_modules"), filepath.Join(root, "node_modules")); err != nil {
		t.Fatal(err)
	}

	built, err := NewBuilder(core.ModeProd).Build(
		[]string{filepath.Join(root, "page.tsx")},
		filepath.Join(root, "dist"),
		[]string{"page"},
	)
	if err != nil {
		t.Fatal(err)
	}
	cssPath := filepath.Join(root, strings.TrimPrefix(built["page"].CSS, "/"))
	cssPath = filepath.Join(root, "dist", filepath.Base(cssPath))
	css, err := os.ReadFile(cssPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{".flex", ".bg-red-500", ".dark\\:bg-neutral-950"} {
		if !strings.Contains(string(css), want) {
			t.Fatalf("Tailwind output does not contain %q", want)
		}
	}
}

func TestCollectTailwindCandidates(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "page.tsx")
	writeTestFile(t, path, `const x = <div className="flex dark:bg-neutral-950 sm:grid-cols-2" />`)
	got := collectTailwindCandidates(map[string]struct{}{path: {}})
	for _, want := range []string{"flex", "dark:bg-neutral-950", "sm:grid-cols-2"} {
		if !contains(got, want) {
			t.Fatalf("candidate %q missing from %v", want, got)
		}
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func contains(values []string, want string) bool {
	return slices.Contains(values, want)
}
