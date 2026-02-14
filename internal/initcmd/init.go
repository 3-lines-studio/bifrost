package initcmd

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/cli"
)

type Options struct {
	Template   string // "static", "full", "desktop"
	ModuleName string // Go module name
	ScaffoldFS fs.FS  // FS rooted at example/ directory
}

// sharedFiles are copied from example/ for all templates.
var sharedFiles = []string{
	"pages/about.tsx",
	"components/hello.tsx",
	"components/mode-toggle.tsx",
	"components/theme-provider.tsx",
	"components/ui/button.tsx",
	"components/ui/dropdown-menu.tsx",
	"layout/base.tsx",
	"lib/theme-script.ts",
	"lib/utils.ts",
	"public/favicon.ico",
	"public/icon.png",
	"style.css",
	"package.json",
	"tsconfig.json",
	"components.json",
	".air.toml",
	".gitignore",
}

// fullExtraFiles are additional pages only included in the full template.
var fullExtraFiles = []string{
	"pages/home.tsx",
	"pages/blog.tsx",
	"pages/nested/page.tsx",
}

// cmdFiles maps template name to the example/cmd/ entrypoint.
var cmdFiles = map[string]string{
	"static":  "cmd/static/main.go",
	"full":    "cmd/full/main.go",
	"desktop": "cmd/desktop/main.go",
}

func Run(projectDir string, opts Options) error {
	cli.PrintHeader("Bifrost Init")

	tmpl := opts.Template
	if tmpl == "" {
		tmpl = "static"
	}
	if tmpl != "static" && tmpl != "full" && tmpl != "desktop" {
		return fmt.Errorf("unknown template %q (choose: static, full, desktop)", tmpl)
	}

	moduleName := opts.ModuleName
	if moduleName == "" {
		abs, err := filepath.Abs(projectDir)
		if err != nil {
			return fmt.Errorf("failed to resolve project directory: %w", err)
		}
		moduleName = filepath.Base(abs)
	}

	cli.PrintStep(cli.EmojiPackage, "Scaffolding %s%s%s template in %s%s%s",
		cli.ColorCyan, tmpl, cli.ColorReset,
		cli.ColorCyan, projectDir, cli.ColorReset)
	fmt.Println()

	// Create project directory
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return fmt.Errorf("failed to create project directory: %w", err)
	}

	// 1. Copy shared frontend/config files
	cli.PrintStep(cli.EmojiCopy, "Copying frontend files...")
	for _, f := range sharedFiles {
		if err := copyFromFS(opts.ScaffoldFS, f, filepath.Join(projectDir, f)); err != nil {
			return fmt.Errorf("failed to copy %s: %w", f, err)
		}
		cli.PrintFile(f)
	}

	// 2. Copy extra files for full template
	if tmpl == "full" {
		for _, f := range fullExtraFiles {
			if err := copyFromFS(opts.ScaffoldFS, f, filepath.Join(projectDir, f)); err != nil {
				return fmt.Errorf("failed to copy %s: %w", f, err)
			}
			cli.PrintFile(f)
		}
	}

	// 3. Transform and write main.go
	cli.PrintStep(cli.EmojiFile, "Generating Go files...")
	srcPath := cmdFiles[tmpl]
	mainGoContent, err := fs.ReadFile(opts.ScaffoldFS, srcPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", srcPath, err)
	}
	transformed := transformMainGo(string(mainGoContent), tmpl)
	if err := writeFile(filepath.Join(projectDir, "main.go"), []byte(transformed)); err != nil {
		return fmt.Errorf("failed to write main.go: %w", err)
	}
	cli.PrintFile("main.go")

	// 4. Generate embed.go
	embedContent := generateEmbedGo(tmpl)
	if err := writeFile(filepath.Join(projectDir, "embed.go"), []byte(embedContent)); err != nil {
		return fmt.Errorf("failed to write embed.go: %w", err)
	}
	cli.PrintFile("embed.go")

	// 5. Generate Makefile
	makefileContent := generateMakefile(tmpl)
	if err := writeFile(filepath.Join(projectDir, "Makefile"), []byte(makefileContent)); err != nil {
		return fmt.Errorf("failed to write Makefile: %w", err)
	}
	cli.PrintFile("Makefile")

	// 6. Create .bifrost/.gitkeep
	bifrostDir := filepath.Join(projectDir, ".bifrost")
	if err := os.MkdirAll(bifrostDir, 0755); err != nil {
		return fmt.Errorf("failed to create .bifrost directory: %w", err)
	}
	if err := writeFile(filepath.Join(bifrostDir, ".gitkeep"), []byte("# This file ensures .bifrost directory exists for go:embed\n")); err != nil {
		return fmt.Errorf("failed to create .gitkeep: %w", err)
	}
	cli.PrintFile(".bifrost/.gitkeep")

	fmt.Println()

	// 7. Run go mod init + go mod tidy
	cli.PrintStep(cli.EmojiGear, "Initializing Go module...")
	if err := runCmd(projectDir, "go", "mod", "init", moduleName); err != nil {
		return fmt.Errorf("go mod init failed: %w", err)
	}

	// Add bifrost dependency
	if err := runCmd(projectDir, "go", "get", "github.com/3-lines-studio/bifrost@latest"); err != nil {
		cli.PrintWarning("go get bifrost failed (you may need to add the dependency manually): %v", err)
	}

	// Add template-specific dependencies
	switch tmpl {
	case "full", "static":
		if err := runCmd(projectDir, "go", "get", "github.com/go-chi/chi/v5@latest"); err != nil {
			cli.PrintWarning("go get chi failed: %v", err)
		}
	case "desktop":
		if err := runCmd(projectDir, "go", "get", "github.com/go-chi/chi/v5@latest"); err != nil {
			cli.PrintWarning("go get chi failed: %v", err)
		}
		if err := runCmd(projectDir, "go", "get", "github.com/webview/webview_go@latest"); err != nil {
			cli.PrintWarning("go get webview failed: %v", err)
		}
		if err := runCmd(projectDir, "go", "get", "github.com/getlantern/systray@latest"); err != nil {
			cli.PrintWarning("go get systray failed: %v", err)
		}
	}

	if err := runCmd(projectDir, "go", "mod", "tidy"); err != nil {
		cli.PrintWarning("go mod tidy failed: %v", err)
	}

	// 8. Run bun install
	cli.PrintStep(cli.EmojiPackage, "Installing frontend dependencies...")
	if err := runCmd(projectDir, "bun", "install"); err != nil {
		cli.PrintWarning("bun install failed (you can run it manually later): %v", err)
	}

	cli.PrintDone("Project scaffolded successfully!")

	fmt.Println()
	cli.PrintStep(cli.EmojiRocket, "Next steps:")
	fmt.Println()
	fmt.Printf("  %scd %s%s\n", cli.ColorCyan, projectDir, cli.ColorReset)
	fmt.Printf("  %smake dev%s\n", cli.ColorCyan, cli.ColorReset)
	fmt.Println()

	return nil
}

func transformMainGo(content string, tmpl string) string {
	// Remove the example package import line
	content = removeImportLine(content, `"github.com/3-lines-studio/bifrost/example"`)

	// Replace example.BifrostFS → bifrostFS
	content = strings.ReplaceAll(content, "example.BifrostFS", "bifrostFS")

	// Replace example.IconPNG → iconPNG (desktop)
	if tmpl == "desktop" {
		content = strings.ReplaceAll(content, "example.IconPNG", "iconPNG")
	}

	return content
}

func removeImportLine(content string, importPath string) string {
	lines := strings.Split(content, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == importPath {
			continue
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

func generateEmbedGo(tmpl string) string {
	var b strings.Builder
	b.WriteString("package main\n\nimport \"embed\"\n\n")
	b.WriteString("//go:embed all:.bifrost\n")
	b.WriteString("var bifrostFS embed.FS\n")
	if tmpl == "desktop" {
		b.WriteString("\n//go:embed public/icon.png\n")
		b.WriteString("var iconPNG []byte\n")
	}
	return b.String()
}

func generateMakefile(tmpl string) string {
	var b strings.Builder

	b.WriteString("dev:\n")
	switch tmpl {
	case "desktop":
		b.WriteString("\tBIFROST_DEV=1 go run .\n")
	default:
		b.WriteString("\tBIFROST_DEV=1 air -c .air.toml --build.cmd \"go build -o ./tmp/main .\"\n")
	}
	b.WriteString("\n")

	b.WriteString("build:\n")
	switch tmpl {
	case "desktop":
		b.WriteString("\tgo run github.com/3-lines-studio/bifrost/cmd/build@latest main.go\n")
		b.WriteString("\tgo build -o app-desktop .\n")
	default:
		b.WriteString("\tgo run github.com/3-lines-studio/bifrost/cmd/build@latest main.go\n")
		b.WriteString("\tgo build -o app .\n")
	}
	b.WriteString("\n")

	b.WriteString("start: build\n")
	switch tmpl {
	case "desktop":
		b.WriteString("\t./app-desktop\n")
	default:
		b.WriteString("\t./app\n")
	}

	return b.String()
}

func copyFromFS(srcFS fs.FS, srcPath, dstPath string) error {
	data, err := fs.ReadFile(srcFS, srcPath)
	if err != nil {
		return err
	}
	return writeFile(dstPath, data)
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func runCmd(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
