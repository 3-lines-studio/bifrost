//go:build !windows

package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestEndToEndNativeApp builds the scaffold of native pages, runs the server
// it produces, and reads what a browser and a crawler would get. It builds a
// wasm and runs a binary, so it is skipped unless BIFROST_E2E is set.
//
//	BIFROST_E2E=1 go test ./cmd/bifrost -run TestEndToEndNativeApp -v
func TestEndToEndNativeApp(t *testing.T) {
	if os.Getenv("BIFROST_E2E") == "" {
		t.Skip("set BIFROST_E2E=1 to build and run a native app")
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err := runInit([]string{"--no-install", "--native", directory}); err != nil {
		t.Fatal(err)
	}
	module := fmt.Sprintf(`module example.com/e2e

go 1.27.3

require (
	github.com/3-lines-studio/bifrost v0.0.0
	github.com/3-lines-studio/bifrost/mygo v0.0.0
	github.com/egoist/mygo v0.10.0
)

replace github.com/3-lines-studio/bifrost => %s

replace github.com/3-lines-studio/bifrost/mygo => %s
`, repoRoot, filepath.Join(repoRoot, "mygo"))
	if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte(module), 0o644); err != nil {
		t.Fatal(err)
	}
	screenDirectory := filepath.Join(directory, "app", "settings")
	if err := os.MkdirAll(screenDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	screen := `package settings

import "github.com/egoist/mygo/ui"

func Screen() func(*ui.Context) {
	return func(c *ui.Context) {
		ui.Text(c, "Settings").FontSize(24).Bold().Role(ui.RoleHeading).Level(1)
	}
}
`
	if err := os.WriteFile(filepath.Join(screenDirectory, "page.go"), []byte(screen), 0o644); err != nil {
		t.Fatal(err)
	}
	brokenDirectory := filepath.Join(directory, "app", "broken")
	if err := os.MkdirAll(brokenDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	broken := `package broken

import (
	"errors"
	"net/http"

	"github.com/egoist/mygo/ui"
)

func Load(*http.Request) (any, error) {
	return nil, errors.New("the store is down")
}

func View() func(*ui.Context) {
	return func(c *ui.Context) {
		ui.Text(c, "Broken").FontSize(24).Bold().Role(ui.RoleHeading).Level(1)
	}
}
`
	if err := os.WriteFile(filepath.Join(brokenDirectory, "page.go"), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = directory
	if output, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy: %v\n%s", err, output)
	}
	if err := runBuild([]string{"-C", directory}); err != nil {
		t.Fatal(err)
	}

	address := freeAddress(t)
	var output bytes.Buffer
	server := exec.Command(filepath.Join(directory, ".bifrost", "bifrost-app"))
	server.Dir = directory
	server.Env = append(os.Environ(),
		"BIFROST_ADDR="+address,
		"BIFROST_SITE_URL=https://example.com",
		"BIFROST_WASM_DIR="+filepath.Join(directory, "dist", "web"),
	)
	server.Stdout = &output
	server.Stderr = &output
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = server.Process.Kill()
		_, _ = server.Process.Wait()
		if t.Failed() {
			t.Logf("server output:\n%s", output.String())
		}
	})
	waitForServer(t, "http://"+address+"/")

	home := get(t, "http://"+address+"/", "")
	for _, want := range []string{"<title>", "<h1>", `<a href="/about">`, `id="mygo"`, "setTimeout"} {
		if !strings.Contains(home, want) {
			t.Errorf("home misses %q:\n%s", want, home)
		}
	}
	for _, unwanted := range []string{"Loading", "#/"} {
		if strings.Contains(home, unwanted) {
			t.Errorf("home has %q:\n%s", unwanted, home)
		}
	}

	brokenPage := request(t, "http://"+address+"/broken", "")
	brokenBody := read(t, brokenPage.Body)
	if brokenPage.StatusCode != http.StatusInternalServerError {
		t.Errorf("broken page answered %d", brokenPage.StatusCode)
	}
	for _, want := range []string{"Something went wrong", `<meta name="robots" content="noindex">`} {
		if !strings.Contains(brokenBody, want) {
			t.Errorf("broken page misses %q:\n%s", want, brokenBody)
		}
	}
	if got := brokenPage.Header.Get("Cache-Control"); strings.Contains(got, "public") {
		t.Errorf("error page is cached: %q", got)
	}

	screenPage := get(t, "http://"+address+"/settings", "")
	if !strings.Contains(screenPage, `<meta name="robots" content="noindex">`) {
		t.Errorf("screen is indexable:\n%s", screenPage)
	}
	if strings.Contains(screenPage, "<main>") {
		t.Errorf("screen has server-rendered content:\n%s", screenPage)
	}

	sitemap := get(t, "http://"+address+"/sitemap.xml", "")
	for _, want := range []string{"<loc>https://example.com/</loc>", "https://example.com/about"} {
		if !strings.Contains(sitemap, want) {
			t.Errorf("sitemap misses %q:\n%s", want, sitemap)
		}
	}
	if strings.Contains(sitemap, "/settings") {
		t.Errorf("sitemap lists a screen:\n%s", sitemap)
	}

	robots := get(t, "http://"+address+"/robots.txt", "")
	if !strings.Contains(robots, "Sitemap: https://example.com/sitemap.xml") {
		t.Errorf("robots:\n%s", robots)
	}

	response := request(t, "http://"+address+"/missing", "")
	if response.StatusCode != http.StatusNotFound || !strings.Contains(read(t, response.Body), "Not found") {
		t.Errorf("missing page answered %d", response.StatusCode)
	}

	response = requestWithHeader(t, "http://"+address+"/app.wasm", "Accept-Encoding", "br")
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Encoding") != "br" {
		t.Fatalf("wasm answered %d %q", response.StatusCode, response.Header.Get("Content-Encoding"))
	}
	wasm, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if len(wasm) < 1<<20 {
		t.Fatalf("wasm has %d bytes", len(wasm))
	}
}

func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func waitForServer(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(url)
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s did not answer", url)
}

func get(t *testing.T, url, accept string) string {
	t.Helper()
	return read(t, request(t, url, accept).Body)
}

func request(t *testing.T, url, accept string) *http.Response {
	t.Helper()
	return requestWithHeader(t, url, "Accept-Encoding", accept)
}

func requestWithHeader(t *testing.T, url, name, value string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if value != "" {
		request.Header.Set(name, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func read(t *testing.T, body io.ReadCloser) string {
	t.Helper()
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
