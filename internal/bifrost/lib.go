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

	//go:embed static_client_entry.tsx
	staticClientEntryTemplateSource string
	StaticClientEntryTemplate       = template.Must(template.New("static-client-entry").Parse(staticClientEntryTemplateSource))

	//go:embed server-entry.tsx
	serverEntryTemplateSource string
	ServerEntryTemplate       = template.Must(template.New("server-entry").Parse(serverEntryTemplateSource))

	//go:embed error.html
	errorTemplateSource string
	ErrorTemplate       = template.Must(template.New("error").Parse(errorTemplateSource))

	//go:embed bun_renderer_dev.ts
	BunRendererDevSource string

	//go:embed bun_renderer_prod.ts
	BunRendererProdSource string
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

type pageConfig struct {
	path   string
	static bool
}

func extractPageConfigs(filename string) ([]pageConfig, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file: %w", err)
	}

	var configs []pageConfig
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

		static := hasWithClientOnlyOption(callExpr.Args[argIndex:])

		if !seen[path] {
			seen[path] = true
			configs = append(configs, pageConfig{path: path, static: static})
		}

		return true
	})

	return configs, nil
}

func hasWithClientOnlyOption(args []ast.Expr) bool {
	for _, arg := range args {
		callExpr, ok := arg.(*ast.CallExpr)
		if !ok {
			continue
		}

		var funcName string
		switch fn := callExpr.Fun.(type) {
		case *ast.SelectorExpr:
			funcName = fn.Sel.Name
		case *ast.Ident:
			funcName = fn.Name
		}

		if funcName == "WithClientOnly" {
			return true
		}
	}
	return false
}

func extractComponentPaths(filename string) ([]string, error) {
	configs, err := extractPageConfigs(filename)
	if err != nil {
		return nil, err
	}

	paths := make([]string, len(configs))
	for i, config := range configs {
		paths[i] = config.path
	}
	return paths, nil
}

func generateManifest(outdir string, ssrDir string, componentPaths []string, staticFlags map[string]bool) (*buildManifest, error) {
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

			isStatic := staticFlags[entryName]
			ssrPath := findSSRPath(ssrDir, entryName)
			entries[entryName] = manifestEntry{
				Script: script,
				CSS:    css,
				Chunks: entryChunks,
				Static: isStatic,
				SSR:    ssrPath,
			}
		}
	}

	return &buildManifest{Entries: entries, Chunks: chunks}, nil
}

func findSSRPath(ssrDir string, entryName string) string {
	if ssrDir == "" {
		return ""
	}
	files, err := os.ReadDir(ssrDir)
	if err != nil {
		return ""
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		name := file.Name()
		if strings.HasPrefix(name, entryName+"-") || strings.HasPrefix(name, entryName+".") {
			if strings.HasSuffix(name, ".js") {
				return "/ssr/" + name
			}
		}
	}
	return ""
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

func generateStaticHTMLFiles(entryDir, outdir string, componentPaths []string, heads map[string]string, man *buildManifest) error {
	pagesDir := filepath.Join(entryDir, "pages")
	if err := os.MkdirAll(pagesDir, 0755); err != nil {
		return fmt.Errorf("failed to create pages directory: %w", err)
	}

	for _, componentPath := range componentPaths {
		entryName := EntryNameForPath(componentPath)
		pageDir := filepath.Join(pagesDir, entryName)
		if err := os.MkdirAll(pageDir, 0755); err != nil {
			return fmt.Errorf("failed to create page directory %s: %w", pageDir, err)
		}

		scriptSrc, cssHref, chunks, _, _ := getAssetsFromManifest(man, entryName)
		htmlPath := filepath.Join(pageDir, "index.html")

		// Get the rendered head HTML for this page
		headHTML := heads[componentPath]

		if err := writeStaticHTML(htmlPath, scriptSrc, cssHref, chunks, headHTML, outdir); err != nil {
			return fmt.Errorf("failed to write static HTML for %s: %w", componentPath, err)
		}

		fmt.Printf("  📄 %s\n", htmlPath)
	}

	return nil
}

func writeStaticHTML(htmlPath, scriptSrc, cssHref string, chunks []string, headHTML string, outdir string) error {
	relScript := makeRelativeToPages(scriptSrc)
	relChunks := make([]string, len(chunks))
	for i, chunk := range chunks {
		relChunks[i] = makeRelativeToPages(chunk)
	}

	var cssLink string
	if cssHref != "" {
		cssLink = fmt.Sprintf(`<link rel="stylesheet" href="%s" />`, makeRelativeToPages(cssHref))
	}

	head := `<meta charset="UTF-8" /><meta name="viewport" content="width=device-width, initial-scale=1.0" />`
	if headHTML != "" {
		head += headHTML
	}
	head += cssLink

	var chunksHTML strings.Builder
	for _, chunk := range relChunks {
		chunksHTML.WriteString(fmt.Sprintf(`<script src="%s" type="module" defer></script>`, chunk))
		chunksHTML.WriteString("\n")
	}

	html := fmt.Sprintf(`<!doctype html>
<html lang="en">
  <head>
    %s
  </head>
  <body>
    <div id="app"></div>
%s    <script src="%s" type="module" defer></script>
  </body>
</html>
`, head, chunksHTML.String(), relScript)

	return os.WriteFile(htmlPath, []byte(html), 0644)
}

func makeRelativeToPages(path string) string {
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	depth := 2
	prefix := strings.Repeat("../", depth)
	return prefix + strings.Join(parts, "/")
}
