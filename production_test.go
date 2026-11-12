package bifrost

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/3-lines-studio/bifrost/internal/protocol"
)

type readinessRenderer struct{ err error }

func (*readinessRenderer) Render(context.Context, renderRequest, renderSink) error { return nil }
func (*readinessRenderer) Close(context.Context) error                             { return nil }
func (r *readinessRenderer) Ready(context.Context) error                           { return r.err }

func TestRegisterReadyEndpoint(t *testing.T) {
	app := &App{runtime: &runtimeState{}}
	mux := http.NewServeMux()
	if err := app.Register(mux); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/_bifrost/ready", nil)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("ready status = %d", recorder.Code)
	}
	app.runtime.renderer = &readinessRenderer{err: errors.New("not ready")}
	recorder = httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("not ready status = %d", recorder.Code)
	}
}

func TestAppReady(t *testing.T) {
	want := errors.New("not ready")
	app := &App{runtime: &runtimeState{renderer: &readinessRenderer{err: want}}}
	if err := app.Ready(context.Background()); !errors.Is(err, want) {
		t.Fatalf("Ready error = %v", err)
	}
	app.runtime.renderer = nil
	if err := app.Ready(context.Background()); err != nil {
		t.Fatalf("static app readiness = %v", err)
	}
	if err := (&App{}).Ready(context.Background()); err == nil {
		t.Fatal("app without runtime reported ready")
	}
}

func TestRendererQueueHookReportsOverload(t *testing.T) {
	var event QueueEvent
	renderer := &productionRenderer{
		admission: make(chan struct{}, 1),
		idle:      make(chan *rendererWorker),
		queueWait: 10 * time.Millisecond,
		queueHooks: []QueueHook{func(_ context.Context, received QueueEvent) {
			event = received
		}},
	}
	renderer.admission <- struct{}{}
	err := renderer.Render(context.Background(), renderRequest{Pattern: "/busy"}, nil)
	if !errors.Is(err, ErrRendererBusy) || !errors.Is(event.Err, ErrRendererBusy) || event.Pattern != "/busy" {
		t.Fatalf("render error = %v, event = %+v", err, event)
	}
}

func TestRendererQueueWaitsBeforeReportingBusy(t *testing.T) {
	renderer := &productionRenderer{
		admission: make(chan struct{}, 1),
		idle:      make(chan *rendererWorker),
		queueWait: 30 * time.Millisecond,
	}
	renderer.admission <- struct{}{}
	started := time.Now()
	err := renderer.Render(context.Background(), renderRequest{Pattern: "/busy"}, nil)
	if !errors.Is(err, ErrRendererBusy) {
		t.Fatalf("render error = %v", err)
	}
	if waited := time.Since(started); waited < renderer.queueWait {
		t.Fatalf("render gave up after %v, before the %v queue wait", waited, renderer.queueWait)
	}
}

func TestRendererBusyResponseCarriesRetryAfter(t *testing.T) {
	recorder := httptest.NewRecorder()
	serveDefaultError(recorder, httptest.NewRequest(http.MethodGet, "/busy", nil), ErrRendererBusy)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", recorder.Code)
	}
	if got := recorder.Header().Get("Retry-After"); got != renderRetryAfter {
		t.Fatalf("Retry-After = %q", got)
	}
}

func TestRendererCloseWaitsForActiveWork(t *testing.T) {
	renderer := &productionRenderer{}
	renderer.active.Add(1)
	done := make(chan error, 1)
	go func() { done <- renderer.Close(context.Background()) }()
	select {
	case err := <-done:
		t.Fatalf("Close returned before drain: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	renderer.active.Done()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not return after drain")
	}
}

func TestRendererRestartRateLimit(t *testing.T) {
	worker := &rendererWorker{}
	now := time.Unix(1_000, 0)
	for attempt := range 5 {
		if !worker.allowRestart(now.Add(time.Duration(attempt)*time.Second), nil) {
			t.Fatalf("attempt %d was rejected", attempt+1)
		}
	}
	if worker.allowRestart(now.Add(5*time.Second), nil) {
		t.Fatal("sixth restart in one minute was allowed")
	}
	if !worker.allowRestart(now.Add(61*time.Second), nil) {
		t.Fatal("restart limit did not reset after one minute")
	}
}

func TestExtractReaderReusesExtractedArtifacts(t *testing.T) {
	root := t.TempDir()
	if err := extractReader(root, "runtime/renderer", strings.NewReader("first"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := extractReader(root, "runtime/renderer", strings.NewReader("second"), 0o700); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(root, "runtime", "renderer")
	data, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "first" {
		t.Fatalf("artifact = %q", data)
	}
	info, err := os.Stat(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
}

func TestPruneRuntimeDirectoriesRemovesUnusedCopies(t *testing.T) {
	base := t.TempDir()
	locked, err := newRuntimeLock(filepath.Join(base, "bifrost-runtime-locked"))
	if err != nil {
		t.Fatal(err)
	}
	defer locked.release()
	current := filepath.Join(base, "bifrost-runtime-current")
	unused := filepath.Join(base, "bifrost-runtime-unused")
	recent := filepath.Join(base, "bifrost-runtime-recent")
	unrelated := filepath.Join(base, "unrelated")
	for _, root := range []string{current, unused, recent, unrelated} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-runtimeRetention - time.Hour)
	if err := os.Chtimes(unused, old, old); err != nil {
		t.Fatal(err)
	}
	pruneRuntimeDirectories(base, current)
	for _, kept := range []string{filepath.Join(base, "bifrost-runtime-locked"), current, recent, unrelated} {
		if _, err := os.Stat(kept); err != nil {
			t.Fatalf("%s was removed: %v", kept, err)
		}
	}
	if _, err := os.Stat(unused); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("unused copy survived: %v", err)
	}
}

func TestRemoveRuntimeDirectoryKeepsLockedRuntime(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bifrost-runtime-locked")
	lock, err := newRuntimeLock(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := removeRuntimeDirectory(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("locked runtime was removed: %v", err)
	}
	lock.release()
	if err := removeRuntimeDirectory(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("runtime directory survived removal: %v", err)
	}
}

func TestRuntimeBaseUsesUserCache(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	if got, want := runtimeBase(), filepath.Join(cache, "bifrost"); got != want {
		t.Fatalf("runtimeBase = %q, want %q", got, want)
	}
}

func TestRuntimeBaseFallsBackToTempDir(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")
	if got, want := runtimeBase(), os.TempDir(); got != want {
		t.Fatalf("runtimeBase = %q, want %q", got, want)
	}
}

func TestExtractVerifiedReaderRejectsWrongHash(t *testing.T) {
	root := t.TempDir()
	ref := protocol.FileRef{Path: "runtime/renderer", Hash: digestString("other"), Size: 5}
	err := extractVerifiedReader(root, ref, strings.NewReader("hello"), 0o700)
	if err == nil {
		t.Fatal("wrong hash accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "runtime", "renderer")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("artifact survived: %v", err)
	}
}

func TestPublishRuntimeScript(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "runtime"), 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "runtime", "bifrost-runtime.ts")
	if err := os.WriteFile(source, []byte("console.log('renderer')"), 0o700); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), ".bee", "blue-renderer.ts")
	ref := protocol.FileRef{Path: "runtime/bifrost-runtime.ts"}
	if err := publishRuntimeScript(root, ref, destination); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "console.log('renderer')" {
		t.Fatalf("script = %q", data)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
}
