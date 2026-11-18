package web

import (
	"strings"
	"testing"
)

func TestShellKeepsContent(t *testing.T) {
	body := string(Shell([]byte(`<main><h1>Home</h1></main>`)))
	want := `<div id="seo"><main><h1>Home</h1></main></div><div id="mygo"></div>`
	if !strings.HasPrefix(body, want) {
		t.Fatalf("body:\n%s", body)
	}
}

func TestShellBootsApp(t *testing.T) {
	body := string(Shell(nil))
	wants := []string{
		`<div id="mygo"></div>`,
		`<div id="mygo-loading" role="status">`,
		`if(loader){loader.remove()}`,
		`}).catch(function(){`,
		`<script src="/wasm_exec.js"></script>`,
		`fetch("/app.wasm")`,
		`go.run(result.instance)`,
		`if(seo){seo.remove()}`,
	}
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Errorf("body misses %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, `<div id="seo">`) {
		t.Errorf("body has content: %s", body)
	}
	if strings.Contains(body, "history.replaceState") {
		t.Errorf("the shell rewrites the URL: %s", body)
	}
	if strings.Contains(body, "Loading") {
		t.Errorf("body has a translatable label: %s", body)
	}
	if !strings.Contains(body, `display:none`) {
		t.Errorf("the spinner shows before the app is late: %s", body)
	}
	if !strings.Contains(body, "setTimeout") {
		t.Errorf("the spinner has no delay: %s", body)
	}
	if !strings.Contains(body, `typeof Go==="undefined"`) {
		t.Errorf("a missing wasm_exec.js leaves the spinner over the HTML: %s", body)
	}
}

func TestShellDeterministic(t *testing.T) {
	if string(Shell([]byte("x"))) != string(Shell([]byte("x"))) {
		t.Fatal("shells differ")
	}
}

func TestShellStylesTheContent(t *testing.T) {
	body := string(Shell([]byte("<main><h1>Home</h1></main>")))
	if !strings.Contains(body, `#seo{max-width:42rem`) {
		t.Fatalf("body styles no content:\n%s", body)
	}
}

func TestShellReloadsOnlyWhenAsked(t *testing.T) {
	if strings.Contains(string(Shell(nil)), ReloadPath) {
		t.Fatal("the shell follows the server without being asked")
	}
	body := string(Shell(nil, Reload()))
	if !strings.Contains(body, `new EventSource("`+ReloadPath+`")`) {
		t.Fatalf("the shell does not follow the server:\n%s", body)
	}
}

func TestScreenShowsTheSpinnerAtOnce(t *testing.T) {
	body := string(Shell(nil, Screen()))
	if !strings.Contains(body, "#mygo-loading{display:flex}") {
		t.Errorf("a screen hides its spinner:\n%s", body)
	}
	if strings.Contains(body, "setTimeout") {
		t.Errorf("a screen waits before showing it:\n%s", body)
	}
	if strings.Contains(body, "loader.remove()}") && strings.Contains(body, "catch(function(){if(loader){loader.remove()}") {
		t.Errorf("a screen drops the spinner when the build fails:\n%s", body)
	}
	content := string(Shell([]byte("<main></main>")))
	if strings.Contains(content, "#mygo-loading{display:flex}") {
		t.Errorf("a page with content shows the spinner at once:\n%s", content)
	}
	if !strings.Contains(content, "setTimeout") {
		t.Errorf("a page with content waits before showing it:\n%s", content)
	}
}
