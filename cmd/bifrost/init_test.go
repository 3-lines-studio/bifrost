package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunInitCreatesFormattedScaffoldAndRefusesOverwrite(t *testing.T) {
	target := filepath.Join(t.TempDir(), "app")
	if err := runInit([]string{"--no-install", target}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"app/layout.tsx", "app/page.tsx", "app/page.go", "app/style.css", "package.json", "vite.config.ts", "bifrost.d.ts"} {
		if _, err := os.Stat(filepath.Join(target, filepath.FromSlash(name))); err != nil {
			t.Fatalf("the scaffold is missing %s: %v", name, err)
		}
	}
	pageData, err := os.ReadFile(filepath.Join(target, "app", "page.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"func Load(r *http.Request) (any, error)", "bifrost.PageData"} {
		if !strings.Contains(string(pageData), expected) {
			t.Fatalf("app/page.go does not contain %q", expected)
		}
	}
	layoutData, err := os.ReadFile(filepath.Join(target, "app", "layout.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(layoutData), "export function Layout") || !strings.Contains(string(layoutData), "./style.css") {
		t.Fatalf("app/layout.tsx is incomplete:\n%s", layoutData)
	}
	viteData, err := os.ReadFile(filepath.Join(target, "vite.config.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(viteData), "@tailwindcss/vite") || !strings.Contains(string(viteData), "@vitejs/plugin-react") {
		t.Fatalf("vite.config.ts is incomplete:\n%s", viteData)
	}
	types, err := os.ReadFile(filepath.Join(target, "bifrost.d.ts"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"virtual:bifrost/navigation", "navigate(href: string): Promise<void>", "replace(href: string): Promise<void>", "usePathname(): string", "useSearchParams(): URLSearchParams", "export function Link"} {
		if !strings.Contains(string(types), expected) {
			t.Fatalf("bifrost.d.ts does not contain %q", expected)
		}
	}
	moduleData, err := os.ReadFile(filepath.Join(target, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(moduleData), "require") {
		t.Fatalf("go.mod pins a bifrost version instead of letting go get pick one:\n%s", moduleData)
	}
	if err := runInit([]string{"--no-install", target}); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("second init error = %v", err)
	}
}

func TestRunInitScaffoldsTheClassicRouterWithClassic(t *testing.T) {
	target := filepath.Join(t.TempDir(), "classic")
	if err := runInit([]string{"--no-install", "--classic", target}); err != nil {
		t.Fatal(err)
	}
	mainData, err := os.ReadFile(filepath.Join(target, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"bifrost.Building()", "bifrost.Server", "app.Close", "BIFROST_ADDR"} {
		if !strings.Contains(string(mainData), expected) {
			t.Fatalf("main.go does not contain %q", expected)
		}
	}
	if _, err := os.Stat(filepath.Join(target, "pages", "home.tsx")); err != nil {
		t.Fatalf("the classic scaffold is missing pages/home.tsx: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "app")); !os.IsNotExist(err) {
		t.Fatalf("the classic scaffold wrote an app directory: %v", err)
	}
}

func TestTreeStampIgnoresBuildOutput(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := goTreeStamp(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".bifrost"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".bifrost", "asset.js"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := goTreeStamp(root)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("build output changed tree stamp: %q != %q", first, second)
	}
}
