package bifrost

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/dochtml"
	"github.com/3-lines-studio/bifrost/internal/protocol"
)

func TestRawPropsAreValidatedCompactedAndEscaped(t *testing.T) {
	props, err := marshalProps(RawProps(` { "html": "</script>" } `))
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(props) || bytes.Contains(props, []byte("<")) || strings.Contains(string(props), " ") {
		t.Fatalf("unsafe raw props: %s", props)
	}
	if _, err := marshalProps(RawProps(`{"bad":`)); err == nil {
		t.Fatal("invalid RawProps succeeded")
	}
}

func TestPropsMustBeJSONObject(t *testing.T) {
	for _, props := range []any{
		RawProps(`[1,2,3]`),
		RawProps(`"hello"`),
		[]int{1, 2, 3},
		"hello",
	} {
		if _, err := marshalProps(props); err == nil {
			t.Fatalf("non-object props succeeded: %v", props)
		}
	}
}

func TestDocumentAttributesAreNormalizedAndEscaped(t *testing.T) {
	document, err := normalizeDocument(Document{Lang: "pt-BR", Class: " dark\tcontrast ", Dir: "rtl"})
	if err != nil {
		t.Fatal(err)
	}
	if document.Class != "dark contrast" {
		t.Fatalf("class = %q", document.Class)
	}
	opened := dochtml.Open(protocolDocument(document))
	for _, expected := range []string{`lang="pt-BR"`, `class="dark contrast"`, `dir="rtl"`} {
		if !strings.Contains(opened, expected) {
			t.Fatalf("document lacks %q: %s", expected, opened)
		}
	}
	for _, invalid := range []Document{{Lang: "../es"}, {Lang: "es", Dir: "sideways"}, {Lang: "es", Class: "bad\x00class"}} {
		if _, err := normalizeDocument(invalid); err == nil {
			t.Fatalf("invalid document succeeded: %+v", invalid)
		}
	}
}

func TestPageDataSeparatesPropsFromDocument(t *testing.T) {
	props, document, err := splitPageData(PageData{Props: map[string]string{"name": "Don"}, Document: Document{Lang: "es"}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := marshalProps(props)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"name":"Don"}` || document.Lang != "es" {
		t.Fatalf("props = %s, document = %+v", encoded, document)
	}
}

func TestDocumentGivesModulePreloadsLowPriority(t *testing.T) {
	shell, err := dochtml.NewShell(protocol.AssetSet{
		Entry:   protocol.FileRef{Path: "dist/app.js"},
		Imports: []protocol.FileRef{{Path: "dist/vendor.js"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	document, err := shell.ClientDocument(emptyProps)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(document), `rel="modulepreload" fetchpriority="low"`) != 2 {
		t.Fatalf("module preloads do not have low priority:\n%s", document)
	}
}

func TestDevelopmentDocumentIncludesViteRuntime(t *testing.T) {
	shell, err := dochtml.NewShell(protocol.AssetSet{Entry: protocol.FileRef{Path: dochtml.DevPrefix + "@fs/app.tsx"}})
	if err != nil {
		t.Fatal(err)
	}
	document, err := shell.ClientDocument(emptyProps)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"/_bifrost/dev/@vite/client", "/_bifrost/dev/@react-refresh", "/_bifrost/dev/@fs/app.tsx"} {
		if !bytes.Contains(document, []byte(expected)) {
			t.Fatalf("development document lacks %q: %s", expected, document)
		}
	}
}

func TestNormalizeLimits(t *testing.T) {
	limits, err := normalizeLimits(Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if limits.MaxPropsBytes == 0 || limits.MaxHeadBytes == 0 || limits.MaxFrameBytes == 0 || limits.MaxMarkdownBytes == 0 {
		t.Fatalf("defaults not applied: %+v", limits)
	}
	if _, err := normalizeLimits(Limits{MaxPropsBytes: -1}); err == nil {
		t.Fatal("negative limit succeeded")
	}
	if _, err := normalizeLimits(Limits{MaxMarkdownBytes: -1}); err == nil {
		t.Fatal("negative markdown limit succeeded")
	}
	if _, err := normalizeLimits(Limits{MaxFrameBytes: maxWireFrameBytes + 1}); err == nil {
		t.Fatal("oversized wire frame limit succeeded")
	}
}
