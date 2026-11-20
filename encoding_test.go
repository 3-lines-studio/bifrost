package bifrost

import (
	"testing"

	"github.com/3-lines-studio/bifrost/internal/protocol"
)

func TestNegotiateEncoding(t *testing.T) {
	identity := protocol.FileRef{Path: "dist/app.js", Hash: "identity", Size: 100}
	brotli := protocol.FileRef{Path: "dist/app.js.br", Hash: "brotli", Size: 20}
	gzipped := protocol.FileRef{Path: "dist/app.js.gz", Hash: "gzip", Size: 30}
	encoded := map[string]protocol.FileRef{"br": brotli, "gzip": gzipped}

	testCases := []struct {
		name   string
		header string
		file   protocol.FileRef
		want   string
	}{
		{"brotli preferred", "gzip, deflate, br, zstd", brotli, "br"},
		{"gzip only", "gzip, deflate", gzipped, "gzip"},
		{"legacy gzip alias", "x-gzip", gzipped, "gzip"},
		{"wildcard", "*", brotli, "br"},
		{"identity only", "identity", identity, ""},
		{"empty header", "", identity, ""},
		{"brotli refused", "br;q=0, gzip", gzipped, "gzip"},
		{"both refused", "br;q=0, gzip;q=0", identity, ""},
		{"quality wins", "br;q=0.1, gzip;q=0.9", gzipped, "gzip"},
		{"quality ties fall back to preference", "br;q=0.5, gzip;q=0.5", brotli, "br"},
		{"malformed quality", "br;q=abc", identity, ""},
		{"upper case", "GZIP", gzipped, "gzip"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			file, encoding := negotiateEncoding(testCase.header, identity, encoded)
			if file != testCase.file || encoding != testCase.want {
				t.Fatalf("negotiateEncoding(%q) = %q %q, want %q %q", testCase.header, file.Path, encoding, testCase.file.Path, testCase.want)
			}
		})
	}
}

func TestNegotiateEncodingWithoutVariants(t *testing.T) {
	identity := protocol.FileRef{Path: "dist/app.js", Hash: "identity", Size: 100}
	file, encoding := negotiateEncoding("gzip, br", identity, nil)
	if file != identity || encoding != "" {
		t.Fatalf("negotiateEncoding without variants = %q %q", file.Path, encoding)
	}
}

func TestNegotiateEncodingSkipsMissingVariant(t *testing.T) {
	identity := protocol.FileRef{Path: "dist/app.js", Hash: "identity", Size: 100}
	gzipped := protocol.FileRef{Path: "dist/app.js.gz", Hash: "gzip", Size: 30}
	file, encoding := negotiateEncoding("br, gzip", identity, map[string]protocol.FileRef{"gzip": gzipped})
	if file != gzipped || encoding != "gzip" {
		t.Fatalf("negotiateEncoding with gzip only = %q %q", file.Path, encoding)
	}
}
