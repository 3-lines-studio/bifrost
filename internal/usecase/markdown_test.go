package usecase

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func TestConvertHTMLToMarkdown_Headings(t *testing.T) {
	md, err := convertHTMLToMarkdown("<h1>Hello</h1>")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if md != "# Hello" {
		t.Errorf("expected %q, got %q", "# Hello", md)
	}
}

func TestConvertHTMLToMarkdown_ParagraphWithFormatting(t *testing.T) {
	md, err := convertHTMLToMarkdown("<p>foo <strong>bar</strong></p>")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if md != "foo **bar**" {
		t.Errorf("expected %q, got %q", "foo **bar**", md)
	}
}

func TestConvertHTMLToMarkdown_List(t *testing.T) {
	md, err := convertHTMLToMarkdown("<ul><li>one</li><li>two</li></ul>")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "- one") || !strings.Contains(md, "- two") {
		t.Errorf("expected list items, got %q", md)
	}
}

func TestRenderSSR_MarkdownOutputSkipsShellWrapping(t *testing.T) {
	renderer := &fakeRenderer{
		renderFn: func(componentPath string, props any) (core.RenderedPage, error) {
			return core.RenderedPage{
				Head: "<title>Home</title>",
				Body: "<h1>Hello</h1><p>World</p>",
			}, nil
		},
	}
	service := NewPageService(renderer)

	input := ServePageInput{
		Config: core.PageConfig{
			ComponentPath: "./pages/home.tsx",
			Mode:          core.ModeSSR,
		},
		DefaultHTMLLang: "en",
		IsDev:           false,
		EntryName:       core.EntryNameForPath("./pages/home.tsx"),
		RequestPath:     "/",
		Request:         httptest.NewRequest(http.MethodGet, "/", nil),
		Markdown:        true,
	}

	output := service.ServePage(context.Background(), input)
	if output.Error != nil {
		t.Fatalf("ServePage() error = %v", output.Error)
	}
	if output.Action != core.ActionRenderSSR {
		t.Fatalf("expected action %v, got %v", core.ActionRenderSSR, output.Action)
	}
	if output.HTML != "" {
		t.Errorf("expected empty HTML (shell wrapping skipped), got %q", output.HTML)
	}
	if !output.IsMarkdown {
		t.Fatal("expected markdown output marker")
	}
	if output.Markdown == "" {
		t.Fatal("expected non-empty markdown output")
	}
	if !strings.Contains(output.Markdown, "# Hello") {
		t.Errorf("expected heading in markdown, got %q", output.Markdown)
	}
	if strings.Contains(output.Markdown, "<html") || strings.Contains(output.Markdown, "<!doctype") {
		t.Errorf("markdown output should not contain shell HTML, got %q", output.Markdown)
	}
}

func TestServePage_MarkdownOnlyAllowedInSSR(t *testing.T) {
	cases := []struct {
		name string
		mode core.PageMode
	}{
		{name: "ClientOnly", mode: core.ModeClientOnly},
		{name: "StaticPrerender", mode: core.ModeStaticPrerender},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := NewPageService(&fakeRenderer{})
			output := service.ServePage(context.Background(), ServePageInput{
				Config: core.PageConfig{
					ComponentPath: "./pages/home.tsx",
					Mode:          tc.mode,
				},
				IsDev:    false,
				Markdown: true,
			})
			if output.Error == nil {
				t.Fatal("expected error for markdown in non-SSR mode")
			}
			if !strings.Contains(output.Error.Error(), "SSR mode") {
				t.Errorf("expected SSR mode error message, got %v", output.Error)
			}
		})
	}
}

func TestRenderForMode_MarkdownSSRDoesNotError(t *testing.T) {
	renderer := &fakeRenderer{
		renderFn: func(componentPath string, props any) (core.RenderedPage, error) {
			return core.RenderedPage{Body: "<p>ok</p>"}, nil
		},
	}
	shell, err := core.NewHTMLDocumentShell("/dist/home.js", "", nil, nil)
	if err != nil {
		t.Fatalf("setup shell: %v", err)
	}
	service := NewPageService(renderer)
	state := pageRequestState{
		input: ServePageInput{
			Config: core.PageConfig{
				ComponentPath: "./pages/home.tsx",
				Mode:          core.ModeSSR,
			},
			IsDev:       false,
			EntryName:   "home",
			RequestPath: "/",
			Request:     httptest.NewRequest(http.MethodGet, "/", nil),
			Markdown:    true,
		},
		shell: &shell,
	}
	output := service.renderForMode(context.Background(), state)
	if output.Error != nil {
		t.Fatalf("expected no error for markdown in SSR mode, got %v", output.Error)
	}
	if output.Markdown == "" || !output.IsMarkdown {
		t.Fatal("expected markdown output")
	}
}
