package bifrost

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/3-lines-studio/bifrost/internal/protocol"
)

type aboutData struct {
	Title string
	Lines []string
}

const aboutTemplates = "app/about/page.html"

func htmlManifest(t *testing.T, app *App) (fstest.MapFS, protocol.Manifest) {
	t.Helper()
	files := fstest.MapFS{}
	styles := addFixtureFile(files, "dist/layout.css", "body{}")
	scripts := addFixtureFile(files, "dist/about.js", "console.log(1)")
	views := make([]protocol.BuiltView, len(app.routes))
	built := make([]protocol.BuiltRoute, len(app.routes))
	for index, route := range app.routes {
		viewID := digestString(route.view)
		views[index] = protocol.BuiltView{
			ID:     viewID,
			Source: route.view,
			Mode:   "html",
			Client: protocol.AssetSet{Entry: scripts, Styles: []protocol.FileRef{styles}},
		}
		built[index] = protocol.BuiltRoute{Pattern: route.pattern, Kind: "html", ViewID: viewID}
	}
	return files, protocol.Manifest{
		Schema:      protocol.Schema,
		SpecHash:    app.specHash,
		BuildID:     digestString("build"),
		Toolchain:   protocol.Toolchain{Bifrost: "1.4.0", Bun: "1-test", Vite: "8.2.1", React: "19.2.0"},
		ClientFiles: []protocol.FileRef{styles, scripts},
		Views:       views,
		Routes:      built,
	}
}

func htmlHandler(t *testing.T, app *App, templates fstest.MapFS) *runtimeState {
	t.Helper()
	app.templates = templates
	files, manifest := htmlManifest(t, app)
	compiled, err := validateManifest(files, app.spec, app.specHash, manifest)
	if err != nil {
		t.Fatal(err)
	}
	state, err := compileRuntime(app, files, compiled, nil)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func newHTMLApp(t *testing.T, templates fstest.MapFS, route Route) *App {
	t.Helper()
	app, err := newApp(Config{
		SourceRoot: t.TempDir(),
		Templates:  templates,
		Routes:     []Route{route},
	})
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func TestHTMLPageWrapsLayoutsAndLoads(t *testing.T) {
	templates := fstest.MapFS{
		"app/layout.html": {Data: []byte(`<html><body data-path="{{.Path}}"><nav>chrome</nav>{{.Children}}<footer>foot</footer></body></html>`)},
		"app/shell.html":  {Data: []byte(`<main>{{.Children}}</main>`)},
		aboutTemplates:    {Data: []byte(`<h1>{{.Title}}</h1>{{range .Lines}}<p>{{.}}</p>{{end}}`)},
	}
	page := HTMLPage{
		Page:    aboutTemplates,
		Layouts: []string{"app/layout.html", "app/shell.html"},
		Load: func(*http.Request) (any, error) {
			return aboutData{Title: "Sobre <picsel>", Lines: []string{"uno", "dos"}}, nil
		},
	}
	app := newHTMLApp(t, templates, HTML("/about", page))
	state := htmlHandler(t, app, templates)

	recorder := httptest.NewRecorder()
	state.handlers["/about"].ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/about", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, want := range []string{
		"<!doctype html>",
		"<nav>chrome</nav>",
		"<main>",
		"<h1>Sobre &lt;picsel&gt;</h1>",
		"<p>uno</p><p>dos</p>",
		"<footer>foot</footer>",
		`data-path="/about"`,
		`<link rel="stylesheet" href="/_bifrost/dist/layout.css">`,
		`<script type="module" src="/_bifrost/dist/about.js"></script>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "BIFROST_PROPS") {
		t.Fatal("an HTML page carries no props payload")
	}
	if got, want := recorder.Header().Get("Content-Type"), "text/html; charset=utf-8"; got != want {
		t.Fatalf("content type = %q, want %q", got, want)
	}
	if got, want := recorder.Header().Get("Cache-Control"), defaultDocumentCacheControl; got != want {
		t.Fatalf("cache control = %q, want %q", got, want)
	}
}

func TestHTMLPageLinksLayoutAssetsFirst(t *testing.T) {
	templates := fstest.MapFS{aboutTemplates: {Data: []byte(`<p>page</p>`)}}
	app := newHTMLApp(t, templates, HTML("/about", HTMLPage{Page: aboutTemplates}))
	app.templates = templates
	files, manifest := htmlManifest(t, app)
	layoutStyle := addFixtureFile(files, "dist/chrome.css", "nav{}")
	layoutScript := addFixtureFile(files, "dist/chrome.js", "theme()")
	manifest.Views[0].Layout = &protocol.AssetSet{Entry: layoutScript, Styles: []protocol.FileRef{layoutStyle}}
	manifest.ClientFiles = append(manifest.ClientFiles, layoutStyle, layoutScript)

	compiled, err := validateManifest(files, app.spec, app.specHash, manifest)
	if err != nil {
		t.Fatal(err)
	}
	state, err := compileRuntime(app, files, compiled, nil)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	state.handlers["/about"].ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/about", nil))

	body := recorder.Body.String()
	chromeStyle := strings.Index(body, "chrome.css")
	pageStyle := strings.Index(body, "layout.css")
	chromeScript := strings.Index(body, "chrome.js")
	if chromeStyle < 0 || pageStyle < 0 || chromeScript < 0 {
		t.Fatalf("layout assets missing:\n%s", body)
	}
	if chromeStyle > pageStyle {
		t.Fatalf("the chrome must come first:\n%s", body)
	}
	if !strings.Contains(body, `<script type="module" src="/_bifrost/dist/chrome.js"></script>`) {
		t.Fatalf("layout script missing:\n%s", body)
	}
}

func TestManifestRejectsLayoutAssetsOutsideHTML(t *testing.T) {
	files, app, manifest := validManifestFixture(t)
	manifest.Views[0].Layout = &protocol.AssetSet{Styles: manifest.Views[0].Client.Styles}
	if _, err := validateManifest(files, app.spec, app.specHash, manifest); err == nil {
		t.Fatal("a server view accepted a layout asset set")
	}
}

func TestHTMLPageMetadataIsEscaped(t *testing.T) {
	templates := fstest.MapFS{aboutTemplates: {Data: []byte(`<p>page</p>`)}}
	page := HTMLPage{
		Page: aboutTemplates,
		Metadata: func(*http.Request) (Metadata, error) {
			return Metadata{
				Title:       `About "picsel" <app>`,
				Description: "one & two",
				Canonical:   "https://picsel.app/about",
				Robots:      "index, follow",
				Type:        "website",
				Head:        `<script type="application/ld+json">{"a":1}</script>`,
			}, nil
		},
	}
	app := newHTMLApp(t, templates, HTML("/about", page))
	state := htmlHandler(t, app, templates)

	recorder := httptest.NewRecorder()
	state.handlers["/about"].ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/about", nil))
	body := recorder.Body.String()

	for _, want := range []string{
		`<title>About &#34;picsel&#34; &lt;app&gt;</title>`,
		`<meta name="description" content="one &amp; two">`,
		`<link rel="canonical" href="https://picsel.app/about">`,
		`<meta property="og:type" content="website">`,
		`<script type="application/ld+json">{"a":1}</script>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("head lacks %q:\n%s", want, body)
		}
	}
}

func TestHTMLPageReadsParamsAndCache(t *testing.T) {
	templates := fstest.MapFS{
		"app/user/user.html":   {Data: []byte(`<p>{{.}}</p>`)},
		"app/user/layout.html": {Data: []byte(`<p>{{index .Params "username"}}</p>{{.Children}}`)},
	}
	page := HTMLPage{
		Page:    "app/user/user.html",
		Layouts: []string{"app/user/layout.html"},
		Params:  []string{"username"},
		Load:    func(*http.Request) (any, error) { return "cuerpo", nil },
	}
	app := newHTMLApp(t, templates, HTML("/user/{username}", page).WithCache("public, max-age=60"))
	state := htmlHandler(t, app, templates)

	request := httptest.NewRequest(http.MethodGet, "/user/ana", nil)
	request.SetPathValue("username", "ana")
	recorder := httptest.NewRecorder()
	state.handlers["/user/{username}"].ServeHTTP(recorder, request)

	body := recorder.Body.String()
	if !strings.Contains(body, "<p>ana</p>") || !strings.Contains(body, "<p>cuerpo</p>") {
		t.Fatalf("params or body missing:\n%s", body)
	}
	if got, want := recorder.Header().Get("Cache-Control"), "public, max-age=60"; got != want {
		t.Fatalf("cache control = %q, want %q", got, want)
	}
}

func TestHTMLPageStatusAndMissingTemplate(t *testing.T) {
	templates := fstest.MapFS{aboutTemplates: {Data: []byte(`<p>no encontrado</p>`)}}
	app := newHTMLApp(t, templates, HTML("/about", HTMLPage{
		Page: aboutTemplates,
		Load: func(*http.Request) (any, error) {
			return PageData{Props: aboutData{Title: "x"}, Document: Document{Lang: "es"}, Status: http.StatusNotFound}, nil
		},
	}))
	state := htmlHandler(t, app, templates)

	recorder := httptest.NewRecorder()
	state.handlers["/about"].ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/about", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `<html lang="es">`) {
		t.Fatalf("document language missing:\n%s", recorder.Body.String())
	}

	handler := &htmlPageHandler{pattern: "/about", page: HTMLPage{Page: "app/about/missing.html"}, templates: templates}
	if _, err := handler.render(httptest.NewRequest(http.MethodGet, "/about", nil), nil, nil); err == nil {
		t.Fatal("a missing template must fail")
	}
}

func TestHTMLPageNeedsTemplates(t *testing.T) {
	app, err := newApp(Config{
		SourceRoot: t.TempDir(),
		Routes:     []Route{HTML("/about", HTMLPage{Page: "app/about/page.html"})},
	})
	if err != nil {
		t.Fatal(err)
	}
	files, manifest := htmlManifest(t, app)
	compiled, err := validateManifest(files, app.spec, app.specHash, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compileRuntime(app, files, compiled, nil); err == nil {
		t.Fatal("an HTML route without Config.Templates must fail at boot")
	}
}

func TestHTMLPageRejectsScriptsForServerRoutes(t *testing.T) {
	_, err := newApp(Config{
		SourceRoot: t.TempDir(),
		Routes:     []Route{HTML("/about", HTMLPage{Page: "app/about/page.tsx"})},
	})
	if err == nil {
		t.Fatal("an HTML route must not accept a React view")
	}
	if !strings.Contains(err.Error(), ".html") {
		t.Fatalf("error = %v, want a .html hint", err)
	}
}

func TestHTMLPageLoadErrorIsServerError(t *testing.T) {
	templates := fstest.MapFS{aboutTemplates: {Data: []byte(`<p>x</p>`)}}
	app := newHTMLApp(t, templates, HTML("/about", HTMLPage{
		Page: aboutTemplates,
		Load: func(*http.Request) (any, error) { return nil, errors.New("boom") },
	}))
	state := htmlHandler(t, app, templates)
	recorder := httptest.NewRecorder()
	state.handlers["/about"].ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/about", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
}

func markdownHTMLFixture(t *testing.T) *App {
	t.Helper()
	templates := fstest.MapFS{
		"app/layout.html": {Data: []byte(`<html><body><nav><a href="/">picsel</a></nav>{{.Children}}<footer>hecho en argentina</footer></body></html>`)},
		aboutTemplates:    {Data: []byte(`<h1>{{.Title}}</h1>{{range .Lines}}<p>{{.}}</p>{{end}}`)},
	}
	page := HTMLPage{
		Page:    aboutTemplates,
		Layouts: []string{"app/layout.html"},
		Load: func(*http.Request) (any, error) {
			return aboutData{Title: "Sobre picsel", Lines: []string{"Fotos de eventos"}}, nil
		},
	}
	app := newHTMLApp(t, templates, HTML("/about", page))
	app.runtime = htmlHandler(t, app, templates)
	return app
}

func TestHTMLPageAnswersMarkdown(t *testing.T) {
	app := markdownHTMLFixture(t)

	request := httptest.NewRequest(http.MethodGet, "/about", nil)
	request.Header.Set("Accept", "text/markdown")
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)

	if got, want := recorder.Header().Get("Content-Type"), "text/markdown; charset=utf-8"; got != want {
		t.Fatalf("content type = %q, want %q", got, want)
	}
	body := recorder.Body.String()
	for _, want := range []string{"# Sobre picsel", "Fotos de eventos", "[picsel](/)", "hecho en argentina"} {
		if !strings.Contains(body, want) {
			t.Fatalf("markdown lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "<h1>") || strings.Contains(body, "<footer>") {
		t.Fatalf("markdown still carries markup:\n%s", body)
	}
}

func TestHTMLPageAnswersMarkdownSuffix(t *testing.T) {
	app := markdownHTMLFixture(t)

	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/about.md", nil))

	if got, want := recorder.Header().Get("Content-Type"), "text/markdown; charset=utf-8"; got != want {
		t.Fatalf("content type = %q, want %q", got, want)
	}
	if !strings.Contains(recorder.Body.String(), "# Sobre picsel") {
		t.Fatalf("body is not markdown:\n%s", recorder.Body.String())
	}
}

func TestHTMLPageMarkdownKeepsStatus(t *testing.T) {
	templates := fstest.MapFS{aboutTemplates: {Data: []byte(`<h1>{{.Title}}</h1>`)}}
	app := newHTMLApp(t, templates, HTML("/about", HTMLPage{
		Page: aboutTemplates,
		Load: func(*http.Request) (any, error) {
			return PageData{Status: http.StatusNotFound, Props: aboutData{Title: "No está"}}, nil
		},
	}))
	app.runtime = htmlHandler(t, app, templates)

	request := httptest.NewRequest(http.MethodGet, "/about", nil)
	request.Header.Set("Accept", "text/markdown")
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "# No está") {
		t.Fatalf("body is not markdown:\n%s", recorder.Body.String())
	}
}

func TestHTMLPageServesHTMLWithoutMarkdownRequest(t *testing.T) {
	app := markdownHTMLFixture(t)

	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/about", nil))

	if got, want := recorder.Header().Get("Content-Type"), "text/html; charset=utf-8"; got != want {
		t.Fatalf("content type = %q, want %q", got, want)
	}
	if !strings.Contains(recorder.Body.String(), "<!doctype html>") {
		t.Fatalf("body is not a document:\n%s", recorder.Body.String())
	}
}

func TestHTMLPageVariesByAccept(t *testing.T) {
	templates := fstest.MapFS{aboutTemplates: {Data: []byte(`<p>page</p>`)}}
	app := newHTMLApp(t, templates, HTML("/about", HTMLPage{Page: aboutTemplates}))
	state := htmlHandler(t, app, templates)

	recorder := httptest.NewRecorder()
	state.handlers["/about"].ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/about", nil))

	if got := recorder.Header().Values("Vary"); len(got) != 1 || got[0] != "Accept" {
		t.Fatalf("vary = %v, want [Accept]", got)
	}
}
