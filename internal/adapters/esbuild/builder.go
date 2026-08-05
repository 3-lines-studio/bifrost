package esbuild

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/3-lines-studio/bifrost/internal/core"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/grafana/sobek"
)

type Builder struct {
	mode core.Mode

	tailwindMu sync.Mutex
	tailwind   map[string]*tailwindCompiler
}

type metafile struct {
	Inputs  map[string]struct{}       `json:"inputs"`
	Outputs map[string]metafileOutput `json:"outputs"`
}

type metafileOutput struct {
	EntryPoint string              `json:"entryPoint"`
	CSSBundle  string              `json:"cssBundle"`
	Imports    []metafileImport    `json:"imports"`
	Inputs     map[string]struct{} `json:"inputs"`
}

type metafileImport struct {
	Path string `json:"path"`
}

type tailwindCompiler struct {
	runtime *sobek.Runtime
	compile sobek.Callable
}

func NewBuilder(mode core.Mode) *Builder {
	return &Builder{mode: mode, tailwind: make(map[string]*tailwindCompiler)}
}

func (b *Builder) Build(entrypoints []string, outdir string, entryNames []string) (map[string]core.ClientBuildResult, error) {
	if len(entrypoints) == 0 {
		return nil, fmt.Errorf("missing entrypoints")
	}
	if outdir == "" {
		return nil, fmt.Errorf("missing outdir")
	}
	if len(entryNames) != len(entrypoints) {
		return nil, fmt.Errorf("entryNames length %d does not match entrypoints length %d", len(entryNames), len(entrypoints))
	}
	if err := os.MkdirAll(outdir, 0o755); err != nil {
		return nil, fmt.Errorf("create client output directory: %w", err)
	}

	advanced := make([]api.EntryPoint, len(entrypoints))
	for i := range entrypoints {
		advanced[i] = api.EntryPoint{InputPath: entrypoints[i], OutputPath: entryNames[i]}
	}
	production := b.mode != core.ModeDev
	entryPattern := "[name]"
	sourceMap := api.SourceMapInline
	if production {
		entryPattern = "[name]-[hash]"
		sourceMap = api.SourceMapNone
	}
	result := api.Build(api.BuildOptions{
		EntryPointsAdvanced: advanced,
		Outdir:              outdir,
		Bundle:              true,
		Write:               false,
		Platform:            api.PlatformBrowser,
		Format:              api.FormatESModule,
		Target:              api.ES2015,
		Splitting:           true,
		Metafile:            true,
		Sourcemap:           sourceMap,
		Conditions:          []string{"browser", "style"},
		EntryNames:          entryPattern,
		ChunkNames:          "chunk-[name]-[hash]",
		AssetNames:          "asset-[name]-[hash]",
		MinifyWhitespace:    production,
		MinifyIdentifiers:   production,
		MinifySyntax:        production,
		Define: map[string]string{
			"process.env.NODE_ENV": quotedNodeEnv(production),
		},
		LogLevel: api.LogLevelSilent,
	})
	if err := buildError("client build", result.Errors); err != nil {
		return nil, err
	}

	var meta metafile
	if err := json.Unmarshal([]byte(result.Metafile), &meta); err != nil {
		return nil, fmt.Errorf("decode esbuild client metadata: %w", err)
	}
	if err := b.processCSS(entrypoints, result.OutputFiles, meta, production); err != nil {
		return nil, err
	}
	if err := writeOutputs(result.OutputFiles); err != nil {
		return nil, err
	}
	return mapClientResults(meta, entrypoints, entryNames)
}

func (b *Builder) BuildSSR(entrypoints []string, outdir string) error {
	if len(entrypoints) == 0 {
		return fmt.Errorf("missing entrypoints")
	}
	if outdir == "" {
		return fmt.Errorf("missing outdir")
	}
	if err := os.MkdirAll(outdir, 0o755); err != nil {
		return fmt.Errorf("create SSR output directory: %w", err)
	}
	production := b.mode != core.ModeDev
	sourceMap := api.SourceMapInline
	if production {
		sourceMap = api.SourceMapNone
	}
	result := api.Build(api.BuildOptions{
		EntryPoints:       entrypoints,
		Outdir:            outdir,
		Bundle:            true,
		Write:             true,
		Platform:          api.PlatformBrowser,
		Format:            api.FormatESModule,
		Target:            api.ES2015,
		Splitting:         false,
		Sourcemap:         sourceMap,
		Conditions:        []string{"browser"},
		Plugins:           []api.Plugin{ignoreCSSPlugin()},
		EntryNames:        "[name]",
		MinifyWhitespace:  production,
		MinifyIdentifiers: production,
		MinifySyntax:      production,
		Define: map[string]string{
			"process.env.NODE_ENV": quotedNodeEnv(production),
		},
		LogLevel: api.LogLevelSilent,
	})
	return buildError("SSR build", result.Errors)
}

func ignoreCSSPlugin() api.Plugin {
	return api.Plugin{
		Name: "bifrost-ignore-ssr-css",
		Setup: func(build api.PluginBuild) {
			build.OnLoad(api.OnLoadOptions{Filter: `\.css$`}, func(api.OnLoadArgs) (api.OnLoadResult, error) {
				empty := ""
				return api.OnLoadResult{Contents: &empty, Loader: api.LoaderEmpty}, nil
			})
		},
	}
}

func quotedNodeEnv(production bool) string {
	if production {
		return `"production"`
	}
	return `"development"`
}

func buildError(prefix string, messages []api.Message) error {
	if len(messages) == 0 {
		return nil
	}
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		text := message.Text
		if message.Location != nil {
			text = fmt.Sprintf("%s:%d:%d: %s", message.Location.File, message.Location.Line, message.Location.Column, text)
		}
		parts = append(parts, text)
	}
	return fmt.Errorf("%s failed: %s", prefix, strings.Join(parts, "; "))
}

func writeOutputs(outputs []api.OutputFile) error {
	for _, output := range outputs {
		if err := os.MkdirAll(filepath.Dir(output.Path), 0o755); err != nil {
			return fmt.Errorf("create output directory for %q: %w", output.Path, err)
		}
		if err := os.WriteFile(output.Path, output.Contents, 0o644); err != nil {
			return fmt.Errorf("write build output %q: %w", output.Path, err)
		}
	}
	return nil
}

func mapClientResults(meta metafile, entrypoints, entryNames []string) (map[string]core.ClientBuildResult, error) {
	out := make(map[string]core.ClientBuildResult, len(entryNames))
	for i, entrypoint := range entrypoints {
		entryKey := findEntryOutput(meta.Outputs, entrypoint)
		if entryKey == "" {
			return nil, fmt.Errorf("no JavaScript output for client entry %q", entrypoint)
		}
		css := ""
		if cssKey := findOutputKey(meta.Outputs, meta.Outputs[entryKey].CSSBundle); cssKey != "" {
			css = distURL(cssKey)
		}
		chunks := collectImportedChunks(meta.Outputs, entryKey)
		out[entryNames[i]] = core.ClientBuildResult{
			Script: distURL(entryKey),
			CSS:    css,
			Chunks: chunks,
		}
	}
	return out, nil
}

func findEntryOutput(outputs map[string]metafileOutput, entrypoint string) string {
	want, _ := filepath.Abs(entrypoint)
	for key, output := range outputs {
		if output.EntryPoint == "" || filepath.Ext(key) != ".js" {
			continue
		}
		got, _ := filepath.Abs(output.EntryPoint)
		if filepath.Clean(got) == filepath.Clean(want) {
			return key
		}
	}
	return ""
}

func findOutputKey(outputs map[string]metafileOutput, path string) string {
	if path == "" {
		return ""
	}
	if _, ok := outputs[path]; ok {
		return path
	}
	clean := filepath.Clean(path)
	for key := range outputs {
		if filepath.Clean(key) == clean || filepath.Base(key) == filepath.Base(clean) {
			return key
		}
	}
	return ""
}

func collectImportedChunks(outputs map[string]metafileOutput, entryKey string) []string {
	seen := make(map[string]struct{})
	var chunks []string
	var visit func(string)
	visit = func(key string) {
		for _, imported := range outputs[key].Imports {
			child := findOutputKey(outputs, imported.Path)
			if child == "" || filepath.Ext(child) != ".js" || child == entryKey {
				continue
			}
			if _, ok := seen[child]; ok {
				continue
			}
			seen[child] = struct{}{}
			chunks = append(chunks, distURL(child))
			visit(child)
		}
	}
	visit(entryKey)
	sort.Strings(chunks)
	return chunks
}

func distURL(path string) string {
	return "/dist/" + filepath.Base(filepath.Clean(path))
}

func (b *Builder) processCSS(entrypoints []string, outputs []api.OutputFile, meta metafile, minify bool) error {
	for i := range outputs {
		if filepath.Ext(outputs[i].Path) != ".css" || !usesTailwind(outputs[i].Contents) {
			continue
		}
		candidates := collectTailwindCandidates(inputsForCSS(meta.Outputs, outputs[i].Path))
		packageRoot := findNodePackage(filepath.Dir(entrypoints[0]), "tailwindcss")
		if packageRoot == "" {
			return fmt.Errorf("tailwind CSS was imported but node_modules/tailwindcss was not found")
		}
		compiled, err := b.compileTailwind(packageRoot, string(outputs[i].Contents), candidates)
		if err != nil {
			return fmt.Errorf("compile Tailwind output %q: %w", outputs[i].Path, err)
		}
		if minify {
			transformed := api.Transform(compiled, api.TransformOptions{
				Loader:           api.LoaderCSS,
				MinifyWhitespace: true,
				LegalComments:    api.LegalCommentsNone,
				LogLevel:         api.LogLevelSilent,
			})
			if err := buildError("CSS minification", transformed.Errors); err != nil {
				return err
			}
			outputs[i].Contents = transformed.Code
		} else {
			outputs[i].Contents = []byte(compiled)
		}
		if minify {
			oldPath := outputs[i].Path
			newPath := contentHashedCSSPath(oldPath, outputs[i].Contents)
			if newPath != oldPath {
				updateMetafileOutputPath(meta.Outputs, oldPath, newPath)
				outputs[i].Path = newPath
			}
		}
	}
	return nil
}

func contentHashedCSSPath(path string, content []byte) string {
	ext := filepath.Ext(path)
	name := strings.TrimSuffix(filepath.Base(path), ext)
	if dash := strings.LastIndexByte(name, '-'); dash >= 0 {
		name = name[:dash]
	}
	hash := sha256.Sum256(content)
	return filepath.Join(filepath.Dir(path), fmt.Sprintf("%s-%x%s", name, hash[:4], ext))
}

func updateMetafileOutputPath(outputs map[string]metafileOutput, oldPath, newPath string) {
	oldKey := findOutputKey(outputs, oldPath)
	if oldKey == "" {
		return
	}
	newKey := filepath.ToSlash(filepath.Join(filepath.Dir(oldKey), filepath.Base(newPath)))
	node := outputs[oldKey]
	delete(outputs, oldKey)
	outputs[newKey] = node
	for key, output := range outputs {
		if output.CSSBundle != "" && filepath.Base(output.CSSBundle) == filepath.Base(oldKey) {
			output.CSSBundle = newKey
		}
		for i := range output.Imports {
			if filepath.Base(output.Imports[i].Path) == filepath.Base(oldKey) {
				output.Imports[i].Path = newKey
			}
		}
		outputs[key] = output
	}
}

func inputsForCSS(outputs map[string]metafileOutput, cssPath string) map[string]struct{} {
	cssKey := findOutputKey(outputs, cssPath)
	inputs := make(map[string]struct{})
	seen := make(map[string]struct{})
	var visit func(string)
	visit = func(key string) {
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		for input := range outputs[key].Inputs {
			inputs[input] = struct{}{}
		}
		for _, imported := range outputs[key].Imports {
			if child := findOutputKey(outputs, imported.Path); child != "" && filepath.Ext(child) == ".js" {
				visit(child)
			}
		}
	}
	for key, output := range outputs {
		if findOutputKey(outputs, output.CSSBundle) == cssKey {
			visit(key)
		}
	}
	return inputs
}

func usesTailwind(css []byte) bool {
	text := string(css)
	return strings.Contains(text, "@tailwind") ||
		strings.Contains(text, "@theme") ||
		strings.Contains(text, "@custom-variant") ||
		strings.Contains(text, "@utility")
}

func findNodePackage(start, name string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, "node_modules", filepath.FromSlash(name))
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func (b *Builder) compileTailwind(packageRoot, css string, candidates []string) (string, error) {
	b.tailwindMu.Lock()
	defer b.tailwindMu.Unlock()
	compiler, ok := b.tailwind[packageRoot]
	if !ok {
		var err error
		compiler, err = loadTailwindCompiler(packageRoot)
		if err != nil {
			return "", err
		}
		b.tailwind[packageRoot] = compiler
	}
	value, err := compiler.compile(sobek.Undefined(), compiler.runtime.ToValue(css))
	if err != nil {
		return "", err
	}
	promise, ok := value.Export().(*sobek.Promise)
	if !ok {
		return "", fmt.Errorf("tailwind compile returned %T, not a Promise", value.Export())
	}
	if promise.State() != sobek.PromiseStateFulfilled {
		return "", fmt.Errorf("tailwind compile Promise did not fulfill: %s", promise.Result().String())
	}
	compiled := promise.Result().ToObject(compiler.runtime)
	build, ok := sobek.AssertFunction(compiled.Get("build"))
	if !ok {
		return "", fmt.Errorf("tailwind compile result has no build function")
	}
	output, err := build(compiled, compiler.runtime.ToValue(candidates))
	if err != nil {
		return "", err
	}
	return output.String(), nil
}

func loadTailwindCompiler(packageRoot string) (*tailwindCompiler, error) {
	entry := filepath.Join(packageRoot, "dist", "lib.mjs")
	result := api.Build(api.BuildOptions{
		EntryPoints:       []string{entry},
		Bundle:            true,
		Write:             false,
		Platform:          api.PlatformBrowser,
		Format:            api.FormatIIFE,
		GlobalName:        "__BIFROST_TAILWIND__",
		Target:            api.ES2015,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		MinifySyntax:      true,
		LogLevel:          api.LogLevelSilent,
	})
	if err := buildError("Tailwind compiler bundle", result.Errors); err != nil {
		return nil, err
	}
	if len(result.OutputFiles) != 1 {
		return nil, fmt.Errorf("tailwind compiler bundle returned %d files", len(result.OutputFiles))
	}
	runtime := sobek.New()
	if _, err := runtime.RunString(string(result.OutputFiles[0].Contents)); err != nil {
		return nil, fmt.Errorf("load Tailwind compiler in Sobek: %w", err)
	}
	global := runtime.Get("__BIFROST_TAILWIND__")
	if sobek.IsUndefined(global) || sobek.IsNull(global) {
		return nil, fmt.Errorf("tailwind compiler global was not created")
	}
	compile, ok := sobek.AssertFunction(global.ToObject(runtime).Get("compile"))
	if !ok {
		return nil, fmt.Errorf("tailwind compiler has no compile function")
	}
	return &tailwindCompiler{runtime: runtime, compile: compile}, nil
}

var quotedSource = regexp.MustCompile("(?s)\"(?:\\\\.|[^\"\\\\])*\"|'(?:\\\\.|[^'\\\\])*'|`(?:\\\\.|[^`\\\\])*`")

func collectTailwindCandidates(inputs map[string]struct{}) []string {
	set := make(map[string]struct{})
	for path := range inputs {
		clean := filepath.ToSlash(path)
		if strings.Contains(clean, "/node_modules/") || strings.Contains(clean, "/.bifrost/") {
			continue
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".js", ".jsx", ".ts", ".tsx", ".html":
		default:
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, literal := range quotedSource.FindAllString(string(data), -1) {
			if len(literal) < 2 {
				continue
			}
			for field := range strings.FieldsSeq(literal[1 : len(literal)-1]) {
				candidate := strings.Trim(field, "\\\"'`,;(){}")
				if candidate != "" && !strings.ContainsAny(candidate, "<>={}") {
					set[candidate] = struct{}{}
				}
			}
		}
	}
	out := make([]string, 0, len(set))
	for candidate := range set {
		out = append(out, candidate)
	}
	sort.Strings(out)
	return out
}
