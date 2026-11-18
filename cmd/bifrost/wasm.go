package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// wasmOutput is where the browser build goes: the generated server serves it
// from there, and BIFROST_WASM_DIR points it elsewhere.
func wasmOutput(projectRoot string) string {
	return filepath.Join(projectRoot, "dist", "web")
}

// wasmStatic is where a build copies the browser build for the server to carry
// inside itself: the binary deploys alone.
func wasmStatic(projectRoot string) string {
	return filepath.Join(projectRoot, ".bifrost", "app", "static")
}

// buildWasm compiles the app for the browser and writes app.wasm and the
// wasm_exec.js of the Go toolchain beside it. The wasm is renamed into place,
// so a request never reads half a file. A build also writes the gzip of it,
// which a dev server leaves to the first request that asks.
func buildWasm(ctx context.Context, projectRoot string, compress bool) error {
	out := wasmOutput(projectRoot)
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	target := filepath.Join(out, "app.wasm")
	temporary := target + ".tmp"
	command := exec.CommandContext(ctx, "go", "build", "-trimpath", "-buildvcs=false", "-ldflags=-s -w", "-o", temporary, ".")
	command.Dir = projectRoot
	command.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("bifrost: build the browser app: %w", err)
	}
	if err := os.Rename(temporary, target); err != nil {
		return err
	}
	if err := copyWasmExec(out); err != nil {
		return err
	}
	if !compress {
		// A compressed build of an older wasm would be served beside the new
		// one, so a dev server drops it and the file server compresses what
		// it reads.
		for _, suffix := range []string{".br", ".gz"} {
			if err := os.Remove(target + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		return nil
	}
	return writeEncoded(target)
}

// writeEncoded leaves the compressed builds of the wasm beside it, so serving
// the app costs a read and not a compression.
func writeEncoded(target string) error {
	data, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	var gzipOut bytes.Buffer
	gzipWriter, err := gzip.NewWriterLevel(&gzipOut, gzip.BestCompression)
	if err != nil {
		return err
	}
	if _, err := gzipWriter.Write(data); err != nil {
		return err
	}
	if err := gzipWriter.Close(); err != nil {
		return err
	}
	return writeAtomically(target+".gz", gzipOut.Bytes())
}

func writeAtomically(path string, data []byte) error {
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o644); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

// clearWasmStatic leaves the generated server without a browser build, which
// is what a dev server serves from disk and what a build clears before it
// copies the new one in.
func clearWasmStatic(projectRoot string) error {
	target := wasmStatic(projectRoot)
	if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	return writeStaticPlaceholder(target)
}

// writeStaticPlaceholder keeps the directory embeddable when it holds no build:
// go:embed fails on a directory of no files.
func writeStaticPlaceholder(target string) error {
	return os.WriteFile(filepath.Join(target, ".keep"), []byte("the browser build, copied here by bifrost build\n"), 0o644)
}

// copyWasmStatic copies the browser build into the generated server, which
// carries it inside the binary: a build deploys as one file.
func copyWasmStatic(projectRoot string) error {
	if err := clearWasmStatic(projectRoot); err != nil {
		return err
	}
	target := wasmStatic(projectRoot)
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	source := wasmOutput(projectRoot)
	for _, name := range []string{"app.wasm", "app.wasm.gz", "wasm_exec.js"} {
		data, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		if err := os.WriteFile(filepath.Join(target, name), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// copyWasmExec copies the runtime the browser needs from the Go toolchain.
func copyWasmExec(out string) error {
	goroot, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return fmt.Errorf("bifrost: locate GOROOT: %w", err)
	}
	root := strings.TrimSpace(string(goroot))
	candidates := []string{
		filepath.Join(root, "lib", "wasm", "wasm_exec.js"),
		filepath.Join(root, "misc", "wasm", "wasm_exec.js"),
	}
	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		return os.WriteFile(filepath.Join(out, "wasm_exec.js"), data, 0o644)
	}
	return errors.New("bifrost: the Go toolchain has no wasm_exec.js")
}
