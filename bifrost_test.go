package bifrost

import (
	"embed"
	"net/http"
	"testing"
)

var _ Router = (*http.ServeMux)(nil)

var _ interface {
	Handle(...Route) error
	Wrap(Router) http.Handler
	Handler() http.Handler
	Stop() error
	ExportStaticPages(string) error
} = (*App)(nil)

func TestNewReturnsPublicApp(t *testing.T) {
	t.Setenv("BIFROST_EXPORT", "1")

	app, err := New(embed.FS{})
	if err != nil {
		t.Fatal(err)
	}
	if app == nil || app.inner == nil {
		t.Fatal("New returned an invalid App")
	}
	if err := app.Stop(); err != nil {
		t.Fatal(err)
	}
}
