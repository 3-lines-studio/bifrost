//go:build !windows

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

	uploadScreen := get(t, base+"/upload", "")
	for _, want := range []string{`<meta name="robots" content="noindex">`, `id="mygo"`} {
		if !strings.Contains(uploadScreen, want) {
			t.Errorf("upload misses %q:\n%s", want, uploadScreen)
		}
	}
	if strings.Contains(uploadScreen, "<main>") {
		t.Errorf("upload has server-rendered content:\n%s", uploadScreen)
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
	for _, unwanted := range []string{"/dashboard", "/login", "/upload"} {
		if strings.Contains(sitemap, unwanted) {
			t.Errorf("sitemap lists the screen %s:\n%s", unwanted, sitemap)
		}
	}

	if robots := get(t, base+"/robots.txt", ""); !strings.Contains(robots, "Sitemap: https://demo.example.com/sitemap.xml") {
		t.Errorf("robots:\n%s", robots)
	}

	uploadFile(t, base)

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

// uploadFile sends a file in parts, as the app does, and reads it back from the
// server: start, sign, put every part, complete, and the file the server joined.
func uploadFile(t *testing.T, base string) {
	t.Helper()
	size := int64(2*1024*1024 + 512*1024)
	content := make([]byte, size)
	for i := range content {
		content[i] = byte(i*17 + i/5)
	}
	started := postJSON(t, base+"/api/upload/start", fmt.Sprintf(`{"name":"big.jpg","size":%d,"type":"image/jpeg"}`, size), http.StatusOK)
	var plan struct {
		UploadID string `json:"uploadId"`
		PartSize int64  `json:"partSize"`
	}
	if err := json.Unmarshal([]byte(started), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.UploadID == "" || plan.PartSize <= 0 {
		t.Fatalf("plan %+v", plan)
	}
	count := int((size + plan.PartSize - 1) / plan.PartSize)
	if count != 3 {
		t.Fatalf("%d parts of %d bytes", count, plan.PartSize)
	}
	numbers := make([]int, count)
	for i := range numbers {
		numbers[i] = i + 1
	}
	signBody, err := json.Marshal(map[string]any{"uploadId": plan.UploadID, "numbers": numbers})
	if err != nil {
		t.Fatal(err)
	}
	signed := postJSON(t, base+"/api/upload/sign", string(signBody), http.StatusOK)
	var urls struct {
		URLs map[string]string `json:"urls"`
	}
	if err := json.Unmarshal([]byte(signed), &urls); err != nil {
		t.Fatal(err)
	}

	type part struct {
		Number int    `json:"number"`
		ETag   string `json:"etag"`
		Size   int64  `json:"size"`
	}
	parts := make([]part, 0, count)
	for _, number := range numbers {
		start := int64(number-1) * plan.PartSize
		end := min(start+plan.PartSize, size)
		request, err := http.NewRequest(http.MethodPut, base+urls.URLs[strconv.Itoa(number)], bytes.NewReader(content[start:end]))
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		answer := read(t, response.Body)
		if response.StatusCode != http.StatusNoContent {
			t.Fatalf("part %d answered %d: %s", number, response.StatusCode, answer)
		}
		parts = append(parts, part{Number: number, ETag: strings.Trim(response.Header.Get("ETag"), `"`), Size: end - start})
	}

	body, err := json.Marshal(map[string]any{"uploadId": plan.UploadID, "parts": parts})
	if err != nil {
		t.Fatal(err)
	}
	joined := postJSON(t, base+"/api/upload/complete", string(body), http.StatusOK)
	var media struct {
		URL  string `json:"url"`
		Size int64  `json:"size"`
	}
	if err := json.Unmarshal([]byte(joined), &media); err != nil {
		t.Fatal(err)
	}
	if media.Size != size {
		t.Fatalf("the server joined %d bytes of %d", media.Size, size)
	}
	response := request(t, base+media.URL, "")
	got := read(t, response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("media answered %d", response.StatusCode)
	}
	if got != string(content) {
		t.Fatalf("the media is %d bytes, not the %d sent", len(got), len(content))
	}

	abandoned := postJSON(t, base+"/api/upload/start", fmt.Sprintf(`{"name":"small.jpg","size":%d,"type":"image/jpeg"}`, 10), http.StatusOK)
	var dropped struct {
		UploadID string `json:"uploadId"`
	}
	if err := json.Unmarshal([]byte(abandoned), &dropped); err != nil {
		t.Fatal(err)
	}
	postJSON(t, base+"/api/upload/abort", fmt.Sprintf(`{"uploadId":%q}`, dropped.UploadID), http.StatusNoContent)
	postJSON(t, base+"/api/upload/sign", fmt.Sprintf(`{"uploadId":%q,"numbers":[1]}`, dropped.UploadID), http.StatusNotFound)
}
