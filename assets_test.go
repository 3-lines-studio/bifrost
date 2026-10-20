package bifrost

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/3-lines-studio/bifrost/internal/protocol"
)

func TestHashedPublicURL(t *testing.T) {
	cases := []struct {
		url  string
		hash string
		want string
	}{
		{"/styles.css", "abcdef0123456789", "/styles.abcdef012345.css"},
		{"/img/logo.svg", "abc", "/img/logo.abc.svg"},
		{"/file", "abc", "/file.abc"},
	}
	for _, item := range cases {
		if got := hashedPublicURL(item.url, protocol.FileRef{Hash: item.hash}); got != item.want {
			t.Fatalf("hashedPublicURL(%q) = %q, want %q", item.url, got, item.want)
		}
	}
}

func TestPublicAssetIndexAddsHashedURLs(t *testing.T) {
	file := protocol.FileRef{Path: "public/styles.css", Hash: "abcdef0123456789", Size: 10}
	public, hashed, lookup := publicAssetIndex(map[string]protocol.FileRef{"/styles.css": file})
	if _, exists := public["/styles.css"]; !exists {
		t.Fatal("original public URL missing")
	}
	if public["/styles.abcdef012345.css"] != file {
		t.Fatal("hashed public URL missing")
	}
	if _, exists := hashed["/styles.abcdef012345.css"]; !exists {
		t.Fatal("hashed set missing")
	}
	if lookup["/styles.css"] != "/styles.abcdef012345.css" || lookup["styles.css"] != "/styles.abcdef012345.css" {
		t.Fatalf("lookup = %+v", lookup)
	}
}

func TestPublicAssetFuncRejectsMissingAsset(t *testing.T) {
	asset := publicAssetFunc(map[string]string{})
	if _, err := asset("styles.css"); err == nil {
		t.Fatal("expected an error for a missing asset")
	}
}

func TestTemplateRendererAssetFunc(t *testing.T) {
	renderer, err := newTemplateRenderer(&protocol.HTMLAssets{
		Page: protocol.TemplateSource{
			Path:   "app/page.html",
			Source: `<link rel="stylesheet" href="{{asset "/styles.css"}}">`,
		},
	}, template.FuncMap{"asset": func(string) (string, error) { return "/styles.abcdef012345.css", nil }})
	if err != nil {
		t.Fatal(err)
	}
	sink := &templateTestSink{}
	if err := renderer.Render(context.Background(), renderRequest{}, sink); err != nil {
		t.Fatal(err)
	}
	if got, want := sink.body.String(), `<link rel="stylesheet" href="/styles.abcdef012345.css">`; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestPublicAssetHandlerServesHashedURLsImmutably(t *testing.T) {
	data := []byte("body{}")
	handler := &publicAssetHandler{
		assets: fstest.MapFS{"public/styles.css": {Data: data}},
		file: protocol.FileRef{
			Path: "public/styles.css",
			Hash: "abcdef0123456789",
			Size: int64(len(data)),
		},
		immutable: true,
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/styles.abcdef012345.css", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if got, want := response.Header().Get("Cache-Control"), "public, max-age=31536000, immutable"; got != want {
		t.Fatalf("cache-control = %q, want %q", got, want)
	}
}
