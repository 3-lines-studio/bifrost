//nolint:errcheck // Test helpers - error handling deferred to test assertions
package e2e

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"time"

	"github.com/3-lines-studio/bifrost"
	"github.com/3-lines-studio/bifrost/example"
	"github.com/gkampitakis/go-snaps/snaps"
)

var exampleDir string

func init() {
	_, filename, _, _ := runtime.Caller(0)
	testDir := filepath.Dir(filename)
	repoRoot := filepath.Join(testDir, "..", "..")
	exampleDir, _ = filepath.Abs(filepath.Join(repoRoot, "example"))
}

func bunAvailable() bool {
	_, err := exec.LookPath("bun")
	return err == nil
}

func skipIfNoBun(t *testing.T) {
	if !bunAvailable() {
		t.Skip("bun not available, skipping E2E test")
	}
}

type testServer struct {
	app     *bifrost.App
	port    int
	devMode bool
	client  *http.Client
	origDir string
}

func newTestServer(t *testing.T, routes []bifrost.Route, devMode bool) *testServer {
	skipIfNoBun(t)

	origDir, _ := os.Getwd()

	if devMode {
		t.Setenv("BIFROST_DEV", "1")
		os.Chdir(exampleDir)
	} else {
		os.Unsetenv("BIFROST_DEV")
	}

	app := bifrost.New(example.BifrostFS, routes...)

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
	skipIfNoBun(t)

	origDir, _ := os.Getwd()

	if devMode {
		t.Setenv("BIFROST_DEV", "1")
		os.Chdir(exampleDir)
	} else {
		os.Unsetenv("BIFROST_DEV")
	}

	app := bifrost.New(example.BifrostFS, routes...)

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

	html = regexp.MustCompile(`"[^"]*\.[a-f0-9]{8,}\.(js|css|mjs)"`).ReplaceAllString(html, `"[HASHED.$1]"`)

	html = regexp.MustCompile(`id="[^"]*-[a-f0-9]{6,}"`).ReplaceAllString(html, `id="[ID]"`)

	html = regexp.MustCompile(`\s+`).ReplaceAllString(html, " ")

	// Normalize all file paths in error stack traces
	html = regexp.MustCompile(`\(/(?:home|Users)/[^)]+\)`).ReplaceAllString(html, "([FILE_PATH])")

	return html
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
