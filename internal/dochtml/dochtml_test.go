package dochtml

import (
	"bytes"
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/protocol"
)

func TestScriptShellEmitsPropsAndEntry(t *testing.T) {
	shell, err := NewScriptShell(protocol.AssetSet{
		Entry:  protocol.FileRef{Path: "dist/page.js"},
		Styles: []protocol.FileRef{{Path: "dist/page.css"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := shell.WritePreamble(&output, []byte("<title>page</title>"), protocol.DocumentAttributes{Lang: "en"}); err != nil {
		t.Fatal(err)
	}
	if err := shell.WriteSuffix(&output, []byte(`{"title":"page"}`)); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{
		`<link rel="stylesheet" href="/_bifrost/dist/page.css">`,
		`<title>page</title>`,
		`<script id="__BIFROST_PROPS__" type="application/json">{"title":"page"}</script>`,
		`<script type="module" src="/_bifrost/dist/page.js"></script>`,
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("document does not contain %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, `id="app"`) {
		t.Fatalf("script shell wrapped the body:\n%s", text)
	}
}

func TestScriptShellRequiresEntry(t *testing.T) {
	if _, err := NewScriptShell(protocol.AssetSet{}); err == nil {
		t.Fatal("empty client entry was accepted")
	}
}

func TestPlainShellHasNoScripts(t *testing.T) {
	shell := NewPlainShell()
	var output bytes.Buffer
	if err := shell.WritePreamble(&output, nil, protocol.DocumentAttributes{Lang: "en"}); err != nil {
		t.Fatal(err)
	}
	if err := shell.WriteSuffix(&output, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "<script") {
		t.Fatalf("plain shell emitted a script:\n%s", output.String())
	}
}

func TestPlainShellWithDevReload(t *testing.T) {
	shell := NewPlainShell().WithDevReload("build-id")
	var output bytes.Buffer
	if err := shell.WritePreamble(&output, nil, protocol.DocumentAttributes{Lang: "en"}); err != nil {
		t.Fatal(err)
	}
	if err := shell.WriteSuffix(&output, nil); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, `const build="build-id"`) || !strings.Contains(text, `/_bifrost/build-id?wait=`) {
		t.Fatalf("dev reload script missing: %s", text)
	}
	empty := NewPlainShell().WithDevReload("")
	if strings.Contains(empty.suffix, "build-id") {
		t.Fatal("empty build id emitted a reload script")
	}
}
