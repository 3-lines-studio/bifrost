//nolint:errcheck // Test helpers - error handling deferred to test assertions
package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/3-lines-studio/bifrost"
	examplesvelte "github.com/3-lines-studio/bifrost/example-svelte"
)

var exampleSvelteDir string

func init() {
	_, filename, _, _ := runtime.Caller(0)
	testDir := filepath.Dir(filename)
	repoRoot := filepath.Join(testDir, "..", "..")
	exampleSvelteDir, _ = filepath.Abs(filepath.Join(repoRoot, "example-svelte"))
}

type svelteTestServer struct {
	app     *bifrost.App
	port    int
	devMode bool
	client  *http.Client
	origDir string
}

func newSvelteTestServer(t *testing.T, routes []bifrost.Route, devMode bool) *svelteTestServer {
	skipIfNoBun(t)

	origDir, _ := os.Getwd()

	if devMode {
		t.Setenv("BIFROST_DEV", "1")
		os.Chdir(exampleSvelteDir)
	} else {
		os.Unsetenv("BIFROST_DEV")
	}

	app := bifrost.NewWithFramework(examplesvelte.BifrostFS, bifrost.Svelte, routes...)

	port := getFreePort(t)

	t.Cleanup(func() {
		os.Chdir(origDir)
	})

	return &svelteTestServer{
		app:     app,
		port:    port,
		devMode: devMode,
		client:  &http.Client{Timeout: 10 * time.Second},
		origDir: origDir,
	}
}

func (s *svelteTestServer) start(t *testing.T) {
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

func (s *svelteTestServer) url(path string) string {
	return fmt.Sprintf("http://localhost:%d%s", s.port, path)
}

func (s *svelteTestServer) get(t *testing.T, path string) (*http.Response, string) {
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
