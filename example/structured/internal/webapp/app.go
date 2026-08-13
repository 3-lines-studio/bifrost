package webapp

import (
	"context"
	"io/fs"
	"net/http"

	"github.com/3-lines-studio/bifrost"
)

// New declares the web application without opening listeners or external
// services. Runtime startup happens in main after bifrost.Building returns false.
func New(assets fs.FS) (*bifrost.App, error) {
	return bifrost.New(bifrost.Config{
		SourceRoot: ".",
		Assets:     assets,
		Routes: []bifrost.Route{
			bifrost.Server("/{$}", "pages/home.tsx", func(*http.Request) (any, error) {
				return bifrost.PageData{Props: map[string]string{"name": "Home"}, Document: bifrost.Document{Lang: "en"}}, nil
			}),
			bifrost.Server("/hello/{name}", "pages/home.tsx", func(request *http.Request) (any, error) {
				return bifrost.PageData{Props: map[string]string{"name": request.PathValue("name")}, Document: bifrost.Document{Lang: "es", Dir: "ltr"}}, nil
			}),
		},
	})
}

// Handler composes Bifrost with ordinary net/http handlers and applies one
// middleware chain to both.
func Handler(app *bifrost.App) (http.Handler, error) {
	mux := http.NewServeMux()
	if err := app.Register(mux); err != nil {
		return nil, err
	}
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		http.NotFound(w, request)
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("X-Structured-Example", "active")
		mux.ServeHTTP(w, request)
	}), nil
}

func Close(app *bifrost.App) {
	_ = app.Close(context.Background())
}
