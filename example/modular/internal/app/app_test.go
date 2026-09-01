package app

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"testing"

	"github.com/3-lines-studio/bifrost/example/modular/internal/billing"
	"github.com/3-lines-studio/bifrost/example/modular/internal/config"
	"github.com/3-lines-studio/bifrost/example/modular/internal/db"
	"github.com/3-lines-studio/bifrost/example/modular/internal/i18n"
	"github.com/3-lines-studio/bifrost/example/modular/internal/mailer"
	"github.com/3-lines-studio/bifrost/example/modular/internal/notify"
	"github.com/3-lines-studio/bifrost/example/modular/internal/queue"
	"github.com/3-lines-studio/bifrost/example/modular/internal/storage"
	"github.com/3-lines-studio/bifrost/example/modular/internal/user"
)

// Compile-time check that every domain module satisfies the app surface.
var (
	_ Module = (*user.Module)(nil)
	_ Module = (*billing.Module)(nil)
	_ Module = (*notify.Module)(nil)
)

func TestComposition(t *testing.T) {
	cfg := config.New()
	wireConfig(t, cfg)

	database := db.New()
	taskQueue := queue.New()
	files := storage.New()
	sender := mailer.New()
	catalog := i18n.New()
	userModule := user.New()
	billingModule := billing.New()
	notifyModule := notify.New()

	database.Wire(context.Background(), cfg)
	taskQueue.Wire(cfg)
	files.Wire(cfg)
	sender.Wire(cfg)
	catalog.Wire(cfg)
	userModule.Wire(database, catalog)
	billingModule.Wire(database, taskQueue, files, catalog, userModule)
	notifyModule.Wire(taskQueue, sender, catalog)

	if len(userModule.Pages())+len(billingModule.Pages()) == 0 {
		t.Fatal("expected pages from domain modules")
	}

	// Build phase: the app composes muxes and declares pages without requiring a
	// production manifest. The describe phase writes its spec to BIFROST_FD.
	phaseFile, err := os.CreateTemp(t.TempDir(), "bifrost-spec-*")
	if err != nil {
		t.Fatalf("create temp spec: %v", err)
	}
	t.Cleanup(func() { _ = phaseFile.Close() })
	t.Setenv("BIFROST_PHASE", "describe")
	t.Setenv("BIFROST_FD", strconv.Itoa(int(phaseFile.Fd())))

	web, err := New(cfg, nil, nil, userModule, billingModule, notifyModule)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// The domain REST routes are mounted on the HTTP mux. Assert the patterns are
	// registered without executing them (their handlers touch Postgres).
	mux, ok := web.Handler().(*http.ServeMux)
	if !ok {
		t.Fatalf("handler is %T, want *http.ServeMux", web.Handler())
	}
	assertMuxPattern(t, mux, "GET /api/users/{id}")
	assertMuxPattern(t, mux, "GET /api/users")
	assertMuxPattern(t, mux, "GET /api/invoices/{id}")
	assertMuxPattern(t, mux, "POST /api/invoices")
	assertMuxPattern(t, mux, "POST /api/notify")
}

// assertMuxPattern checks that the exact pattern is registered by issuing a
// request its path would reach and comparing the matched pattern returned by
// ServeMux.Handler.
func assertMuxPattern(t *testing.T, mux *http.ServeMux, pattern string) {
	t.Helper()
	method, path := splitPattern(pattern)
	req, _ := http.NewRequest(method, path, nil)
	_, matched := mux.Handler(req)
	if matched != pattern {
		t.Errorf("pattern %q not matched; got %q", pattern, matched)
	}
}

func splitPattern(pattern string) (method, path string) {
	method = "GET"
	if i := indexOfSpace(pattern); i > 0 {
		method = pattern[:i]
		path = pattern[i+1:]
	} else {
		path = pattern
	}
	if path == "/api/users/{id}" || path == "/api/invoices/{id}" {
		path = path[:len(path)-3] + "1" // replace {id}
	}
	return method, path
}

func indexOfSpace(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' {
			return i
		}
	}
	return -1
}

// wireConfig sets the env vars the config loader needs, then loads config.
func wireConfig(t *testing.T, cfg *config.Module) {
	t.Helper()
	t.Setenv("HTTP_ADDR", ":8080")
	t.Setenv("SOURCE_ROOT", ".")
	t.Setenv("POSTGRES_URL", "postgres://user:pass@localhost:5432/db")
	t.Setenv("REDIS_ADDR", "localhost:6379")
	t.Setenv("STORAGE_BUCKET", "bucket")
	t.Setenv("STORAGE_REGION", "us-east-1")
	t.Setenv("SMTP_HOST", "localhost")
	t.Setenv("SMTP_PORT", "1025")
	t.Setenv("I18N_DEFAULT", "en")
	cfg.Wire()
}
