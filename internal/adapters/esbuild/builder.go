package esbuild

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/3-lines-studio/bifrost/internal/core"
	"github.com/3-lines-studio/sobek"
	"github.com/evanw/esbuild/pkg/api"
)

type Builder struct {
	mode      core.Mode
	ssrFormat api.Format

	tailwindMu sync.Mutex
	tailwind   map[string]*tailwindCompiler
}

// BuilderOption configures a Builder.
type BuilderOption func(*Builder)

// WithSSRFormat selects the SSR bundle format. The default is IIFE, which
// the Sobek runtime evaluates directly; ESM output is used by runtimes that
// support native modules.
func WithSSRFormat(format api.Format) BuilderOption {
	return func(b *Builder) { b.ssrFormat = format }
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
	runtime    *sobek.Runtime
	compile    sobek.Callable
	loadModule sobek.Value
}

func NewBuilder(mode core.Mode, opts ...BuilderOption) *Builder {
	b := &Builder{mode: mode, tailwind: make(map[string]*tailwindCompiler)}
	for _, opt := range opts {
		opt(b)
	}
	return b
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
		JSX:                 api.JSXAutomatic,
		Conditions:          []string{"browser", "style"},
		Plugins:             []api.Plugin{annotateTailwindPluginBases()},
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

func (b *Builder) BuildSSRRegistry(entrypoints []string, outdir string) (string, map[string]string, error) {
	if len(entrypoints) == 0 {
		return "", nil, fmt.Errorf("missing entrypoints")
	}
	if outdir == "" {
		return "", nil, fmt.Errorf("missing outdir")
	}
	if err := os.MkdirAll(outdir, 0o755); err != nil {
		return "", nil, fmt.Errorf("create SSR output directory: %w", err)
	}
	production := b.mode != core.ModeDev
	reactServerPlugin, err := legacyReactServerPlugin(entrypoints[0], production)
	if err != nil {
		return "", nil, err
	}

	var source strings.Builder
	exports := make(map[string]string, len(entrypoints))
	for i, entrypoint := range entrypoints {
		exportName := strings.TrimSuffix(filepath.Base(entrypoint), filepath.Ext(entrypoint))
		exports[entrypoint] = exportName
		fmt.Fprintf(&source, "var render%d;\n", i)
	}
	source.WriteString("export const loaders = {\n")
	for i, entrypoint := range entrypoints {
		fmt.Fprintf(
			&source,
			"%q: function() { return render%d || (render%d = require(%q).render); },\n",
			exports[entrypoint], i, i, entrypoint,
		)
	}
	source.WriteString("};\n")

	sourceMap := api.SourceMapInline
	if production {
		sourceMap = api.SourceMapNone
	}
	bundlePath := filepath.Join(outdir, "bifrost-ssr-registry.js")
	format := b.ssrFormat
	if format == 0 {
		format = api.FormatIIFE
	}
	options := api.BuildOptions{
		Stdin: &api.StdinOptions{
			Contents:   source.String(),
			ResolveDir: filepath.Dir(entrypoints[0]),
			Sourcefile: "bifrost-ssr-registry.js",
			Loader:     api.LoaderJS,
		},
		Outfile:           bundlePath,
		Bundle:            true,
		Write:             true,
		Platform:          api.PlatformBrowser,
		Format:            format,
		Target:            api.ES2015,
		Sourcemap:         sourceMap,
		JSX:               api.JSXAutomatic,
		Conditions:        []string{"browser"},
		Plugins:           []api.Plugin{ignoreCSSPlugin(), reactServerPlugin},
		MinifyWhitespace:  production,
		MinifyIdentifiers: production,
		MinifySyntax:      production,
		Define:            map[string]string{"process.env.NODE_ENV": quotedNodeEnv(production)},
		LogLevel:          api.LogLevelSilent,
	}
	if format == api.FormatIIFE {
		options.GlobalName = "__BIFROST_SSR__"
		options.Banner = map[string]string{"js": "/* bifrost:sobek-iife */"}
	}
	result := api.Build(options)
	if err := buildError("SSR registry build", result.Errors); err != nil {
		return "", nil, err
	}
	return bundlePath, exports, nil
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
	reactServerPlugin, err := legacyReactServerPlugin(entrypoints[0], production)
	if err != nil {
		return err
	}
	sourceMap := api.SourceMapInline
	if production {
		sourceMap = api.SourceMapNone
	}
	format := b.ssrFormat
	if format == 0 {
		format = api.FormatIIFE
	}
	options := api.BuildOptions{
		EntryPoints:       entrypoints,
		Outdir:            outdir,
		Bundle:            true,
		Write:             true,
		Platform:          api.PlatformBrowser,
		Format:            format,
		Target:            api.ES2015,
		Splitting:         false,
		Sourcemap:         sourceMap,
		JSX:               api.JSXAutomatic,
		Conditions:        []string{"browser"},
		Plugins:           []api.Plugin{ignoreCSSPlugin(), reactServerPlugin},
		EntryNames:        "[name]",
		MinifyWhitespace:  production,
		MinifyIdentifiers: production,
		MinifySyntax:      production,
		Define: map[string]string{
			"process.env.NODE_ENV": quotedNodeEnv(production),
		},
		LogLevel: api.LogLevelSilent,
	}
	if format == api.FormatIIFE {
		options.GlobalName = "__BIFROST_SSR__"
		options.Banner = map[string]string{"js": "/* bifrost:sobek-iife */"}
	}
	result := api.Build(options)
	return buildError("SSR build", result.Errors)
}

func legacyReactServerPlugin(entrypoint string, production bool) (api.Plugin, error) {
	packageRoot := findNodePackage(filepath.Dir(entrypoint), "react-dom")
	if packageRoot == "" {
		return api.Plugin{}, fmt.Errorf("node_modules/react-dom was not found for SSR build")
	}
	variant := "development"
	if production {
		variant = "production"
	}
	legacyRenderer := filepath.Join(packageRoot, "cjs", "react-dom-server-legacy.browser."+variant+".js")
	if _, err := os.Stat(legacyRenderer); err != nil {
		return api.Plugin{}, fmt.Errorf("find React legacy server renderer: %w", err)
	}
	return api.Plugin{
		Name: "bifrost-react-legacy-server",
		Setup: func(build api.PluginBuild) {
			build.OnResolve(api.OnResolveOptions{Filter: `^react-dom/server$`}, func(api.OnResolveArgs) (api.OnResolveResult, error) {
				return api.OnResolveResult{Path: legacyRenderer}, nil
			})
		},
	}, nil
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
		strings.Contains(text, "@utility") ||
		strings.Contains(text, "@plugin")
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
	if err := compiler.loadPlugins(css); err != nil {
		return "", err
	}
	value, err := compiler.compile(
		sobek.Undefined(),
		compiler.runtime.ToValue(css),
		compiler.runtime.ToValue(map[string]any{"loadModule": compiler.loadModule}),
	)
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
	if _, err := runtime.RunString(tailwindLoadModuleJS); err != nil {
		return nil, fmt.Errorf("define Tailwind loadModule in Sobek: %w", err)
	}
	if _, err := runtime.RunString(`if (typeof structuredClone !== "function") {
		structuredClone = function(value) { return JSON.parse(JSON.stringify(value)); };
	}`); err != nil {
		return nil, fmt.Errorf("define Tailwind structuredClone in Sobek: %w", err)
	}
	loadModule := runtime.Get("__BIFROST_TAILWIND_LOAD_MODULE__")
	if _, ok := sobek.AssertFunction(loadModule); !ok {
		return nil, fmt.Errorf("tailwind loadModule was not created")
	}
	return &tailwindCompiler{runtime: runtime, compile: compile, loadModule: loadModule}, nil
}

const tailwindLoadModuleJS = `__BIFROST_TAILWIND_LOAD_MODULE__ = function(id, base, kind) {
    var entry = __BIFROST_TAILWIND_PLUGINS__[id];
    if (entry === undefined) {
        throw new Error("bifrost: tailwind @plugin " + id + " not found in build");
    }
    return entry;
};`

const tailwindPluginRefPrefix = "bifrost-plugin:"

var (
	pluginDirectiveRe        = regexp.MustCompile(`@plugin\s*(["'])([^"']+)(["'])`)
	encodedPluginDirectiveRe = regexp.MustCompile(`@plugin\s*(["'])(bifrost-plugin:[A-Za-z0-9_-]+)(["'])`)
)

func annotateTailwindPluginBases() api.Plugin {
	return api.Plugin{
		Name: "bifrost-tailwind-plugin-bases",
		Setup: func(build api.PluginBuild) {
			build.OnLoad(api.OnLoadOptions{Filter: `\.css$`}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
				data, err := os.ReadFile(args.Path)
				if err != nil {
					return api.OnLoadResult{}, err
				}
				contents := rewriteTailwindPluginRefs(string(data), filepath.Dir(args.Path))
				return api.OnLoadResult{
					Contents:   &contents,
					Loader:     api.LoaderCSS,
					ResolveDir: filepath.Dir(args.Path),
				}, nil
			})
		},
	}
}

func rewriteTailwindPluginRefs(css, base string) string {
	var out strings.Builder
	for i := 0; i < len(css); {
		if strings.HasPrefix(css[i:], "/*") {
			end := strings.Index(css[i+2:], "*/")
			if end < 0 {
				out.WriteString(css[i:])
				break
			}
			end += i + 4
			out.WriteString(css[i:end])
			i = end
			continue
		}
		if css[i] == '"' || css[i] == '\'' {
			quote := css[i]
			start := i
			i++
			for i < len(css) {
				if css[i] == '\\' && i+1 < len(css) {
					i += 2
					continue
				}
				i++
				if css[i-1] == quote {
					break
				}
			}
			out.WriteString(css[start:i])
			continue
		}
		if strings.HasPrefix(css[i:], "@plugin") {
			match := pluginDirectiveRe.FindStringSubmatchIndex(css[i:])
			if match != nil && match[0] == 0 && css[i+match[2]:i+match[3]] == css[i+match[6]:i+match[7]] {
				id := css[i+match[4] : i+match[5]]
				ref := encodeTailwindPluginRef(base, id)
				out.WriteString(css[i : i+match[4]])
				out.WriteString(ref)
				out.WriteString(css[i+match[5] : i+match[1]])
				i += match[1]
				continue
			}
		}
		out.WriteByte(css[i])
		i++
	}
	return out.String()
}

func encodeTailwindPluginRef(base, id string) string {
	value := filepath.Clean(base) + "\x00" + id
	return tailwindPluginRefPrefix + base64.RawURLEncoding.EncodeToString([]byte(value))
}

func decodeTailwindPluginRef(ref string) (base, id string, err error) {
	if !strings.HasPrefix(ref, tailwindPluginRefPrefix) {
		return "", "", fmt.Errorf("tailwind plugin %q has no source base", ref)
	}
	value, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(ref, tailwindPluginRefPrefix))
	if err != nil {
		return "", "", fmt.Errorf("decode tailwind plugin reference: %w", err)
	}
	base, id, ok := strings.Cut(string(value), "\x00")
	if !ok || base == "" || id == "" {
		return "", "", fmt.Errorf("invalid tailwind plugin reference %q", ref)
	}
	return base, id, nil
}

func (c *tailwindCompiler) loadPlugins(css string) error {
	registry := c.runtime.NewObject()
	seen := make(map[string]bool)
	for _, match := range encodedPluginDirectiveRe.FindAllStringSubmatch(css, -1) {
		ref := match[2]
		if seen[ref] {
			continue
		}
		seen[ref] = true
		base, id, err := decodeTailwindPluginRef(ref)
		if err != nil {
			return err
		}
		plugin, entry, err := c.loadPluginModule(id, base)
		if err != nil {
			return err
		}
		loaded := c.runtime.NewObject()
		if err := loaded.Set("path", entry); err != nil {
			return err
		}
		if err := loaded.Set("base", filepath.Dir(entry)); err != nil {
			return err
		}
		if err := loaded.Set("module", plugin); err != nil {
			return err
		}
		if err := registry.Set(ref, loaded); err != nil {
			return err
		}
	}
	return c.runtime.Set("__BIFROST_TAILWIND_PLUGINS__", registry)
}

func (c *tailwindCompiler) loadPluginModule(id, base string) (sobek.Value, string, error) {
	sourcefile := "bifrost-tailwind-plugin.js"
	source := "import * as plugin from " + strconv.Quote(id) + ";" +
		"globalThis.__BIFROST_TAILWIND_PLUGIN__ = plugin.default ?? plugin;"
	result := api.Build(api.BuildOptions{
		AbsWorkingDir:     base,
		Stdin:             &api.StdinOptions{Contents: source, ResolveDir: base, Sourcefile: sourcefile, Loader: api.LoaderJS},
		Bundle:            true,
		Write:             false,
		Platform:          api.PlatformBrowser,
		MainFields:        []string{"browser", "module", "main"},
		Format:            api.FormatIIFE,
		Target:            api.ES2015,
		Metafile:          true,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		MinifySyntax:      true,
		LogLevel:          api.LogLevelSilent,
	})
	if err := buildError("Tailwind plugin bundle", result.Errors); err != nil {
		return nil, "", err
	}
	if len(result.OutputFiles) != 1 {
		return nil, "", fmt.Errorf("tailwind plugin bundle returned %d files", len(result.OutputFiles))
	}
	entry, err := tailwindPluginEntry(result.Metafile, sourcefile, base, id)
	if err != nil {
		return nil, "", err
	}
	if err := c.runtime.Set("__BIFROST_TAILWIND_PLUGIN__", sobek.Undefined()); err != nil {
		return nil, "", err
	}
	if _, err := c.runtime.RunString(string(result.OutputFiles[0].Contents)); err != nil {
		return nil, "", fmt.Errorf("load Tailwind plugin in Sobek: %w", err)
	}
	plugin := c.runtime.Get("__BIFROST_TAILWIND_PLUGIN__")
	if sobek.IsUndefined(plugin) || sobek.IsNull(plugin) {
		return nil, "", fmt.Errorf("tailwind plugin %q exported no value", id)
	}
	return plugin, entry, nil
}

func tailwindPluginEntry(metafileJSON, sourcefile, base, id string) (string, error) {
	var meta struct {
		Inputs map[string]struct {
			Imports []struct {
				Path     string `json:"path"`
				Original string `json:"original"`
			} `json:"imports"`
		} `json:"inputs"`
	}
	if err := json.Unmarshal([]byte(metafileJSON), &meta); err != nil {
		return "", fmt.Errorf("decode Tailwind plugin metadata: %w", err)
	}
	for path, input := range meta.Inputs {
		if filepath.Base(path) != sourcefile {
			continue
		}
		for _, imported := range input.Imports {
			if imported.Original == id {
				if filepath.IsAbs(imported.Path) {
					return filepath.Clean(imported.Path), nil
				}
				return filepath.Clean(filepath.Join(base, filepath.FromSlash(imported.Path))), nil
			}
		}
	}
	return "", fmt.Errorf("tailwind plugin %q has no resolved entry", id)
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
				if candidate != "" && !strings.ContainsAny(candidate, "{}") {
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
