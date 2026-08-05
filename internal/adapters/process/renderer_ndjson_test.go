package process

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func frame(kind byte, payload []byte) []byte {
	out := make([]byte, 5+len(payload))
	out[0] = kind
	binary.BigEndian.PutUint32(out[1:5], uint32(len(payload)))
	copy(out[5:], payload)
	return out
}

func renderOKFrame(head, body string) []byte {
	p := []byte{frameKindRender}
	p = append(p, be32Bytes(len(head))...)
	p = append(p, head...)
	p = append(p, be32Bytes(len(body))...)
	p = append(p, body...)
	return p
}

func be32Bytes(n int) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(n))
	return b
}

func TestFrameReader_RenderOK(t *testing.T) {
	data := renderOKFrame("<title>x</title>", "<p>y</p>")
	fr := &frameReader{r: bytes.NewReader(data)}
	kind, err := fr.readByte()
	if err != nil {
		t.Fatal(err)
	}
	if kind != frameKindRender {
		t.Fatalf("kind = %d, want %d", kind, frameKindRender)
	}
	head, err := fr.readString()
	if err != nil {
		t.Fatal(err)
	}
	body, err := fr.readString()
	if err != nil {
		t.Fatal(err)
	}
	if head != "<title>x</title>" || body != "<p>y</p>" {
		t.Fatalf("got head=%q body=%q", head, body)
	}
}

func TestFrameReader_RenderOK_ScratchReuse(t *testing.T) {
	data := renderOKFrame("<h>", "<p>y</p>")
	fr := &frameReader{r: bytes.NewReader(data)}
	if _, err := fr.readByte(); err != nil {
		t.Fatal(err)
	}
	head, err := fr.readString()
	if err != nil {
		t.Fatal(err)
	}
	if head != "<h>" {
		t.Fatalf("head = %q", head)
	}
	// The second string must not be corrupted by scratch reuse.
	body, err := fr.readString()
	if err != nil {
		t.Fatal(err)
	}
	if body != "<p>y</p>" {
		t.Fatalf("body = %q", body)
	}
}

func TestFrameReader_ErrorFrame(t *testing.T) {
	errJSON := `{"message":"boom","stack":"at page"}`
	data := frame(frameKindError, []byte(errJSON))
	fr := &frameReader{r: bytes.NewReader(data)}
	kind, err := fr.readByte()
	if err != nil {
		t.Fatal(err)
	}
	if kind != frameKindError {
		t.Fatalf("kind = %d, want %d", kind, frameKindError)
	}
	err = fr.readError()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected boom in error, got %v", err)
	}
}

func TestWriteFrame_Header(t *testing.T) {
	var buf bytes.Buffer
	payload := []byte("abc")
	if err := writeFrame(&buf, frameKindBuild, payload); err != nil {
		t.Fatal(err)
	}
	got := buf.Bytes()
	if len(got) != 8 {
		t.Fatalf("len = %d, want 8", len(got))
	}
	if got[0] != frameKindBuild {
		t.Fatalf("kind = %d", got[0])
	}
	if binary.BigEndian.Uint32(got[1:5]) != 3 {
		t.Fatalf("payload len = %d", binary.BigEndian.Uint32(got[1:5]))
	}
	if string(got[5:]) != "abc" {
		t.Fatalf("payload = %q", got[5:])
	}
}
