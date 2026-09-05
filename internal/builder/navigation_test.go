package builder

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/protocol"
)

func TestNavigationEntriesShareRouterAndLazyViews(t *testing.T) {
	root := t.TempDir()
	output := t.TempDir()
	if err := os.Mkdir(filepath.Join(output, "entries"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.tsx", "b.tsx", "plain.tsx"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("export function Page() { return null }"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	describe := protocol.DescribeResult{Spec: protocol.Spec{Routes: []protocol.RouteSpec{
		{Pattern: "/a", View: "a.tsx", Kind: "server", Navigation: true},
		{Pattern: "/b", View: "b.tsx", Kind: "server", Navigation: true},
		{Pattern: "/plain", View: "plain.tsx", Kind: "server"},
		{Pattern: "/same-source", View: "a.tsx", Kind: "server"},
	}}}
	plans, routes, err := planViews(describe, output)
	if err != nil {
		t.Fatal(err)
	}
	if routes["/a"] == routes["/same-source"] {
		t.Fatal("navigation and document entries shared an ID")
	}
	if err := writeEntries(root, output, plans, ""); err != nil {
		t.Fatal(err)
	}
	router, err := os.ReadFile(filepath.Join(output, "entries", "navigation.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(router), `import("`+filepath.Join(root, "a.tsx")+`")`) || !strings.Contains(string(router), `import("`+filepath.Join(root, "b.tsx")+`")`) || strings.Contains(string(router), "plain.tsx") {
		t.Fatalf("router = %s", router)
	}
	api, err := os.ReadFile(filepath.Join(output, "entries", "navigation-api.ts"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"export async function navigate", "export async function refresh", "export function setRouter"} {
		if !strings.Contains(string(api), expected) {
			t.Fatalf("navigation API lacks %q", expected)
		}
	}
	if !strings.Contains(string(router), `import { setRouter } from "./navigation-api"`) {
		t.Fatal("router does not use the shared navigation API")
	}
	for _, plan := range plans {
		entry, err := os.ReadFile(plan.ClientFile)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(entry), "start(page)") != plan.Navigation {
			t.Fatalf("entry = %s", entry)
		}
	}
}
