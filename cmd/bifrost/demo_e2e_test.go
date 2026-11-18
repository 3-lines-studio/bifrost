//go:build !windows

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestEndToEndDemo builds the demo app of the Go pages, runs the server it
// produces without pointing it at a browser build on disk, and reads every
// route a person and a crawler reach: the pages a search engine keeps, the
// screens it does not, the API the app calls, and the wasm the binary carries.
//
//	BIFROST_E2E=1 go test ./cmd/bifrost -run TestEndToEndDemoApp -v
func TestEndToEndDemoApp(t *testing.T) {
	if os.Getenv("BIFROST_E2E") == "" {
		t.Skip("set BIFROST_E2E=1 to build and run the demo app")
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	copyTree(t, filepath.Join(repoRoot, "mygo", "demo"), directory)
	module, err := os.ReadFile(filepath.Join(directory, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	module = bytes.ReplaceAll(module, []byte("=> ../../"), []byte("=> "+repoRoot))
	module = bytes.ReplaceAll(module, []byte("=> ../"), []byte("=> "+filepath.Join(repoRoot, "mygo")))
	if err := os.WriteFile(filepath.Join(directory, "go.mod"), module, 0o644); err != nil {
		t.Fatal(err)
	}
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = directory
	if output, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy: %v\n%s", err, output)
	}
	if err := runRoutes([]string{"-C", directory}); err != nil {
		t.Fatalf("routes: %v", err)
	}
	if err := runBuild([]string{"-C", directory}); err != nil {
		t.Fatal(err)
	}

	address := freeAddress(t)
	base := "http://" + address
	server := exec.Command(filepath.Join(directory, ".bifrost", "bifrost-app"))
	server.Dir = directory
	server.Env = append(os.Environ(), "BIFROST_ADDR="+address, "BIFROST_SITE_URL=https://demo.example.com")
	t.Cleanup(func() {
		_ = server.Process.Kill()
		_, _ = server.Process.Wait()
	})
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	waitForServer(t, base+"/")

	home := get(t, base+"/", "")
	for _, want := range []string{
		"<title>Bifrost + MyGo</title>",
		`<link rel="canonical" href="https://demo.bifrost.run/">`,
		`<link rel="alternate" hreflang="es" href="https://demo.bifrost.run/es">`,
		`<meta name="keywords" content="golang, ssr, seo, webassembly, desktop">`,
		`<meta property="og:type" content="website">`,
		`<a href="/pricing">Pricing</a>`,
		`id="mygo"`,
	} {
		if !strings.Contains(home, want) {
			t.Errorf("home misses %q:\n%s", want, home)
		}
	}
	if got := strings.Count(home, `<script type="application/ld+json">`); got != 2 {
		t.Errorf("home has %d schema scripts, want 2", got)
	}
	if header := request(t, base+"/", "").Header.Get("Cache-Control"); !strings.Contains(header, "public") {
		t.Errorf("home is not cached: %q", header)
	}

	post := get(t, base+"/blog/one-source-three-targets", "")
	for _, want := range []string{
		`<meta property="og:type" content="article">`,
		`"@type":"BlogPosting"`,
		`<h1>One source, three targets</h1>`,
	} {
		if !strings.Contains(post, want) {
			t.Errorf("post misses %q:\n%s", want, post)
		}
	}

	cached := request(t, base+"/pricing", "")
	if !strings.Contains(read(t, cached.Body), "<h2>Solo · 0 USD a month</h2>") {
		t.Errorf("pricing does not draw the props the loader returned")
	}
	if !strings.Contains(cached.Header.Get("Cache-Control"), "public") {
		t.Errorf("pricing is not cached: %q", cached.Header.Get("Cache-Control"))
	}

	screen := get(t, base+"/dashboard", "")
	for _, want := range []string{`<meta name="robots" content="noindex">`, `id="mygo"`} {
		if !strings.Contains(screen, want) {
			t.Errorf("dashboard misses %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, "<main>") {
		t.Errorf("dashboard has server-rendered content:\n%s", screen)
	}

	failure := request(t, base+"/blog/boom", "")
	if failure.StatusCode != http.StatusInternalServerError {
		t.Errorf("failed page answered %d", failure.StatusCode)
	}
	failureBody := read(t, failure.Body)
	if !strings.Contains(failureBody, "Something went wrong") {
		t.Errorf("failed page shows no error page:\n%s", failureBody)
	}
	if strings.Contains(failure.Header.Get("Cache-Control"), "public") {
		t.Errorf("error page is cached: %q", failure.Header.Get("Cache-Control"))
	}

	missing := request(t, base+"/blog/nope", "")
	missingBody := read(t, missing.Body)
	if missing.StatusCode != http.StatusNotFound {
		t.Errorf("missing post answered %d", missing.StatusCode)
	}
	if !strings.Contains(missingBody, "We could not find that page") || !strings.Contains(missingBody, "<title>Not found") {
		t.Errorf("missing post is not the not-found page:\n%s", missingBody)
	}

	sitemap := get(t, base+"/sitemap.xml", "")
	for _, want := range []string{
		"<loc>https://demo.example.com/blog/one-source-three-targets</loc><lastmod>2026-01-12T00:00:00Z</lastmod>",
		"<loc>https://demo.example.com/pricing</loc>",
	} {
		if !strings.Contains(sitemap, want) {
			t.Errorf("sitemap misses %q:\n%s", want, sitemap)
		}
	}
	for _, unwanted := range []string{"/dashboard", "/login"} {
		if strings.Contains(sitemap, unwanted) {
			t.Errorf("sitemap lists the screen %s:\n%s", unwanted, sitemap)
		}
	}

	if robots := get(t, base+"/robots.txt", ""); !strings.Contains(robots, "Sitemap: https://demo.example.com/sitemap.xml") {
		t.Errorf("robots:\n%s", robots)
	}

	notes := get(t, base+"/api/notes", "")
	if !strings.Contains(notes, "The server renders the page") {
		t.Errorf("notes: %s", notes)
	}
	added := postJSON(t, base+"/api/notes", `{"title":"Written by the test"}`, http.StatusCreated)
	if !strings.Contains(added, "Written by the test") {
		t.Errorf("added note: %s", added)
	}
	postJSON(t, base+"/api/notes", `{"title":" "}`, http.StatusBadRequest)
	postJSON(t, base+"/api/notes", `not json`, http.StatusBadRequest)

	login := postJSON(t, base+"/api/login", `{"email":"ana@example.com","password":"secret"}`, http.StatusOK)
	var session struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(login), &session); err != nil || session.Token == "" {
		t.Fatalf("login answered %s", login)
	}
	postJSON(t, base+"/api/login", `{"email":"ana@example.com","password":"x"}`, http.StatusUnauthorized)

	if response := request(t, base+"/api/me", ""); response.StatusCode != http.StatusUnauthorized {
		t.Errorf("me without a token answered %d", response.StatusCode)
	}
	me := getWithHeader(t, base+"/api/me", "Authorization", "Bearer "+session.Token)
	if !strings.Contains(me, `"email":"ana@example.com"`) {
		t.Errorf("me: %s", me)
	}
	refreshed := postWithHeader(t, base+"/api/refresh", "Authorization", "Bearer "+session.Token)
	if !strings.Contains(refreshed, `"token":"demo.ana@example.com.`) {
		t.Errorf("refresh: %s", refreshed)
	}

	// The browser build the binary carries: no BIFROST_WASM_DIR points at a
	// directory, so the wasm comes from the embedded copy.
	response := requestWithHeader(t, base+"/app.wasm", "Accept-Encoding", "br")
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Encoding") != "br" {
		t.Fatalf("wasm answered %d %q", response.StatusCode, response.Header.Get("Content-Encoding"))
	}
	encoded := len(read(t, response.Body))
	if encoded < 1<<20 {
		t.Fatalf("the embedded wasm has %d bytes", encoded)
	}
	if body := get(t, base+"/wasm_exec.js", ""); !strings.Contains(body, "WebAssembly") {
		t.Errorf("wasm_exec.js is not the runtime of the toolchain")
	}
	if response := requestWithHeader(t, base+"/__bifrost/reload", "Accept", "text/event-stream"); response.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("the reload stream answered %q", response.Header.Get("Content-Type"))
	} else {
		response.Body.Close()
	}
}

func getWithHeader(t *testing.T, url, name, value string) string {
	t.Helper()
	return read(t, requestWithHeader(t, url, name, value).Body)
}

func postJSON(t *testing.T, url, body string, status int) string {
	t.Helper()
	response, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status {
		t.Fatalf("POST %s answered %d, want %d", url, response.StatusCode, status)
	}
	return read(t, response.Body)
}

func postWithHeader(t *testing.T, url, name, value string) string {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set(name, value)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("POST %s answered %d", url, response.StatusCode)
	}
	return read(t, response.Body)
}

// copyTree copies a directory of an app, leaving what a build generated behind.
func copyTree(t *testing.T, source, target string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".bifrost", "dist", "node_modules":
				if relative != "." {
					return filepath.SkipDir
				}
			}
			return os.MkdirAll(filepath.Join(target, relative), 0o755)
		}
		if entry.Name() == "zz_bifrost_routes.go" || entry.Name() == "go.sum" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(target, relative), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}
