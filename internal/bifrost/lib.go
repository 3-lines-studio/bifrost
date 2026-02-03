package bifrost

import (
	_ "embed"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"html/template"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	//go:embed page.html
	pageTemplateSource string
	PageTemplate       = template.Must(template.New("page").Parse(pageTemplateSource))

	//go:embed client-entry.tsx
	clientEntryTemplateSource string
	ClientEntryTemplate       = template.Must(template.New("client-entry").Parse(clientEntryTemplateSource))

	//go:embed error.html
	errorTemplateSource string
	ErrorTemplate       = template.Must(template.New("error").Parse(errorTemplateSource))

	//go:embed reload.js
	reloadScriptSource string

	//go:embed bun_renderer.ts
	BunRendererSource string
)

const (
	PublicDir    = "public"
	BifrostDir   = ".bifrost"
	ManifestFile = "manifest.json"
	DistDir      = "dist"
)

func IsDev() bool {
	watchEnv := strings.ToLower(os.Getenv("BIFROST_DEV"))
	return watchEnv == "1" || watchEnv == "true" || watchEnv == "yes"
}

func extractComponentPaths(filename string) ([]string, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file: %w", err)
	}

	var paths []string
	seen := make(map[string]bool)

	ast.Inspect(node, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		var funcName string
		var argIndex int

		switch fn := callExpr.Fun.(type) {
		case *ast.SelectorExpr:
			funcName = fn.Sel.Name
			argIndex = 0
		case *ast.Ident:
			funcName = fn.Name
			argIndex = 1
		default:
			return true
		}

		if funcName != "NewPage" {
			return true
		}

		if len(callExpr.Args) <= argIndex {
			return true
		}

		firstArg := callExpr.Args[argIndex]
		lit, ok := firstArg.(*ast.BasicLit)
		if !ok {
			slog.Warn("NewPage call with non-string argument", "position", fset.Position(callExpr.Pos()))
			return true
		}

		if lit.Kind != token.STRING {
			slog.Warn("NewPage call with non-string argument", "position", fset.Position(callExpr.Pos()))
			return true
		}

		path, err := strconv.Unquote(lit.Value)
		if err != nil {
			slog.Warn("failed to unquote string", "position", fset.Position(lit.Pos()), "error", err)
			return true
		}

		if !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}

		return true
	})

	return paths, nil
}

func generateManifest(outdir string, componentPaths []string) (*buildManifest, error) {
	entries := make(map[string]manifestEntry)
	chunks := make(map[string]string)

	files, err := os.ReadDir(outdir)
	if err != nil {
		return nil, fmt.Errorf("failed to read outdir: %w", err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()

		if strings.HasPrefix(name, "chunk-") && strings.HasSuffix(name, ".js") {
			chunks[name] = "/dist/" + name
		}
	}

	cssFiles := make(map[string]string)
	cssHashToFile := make(map[string]string)

	for _, componentPath := range componentPaths {
		entryName := EntryNameForPath(componentPath)
		var script, css string
		var entryChunks []string

		for _, file := range files {
			if file.IsDir() {
				continue
			}

			name := file.Name()

			if strings.HasPrefix(name, entryName+"-") || strings.HasPrefix(name, entryName+".") {
				if strings.HasSuffix(name, ".js") {
					script = "/dist/" + name
				} else if strings.HasSuffix(name, ".css") {
					css = "/dist/" + name
				}
			}
		}

		if script != "" {
			entryChunks = findEntryChunks(outdir, script, chunks)

			if css != "" {
				css = dedupeCSSFile(outdir, css, cssFiles, cssHashToFile)
			}

			entries[entryName] = manifestEntry{
				Script: script,
				CSS:    css,
				Chunks: entryChunks,
			}
		}
	}

	return &buildManifest{Entries: entries, Chunks: chunks}, nil
}

func dedupeCSSFile(outdir string, cssPath string, cssFiles map[string]string, cssHashToFile map[string]string) string {
	fullPath := filepath.Join(outdir, filepath.Base(cssPath))
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return cssPath
	}

	hash := hashContent(content)

	if existingPath, exists := cssHashToFile[hash]; exists {
		os.Remove(fullPath)
		return existingPath
	}

	cssHashToFile[hash] = cssPath
	return cssPath
}

func hashContent(content []byte) string {
	result := 0
	for _, b := range content {
		result = (result*31 + int(b)) % 1000000007
	}
	return fmt.Sprintf("%d", result)
}

func findEntryChunks(outdir string, entryScript string, allChunks map[string]string) []string {
	var chunks []string

	content, err := os.ReadFile(strings.TrimPrefix(entryScript, "/dist/"))
	if err != nil {
		return chunks
	}

	contentStr := string(content)
	for chunkName := range allChunks {
		if strings.Contains(contentStr, chunkName) {
			chunks = append(chunks, allChunks[chunkName])
		}
	}

	return chunks
}
