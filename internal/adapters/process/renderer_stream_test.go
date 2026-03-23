package process

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestParseRenderFirstLine_HeadOnly(t *testing.T) {
	head, html, err := parseRenderFirstLine([]byte(`{"head":"<title>x</title>"}` + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if html != nil {
		t.Fatalf("expected nil html ptr, got %v", *html)
	}
	if head != "<title>x</title>" {
		t.Fatalf("head: %q", head)
	}
}

func TestParseRenderFirstLine_HeadAndHTMLFallback(t *testing.T) {
	head, html, err := parseRenderFirstLine([]byte(`{"head":"h","html":"<p>b</p>"}` + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if html == nil || *html != "<p>b</p>" {
		t.Fatalf("html: %v", html)
	}
	if head != "h" {
		t.Fatalf("head: %q", head)
	}
}

func TestParseRenderFirstLine_Error(t *testing.T) {
	_, _, err := parseRenderFirstLine([]byte(`{"error":{"message":"bad"}}` + "\n"))
	if err == nil || !strings.Contains(err.Error(), "bad") {
		t.Fatalf("expected error, got %v", err)
	}
}

func TestHeadThenRawTail(t *testing.T) {
	in := strings.NewReader(`{"head":"H"}` + "\n" + `<main>body</main>`)
	br := bufio.NewReader(in)
	line, err := br.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	head, htmlPtr, err := parseRenderFirstLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if htmlPtr != nil {
		t.Fatal("expected streaming tail")
	}
	if head != "H" {
		t.Fatalf("head %q", head)
	}
	rest, err := io.ReadAll(br)
	if err != nil {
		t.Fatal(err)
	}
	if string(rest) != `<main>body</main>` {
		t.Fatalf("tail %q", rest)
	}
}

func TestCopyResponseBodyWithFlush_FlushesFirstChunkOnly(t *testing.T) {
	src := strings.NewReader(strings.Repeat("a", 64*1024))
	var dst bytes.Buffer
	flushes := 0

	written, err := copyResponseBodyWithFlush(&dst, src, func() { flushes++ }, false)
	if err != nil {
		t.Fatal(err)
	}
	if written != int64(dst.Len()) {
		t.Fatalf("written = %d, dst len = %d", written, dst.Len())
	}
	if flushes != 1 {
		t.Fatalf("flushes = %d, want 1", flushes)
	}
}

func TestCopyResponseBodyWithFlush_FlushesEveryChunkWhenRequested(t *testing.T) {
	src := strings.NewReader(strings.Repeat("a", 64*1024))
	var dst bytes.Buffer
	flushes := 0

	if _, err := copyResponseBodyWithFlush(&dst, src, func() { flushes++ }, true); err != nil {
		t.Fatal(err)
	}
	if flushes < 2 {
		t.Fatalf("flushes = %d, want at least 2", flushes)
	}
}
