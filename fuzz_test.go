package bifrost

import (
	"encoding/json"
	"testing"
)

func FuzzParseManifest(f *testing.F) {
	f.Add([]byte(`{"schema":1}`))
	f.Add([]byte(`not json`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseManifest(data)
	})
}

func FuzzDocumentPath(f *testing.F) {
	for _, seed := range []string{"/", "/blog/post", "../bad", "/a//b", "/a?b"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		_ = validateDocumentPath(value)
	})
}

func FuzzRawProps(f *testing.F) {
	f.Add([]byte(`{"safe":true}`))
	f.Add([]byte(`{"x":"</script>"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		props, err := normalizeRawProps(json.RawMessage(data))
		if err == nil && !json.Valid(props) {
			t.Fatalf("normalized props are invalid: %q", props)
		}
	})
}
