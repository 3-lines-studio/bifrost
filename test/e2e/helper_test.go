//nolint:errcheck // Test helpers - error handling deferred to test assertions
package e2e

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/3-lines-studio/bifrost"
	"github.com/gkampitakis/go-snaps/snaps"
)

var exampleDir string

func init() {
	_, filename, _, _ := runtime.Caller(0)
	testDir := filepath.Dir(filename)
	repoRoot := filepath.Join(testDir, "..", "..")
	exampleDir, _ = filepath.Abs(filepath.Join(repoRoot, "example"))
}

type testServer struct {
	app     *bifrost.App
	port    int
	devMode bool
	client  *http.Client
	origDir string
}

func newTestServer(t *testing.T, routes []bifrost.Route, devMode bool) *testServer {

	origDir, _ := os.Getwd()

	if devMode {
		t.Setenv("BIFROST_DEV", "1")
		if err := os.Chdir(exampleDir); err != nil {
			t.Fatalf("failed to chdir: %v", err)
		}
	} else {
		os.Unsetenv("BIFROST_DEV")
	}

	app, err := bifrost.New(BifrostFS, routes...)
	if err != nil {
		t.Fatalf("create app: %v", err)
	}

	port := getFreePort(t)

	t.Cleanup(func() {
		os.Chdir(origDir)
	})

	return &testServer{
		app:     app,
		port:    port,
		devMode: devMode,
		client:  &http.Client{Timeout: 10 * time.Second},
		origDir: origDir,
	}
}

type testServerWithWrap struct {
	app     *bifrost.App
	port    int
	devMode bool
	client  *http.Client
	origDir string
}

func newTestServerWithWrap(t *testing.T, routes []bifrost.Route, devMode bool) *testServerWithWrap {

	origDir, _ := os.Getwd()

	if devMode {
		t.Setenv("BIFROST_DEV", "1")
		if err := os.Chdir(exampleDir); err != nil {
			t.Fatalf("failed to chdir: %v", err)
		}
	} else {
		os.Unsetenv("BIFROST_DEV")
	}

	app, err := bifrost.New(BifrostFS, routes...)
	if err != nil {
		t.Fatalf("create app: %v", err)
	}

	port := getFreePort(t)

	t.Cleanup(func() {
		os.Chdir(origDir)
	})

	return &testServerWithWrap{
		app:     app,
		port:    port,
		devMode: devMode,
		client:  &http.Client{Timeout: 10 * time.Second},
		origDir: origDir,
	}
}

func (s *testServerWithWrap) start(t *testing.T) {
	t.Helper()

	// Create a ServeMux with API routes
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Wrap with Bifrost
	wrappedHandler := s.app.Wrap(mux)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: wrappedHandler,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			t.Logf("server error: %v", err)
		}
	}()

	time.Sleep(500 * time.Millisecond)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
		s.app.Stop()
	})
}

func (s *testServerWithWrap) url(path string) string {
	return fmt.Sprintf("http://localhost:%d%s", s.port, path)
}

func (s *testServerWithWrap) get(t *testing.T, path string) (*http.Response, string) {
	t.Helper()

	resp, err := s.client.Get(s.url(path))
	if err != nil {
		t.Fatalf("failed to GET %s: %v", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	return resp, string(body)
}

func (s *testServer) start(t *testing.T) {
	t.Helper()

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: s.app.Handler(),
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			t.Logf("server error: %v", err)
		}
	}()

	time.Sleep(500 * time.Millisecond)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
		s.app.Stop()
	})
}

func (s *testServer) url(path string) string {
	return fmt.Sprintf("http://localhost:%d%s", s.port, path)
}

func (s *testServer) get(t *testing.T, path string) (*http.Response, string) {
	t.Helper()

	resp, err := s.client.Get(s.url(path))
	if err != nil {
		t.Fatalf("failed to GET %s: %v", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	return resp, string(body)
}

func getFreePort(t *testing.T) int {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port
}

func normalizeHTML(html string) string {
	html = regexp.MustCompile(`data-rid="[^"]*"`).ReplaceAllString(html, `data-rid="[RID]"`)

	html = regexp.MustCompile(`nonce="[^"]*"`).ReplaceAllString(html, `nonce="[NONCE]"`)

	html = regexp.MustCompile(`\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}`).ReplaceAllString(html, "[TIMESTAMP]")

	html = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})?`).ReplaceAllString(html, "[ISO-TIMESTAMP]")

	html = regexp.MustCompile(`"[^"]*[-.][A-Za-z0-9_-]{6,}\.(js|css|mjs)"`).ReplaceAllString(html, `"[HASHED.$1]"`)

	// esbuild can split the same browser graph into different numbers of
	// chunks. Keep one marker for each asset role instead of snapshotting an
	// engine-specific chunk layout.
	html = regexp.MustCompile(`(?:<link rel="modulepreload" href="\[HASHED\.js\]" />\s*)+`).ReplaceAllString(html, `<link rel="modulepreload" href="[HASHED.js]" /> `)
	html = regexp.MustCompile(`(?:<script src="\[HASHED\.js\]" type="module"(?: defer)?></script>\s*)+`).ReplaceAllString(html, `<script src="[HASHED.js]" type="module"></script> `)

	// Critical CSS formatting and block ordering are backend-specific. Tailwind
	// compilation and candidate coverage have focused tests outside snapshots.
	html = regexp.MustCompile(`(?s)<style data-bifrost-critical>.*?</style>`).ReplaceAllString(html, `<style data-bifrost-critical>[CRITICAL CSS]</style>`)

	html = regexp.MustCompile(`id="[^"]*-[a-f0-9]{6,}"`).ReplaceAllString(html, `id="[ID]"`)

	// Strip hydration comment markers (non-deterministic placement)
	html = regexp.MustCompile(`<!--[^>]*-->`).ReplaceAllString(html, "")

	html = regexp.MustCompile(`\s+`).ReplaceAllString(html, " ")

	// Normalize absolute file paths in error stack traces (with parentheses)
	html = regexp.MustCompile(`\(/(?:home|Users)/[^)]+\)`).ReplaceAllString(html, "([FILE_PATH])")

	// Normalize relative file paths in error templates (e.g. src/App.tsx:2:1)
	html = regexp.MustCompile(`src/[^\s<>"'&;]+\.[a-z]+(:\d+(:\d+)?)?`).ReplaceAllString(html, "src/[FILE]:[LINE]:[COL]")

	// Normalize any absolute path (e.g. /home/user/project/src/...)
	html = regexp.MustCompile(`/(?:home|Users|tmp)/[^\s<>"'&;]+\.[a-z]+`).ReplaceAllString(html, "/[FILE_PATH]")

	// Async stack labels vary with bundle source shapes.
	html = strings.ReplaceAll(html, "async handleRender", "handleRender")

	// Stack trace byte sizes and frames vary by backend.
	html = regexp.MustCompile(`\(\d+ bytes\)`).ReplaceAllString(html, "([N] bytes)")
	html = regexp.MustCompile(`(?s)(<summary>Show stack \(\[N\] bytes\)</summary> <pre>).*?(</pre>)`).ReplaceAllString(html, `$1[STACK]$2`)

	return strings.TrimSpace(html)
}

func TestNormalizeHTMLBackendArtifacts(t *testing.T) {
	input := `<style data-bifrost-critical>.a { color: red }</style>` +
		`<link rel="modulepreload" href="/dist/chunk-A1B2C3D4.js" />` +
		`<link rel="modulepreload" href="/dist/page-Z9Y8X7W6.js" />` +
		`<script src="/dist/chunk-A1B2C3D4.js" type="module" defer></script>` +
		`<script src="/dist/page-Z9Y8X7W6.js" type="module"></script>`
	want := `<style data-bifrost-critical>[CRITICAL CSS]</style>` +
		`<link rel="modulepreload" href="[HASHED.js]" /> ` +
		`<script src="[HASHED.js]" type="module"></script>`
	if got := normalizeHTML(input); got != want {
		t.Fatalf("normalized HTML = %q, want %q", got, want)
	}
}

func assertHTTPStatus(t *testing.T, resp *http.Response, expected int) {
	t.Helper()
	if resp.StatusCode != expected {
		t.Errorf("expected status %d, got %d", expected, resp.StatusCode)
	}
}

func assertRedirect(t *testing.T, url string, expectedLocation string, expectedStatus int) {
	t.Helper()

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("failed to get %s: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != expectedStatus {
		t.Errorf("expected status %d, got %d for %s", expectedStatus, resp.StatusCode, url)
	}

	location := resp.Header.Get("Location")
	if location != expectedLocation {
		t.Errorf("expected redirect to %s, got %s", expectedLocation, location)
	}
}

func assertClientOnlyShell(t *testing.T, html string) {
	t.Helper()
	if !strings.Contains(html, `<div id="app"></div>`) {
		t.Error("expected an empty client-only app shell")
	}
}

func matchSnapshot(t *testing.T, name string, html string) {
	t.Helper()
	normalized := normalizeHTML(html)
	snaps.WithConfig(snaps.Ext(".html")).MatchSnapshot(t, normalized)
}

func TestMain(m *testing.M) {
	v := m.Run()
	snaps.Clean(m)
	os.Exit(v)
}
