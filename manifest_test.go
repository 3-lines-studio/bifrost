package bifrost

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"testing/fstest"

	"github.com/3-lines-studio/bifrost/internal/protocol"
)

func digestString(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func addFixtureFile(files fstest.MapFS, name, content string) protocol.FileRef {
	data := []byte(content)
	files[name] = &fstest.MapFile{Data: data}
	return protocol.FileRef{Path: name, Hash: digestString(content), Size: int64(len(data))}
}

func validManifestFixture(t *testing.T) (fstest.MapFS, *App, protocol.Manifest) {
	t.Helper()
	app, err := newApp(Config{
		SourceRoot: t.TempDir(),
		Routes: []Route{
			Server("/server", "pages/server.tsx", nil),
			Static("/static", "pages/static.tsx", nil),
			Client("/client", "pages/client.tsx"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	files := fstest.MapFS{}
	serverClient := addFixtureFile(files, "dist/server.js", "server client")
	serverBundle := addFixtureFile(files, "ssr/server.js", "server bundle")
	staticClient := addFixtureFile(files, "dist/static.js", "static client")
	staticBundle := addFixtureFile(files, "ssr/static.js", "static bundle")
	clientEntry := addFixtureFile(files, "dist/client.js", "client mount")
	staticHTML := addFixtureFile(files, "pages/static.html", "<html>static</html>")
	runtime := addFixtureFile(files, "runtime/bifrost-renderer", "runtime")

	serverViewID := digestString("server-view")
	staticViewID := digestString("static-view")
	clientViewID := digestString("client-view")
	manifest := protocol.Manifest{
		Schema:   protocol.Schema,
		SpecHash: app.specHash,
		BuildID:  digestString("build"),
		Toolchain: protocol.Toolchain{
			Bifrost: "1.1.0",
			Bun:     "1-test",
			Vite:    "8.2.1",
			React:   "19.2.0",
		},
		Runtime:     &runtime,
		ClientFiles: []protocol.FileRef{serverClient, staticClient, clientEntry},
		Views: []protocol.BuiltView{
			{ID: serverViewID, Source: "pages/server.tsx", Mode: "hydrate", Client: protocol.AssetSet{Entry: serverClient}, Server: &protocol.ServerAssets{Entry: serverBundle}},
			{ID: staticViewID, Source: "pages/static.tsx", Mode: "hydrate", Client: protocol.AssetSet{Entry: staticClient}, Server: &protocol.ServerAssets{Entry: staticBundle}},
			{ID: clientViewID, Source: "pages/client.tsx", Mode: "mount", Client: protocol.AssetSet{Entry: clientEntry}},
		},
		Routes: []protocol.BuiltRoute{
			{Pattern: "/server", Kind: "server", ViewID: serverViewID},
			{Pattern: "/static", Kind: "static", ViewID: staticViewID, Documents: []protocol.Document{{Path: "/static", File: staticHTML, Document: protocol.DocumentAttributes{Lang: "en"}}}},
			{Pattern: "/client", Kind: "client", ViewID: clientViewID},
		},
	}
	return files, app, manifest
}

func TestParseAndValidateManifest(t *testing.T) {
	files, app, manifest := validManifestFixture(t)
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := validateManifest(files, app.spec, app.specHash, parsed)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(compiled.routes), 3; got != want {
		t.Fatalf("compiled routes = %d, want %d", got, want)
	}
	if got, want := len(compiled.views), 3; got != want {
		t.Fatalf("compiled views = %d, want %d", got, want)
	}
}

func TestParseManifestIsStrict(t *testing.T) {
	for _, data := range []string{
		`{"schema":1,"unknown":true}`,
		`{"schema":1}{"schema":1}`,
	} {
		if _, err := parseManifest([]byte(data)); err == nil {
			t.Fatalf("parseManifest(%q) succeeded", data)
		}
	}
}

func TestValidateManifestAcceptsLiveViteDevelopmentManifest(t *testing.T) {
	t.Setenv("BIFROST_DEV_DIR", t.TempDir())
	files, app, manifest := validManifestFixture(t)
	manifest.Runtime = nil
	manifest.Views[0].Server = nil
	manifest.Views[1].Server = nil
	if _, err := validateManifest(files, app.spec, app.specHash, manifest); err != nil {
		t.Fatal(err)
	}
}

func TestValidateManifestAcceptsPrerenderedStaticViewWithoutServerBundle(t *testing.T) {
	files, app, manifest := validManifestFixture(t)
	manifest.Views[1].Server = nil
	if _, err := validateManifest(files, app.spec, app.specHash, manifest); err != nil {
		t.Fatal(err)
	}
}

func TestValidateManifestRejectsInvalidState(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(fstest.MapFS, *App, *protocol.Manifest)
	}{
		{name: "schema", mutate: func(_ fstest.MapFS, _ *App, manifest *protocol.Manifest) { manifest.Schema++ }},
		{name: "stale", mutate: func(_ fstest.MapFS, _ *App, manifest *protocol.Manifest) { manifest.SpecHash = digestString("stale") }},
		{name: "toolchain", mutate: func(_ fstest.MapFS, _ *App, manifest *protocol.Manifest) { manifest.Toolchain.Bun = "" }},
		{name: "vite version", mutate: func(_ fstest.MapFS, _ *App, manifest *protocol.Manifest) { manifest.Toolchain.Vite = "7.0.0" }},
		{name: "missing runtime", mutate: func(_ fstest.MapFS, _ *App, manifest *protocol.Manifest) { manifest.Runtime = nil }},
		{name: "unregistered client file", mutate: func(_ fstest.MapFS, _ *App, manifest *protocol.Manifest) { manifest.ClientFiles = nil }},
		{name: "duplicate view", mutate: func(_ fstest.MapFS, _ *App, manifest *protocol.Manifest) {
			manifest.Views = append(manifest.Views, manifest.Views[0])
		}},
		{name: "bad view ID", mutate: func(_ fstest.MapFS, _ *App, manifest *protocol.Manifest) { manifest.Views[0].ID = "bad" }},
		{name: "bad view mode", mutate: func(_ fstest.MapFS, _ *App, manifest *protocol.Manifest) { manifest.Views[0].Mode = "other" }},
		{name: "unknown route", mutate: func(_ fstest.MapFS, _ *App, manifest *protocol.Manifest) { manifest.Routes[0].Pattern = "/unknown" }},
		{name: "duplicate route", mutate: func(_ fstest.MapFS, _ *App, manifest *protocol.Manifest) {
			manifest.Routes = append(manifest.Routes, manifest.Routes[0])
		}},
		{name: "wrong kind", mutate: func(_ fstest.MapFS, _ *App, manifest *protocol.Manifest) { manifest.Routes[0].Kind = "client" }},
		{name: "unknown view", mutate: func(_ fstest.MapFS, _ *App, manifest *protocol.Manifest) {
			manifest.Routes[0].ViewID = digestString("missing")
		}},
		{name: "wrong source", mutate: func(_ fstest.MapFS, _ *App, manifest *protocol.Manifest) {
			manifest.Views[0].Source = "pages/other.tsx"
		}},
		{name: "server without bundle", mutate: func(_ fstest.MapFS, _ *App, manifest *protocol.Manifest) { manifest.Views[0].Server = nil }},
		{name: "client with bundle", mutate: func(_ fstest.MapFS, _ *App, manifest *protocol.Manifest) {
			bundle := manifest.Views[0].Client.Entry
			manifest.Views[2].Server = &protocol.ServerAssets{Entry: bundle}
		}},
		{name: "document on server", mutate: func(_ fstest.MapFS, _ *App, manifest *protocol.Manifest) {
			manifest.Routes[0].Documents = manifest.Routes[1].Documents
		}},
		{name: "missing exact document", mutate: func(_ fstest.MapFS, _ *App, manifest *protocol.Manifest) { manifest.Routes[1].Documents = nil }},
		{name: "wrong document path", mutate: func(_ fstest.MapFS, _ *App, manifest *protocol.Manifest) {
			manifest.Routes[1].Documents[0].Path = "/other"
		}},
		{name: "invalid document attributes", mutate: func(_ fstest.MapFS, _ *App, manifest *protocol.Manifest) {
			manifest.Routes[1].Documents[0].Document.Lang = "../en"
		}},
		{name: "escaping artifact", mutate: func(_ fstest.MapFS, _ *App, manifest *protocol.Manifest) {
			manifest.Views[0].Client.Entry.Path = "../bad.js"
		}},
		{name: "missing artifact", mutate: func(files fstest.MapFS, _ *App, _ *protocol.Manifest) { delete(files, "dist/server.js") }},
		{name: "corrupt artifact", mutate: func(files fstest.MapFS, _ *App, _ *protocol.Manifest) {
			files["dist/server.js"].Data = []byte("changed")
		}},
		{name: "missing route", mutate: func(_ fstest.MapFS, _ *App, manifest *protocol.Manifest) { manifest.Routes = manifest.Routes[:2] }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files, app, manifest := validManifestFixture(t)
			test.mutate(files, app, &manifest)
			if _, err := validateManifest(files, app.spec, app.specHash, manifest); err == nil {
				t.Fatal("validateManifest succeeded")
			}
		})
	}
}

func TestValidatePublicURLRejectsServeMuxMetacharacters(t *testing.T) {
	for _, value := range []string{"/page{x}.txt", "/a{b}"} {
		if err := validatePublicURL(value); err == nil {
			t.Fatalf("validatePublicURL(%q) succeeded", value)
		}
	}
}

func TestPathMatchesPattern(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{pattern: "/blog/{slug}", path: "/blog/first", want: true},
		{pattern: "/blog/{slug}", path: "/other/first", want: false},
		{pattern: "/docs/{rest...}", path: "/docs/a/b", want: true},
		{pattern: "/{$}", path: "/", want: true},
	}
	for _, test := range tests {
		if got := pathMatchesPattern(test.pattern, test.path); got != test.want {
			t.Errorf("pathMatchesPattern(%q, %q) = %v, want %v", test.pattern, test.path, got, test.want)
		}
	}
}
