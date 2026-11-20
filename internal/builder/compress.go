package builder

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"

	"github.com/3-lines-studio/bifrost/internal/protocol"
)

//go:embed compress.ts
var compressSource []byte

// compressible lists the artifact extensions worth precompressing. Everything
// else (images, fonts, video) is already encoded.
var compressible = map[string]struct{}{
	".css":  {},
	".html": {},
	".js":   {},
	".json": {},
	".mjs":  {},
	".svg":  {},
	".txt":  {},
	".xml":  {},
}

type compressRequest struct {
	Dir   string   `json:"dir"`
	Paths []string `json:"paths"`
}

// compressArtifacts writes brotli and gzip siblings for every compressible
// artifact and returns the manifest entries that describe them.
func compressArtifacts(ctx context.Context, output string, refs []protocol.FileRef) ([]protocol.EncodedFile, error) {
	seen := make(map[string]struct{}, len(refs))
	paths := make([]string, 0, len(refs))
	for _, ref := range refs {
		if _, ok := compressible[path.Ext(ref.Path)]; !ok {
			continue
		}
		if _, ok := seen[ref.Path]; ok {
			continue
		}
		seen[ref.Path] = struct{}{}
		paths = append(paths, ref.Path)
	}
	if len(paths) == 0 {
		return nil, nil
	}
	slices.Sort(paths)

	data, err := json.Marshal(compressRequest{Dir: output, Paths: paths})
	if err != nil {
		return nil, err
	}
	script := filepath.Join(output, "entries", "compress.ts")
	if err := os.WriteFile(script, compressSource, 0o600); err != nil {
		return nil, err
	}
	command := exec.CommandContext(ctx, "bun", "run", script)
	command.Dir = output
	command.Stdin = bytes.NewReader(data)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Env = append(os.Environ(), "BIFROST=1", "BIFROST_BUILD=1")
	if err := command.Run(); err != nil {
		return nil, commandError(command, err)
	}

	encoded := make([]protocol.EncodedFile, 0, len(paths)*2)
	for _, candidate := range paths {
		for _, encoding := range []struct{ name, suffix string }{{"br", ".br"}, {"gzip", ".gz"}} {
			ref, err := fileRef(output, filepath.Join(output, filepath.FromSlash(candidate+encoding.suffix)))
			if err != nil {
				return nil, fmt.Errorf("bifrost: compress %q: %w", candidate, err)
			}
			encoded = append(encoded, protocol.EncodedFile{Path: candidate, Encoding: encoding.name, File: ref})
		}
	}
	return encoded, nil
}
