package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/3-lines-studio/bifrost/internal/adapters"
	"github.com/3-lines-studio/bifrost/internal/adapters/cli"
	"github.com/3-lines-studio/bifrost/internal/adapters/fs"
	"github.com/3-lines-studio/bifrost/internal/core"
	"github.com/3-lines-studio/bifrost/internal/usecase"
)

func main() {
	template := "minimal"
	var projectDir string

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	if os.Args[1] == "--help" || os.Args[1] == "-h" {
		printUsage()
		os.Exit(0)
	}

	argIdx := 1
	for argIdx < len(os.Args) {
		arg := os.Args[argIdx]

		if arg == "--template" {
			if argIdx+1 >= len(os.Args) {
				output := cli.NewOutput()
				output.PrintHeader("Bifrost Init")
				output.PrintError("--template requires a value")
				os.Exit(1)
			}
			template = os.Args[argIdx+1]
			argIdx += 2
			continue
		}

		if projectDir == "" && !isFlag(arg) {
			projectDir = arg
		}
		argIdx++
	}

	if projectDir == "" {
		printUsage()
		os.Exit(1)
	}

	absProjectDir, err := filepath.Abs(projectDir)
	if err != nil {
		output := cli.NewOutput()
		output.PrintHeader("Bifrost Init")
		output.PrintError("Failed to resolve project directory: %v", err)
		os.Exit(1)
	}

	fsAdapter := fs.NewOSFileSystem()
	output := cli.NewOutput()
	templateAdapter := adapters.NewTemplateSource()

	initService := usecase.NewInitService(fsAdapter, fsAdapter, templateAdapter, output)

	input := usecase.InitInput{
		ProjectDir: absProjectDir,
		Template:   template,
		ModuleName: core.DeriveModuleName(absProjectDir),
	}

	result := initService.InitProject(input)
	if result.Error != nil {
		output.PrintError("%v", result.Error)
		os.Exit(1)
	}

	fmt.Println()
	output.PrintStep(cli.EmojiInfo, "Next steps:")
	fmt.Println()
	fmt.Printf("  # Install air\n")
	fmt.Printf("  go install github.com/air-verse/air@latest\n")
	fmt.Printf("  cd %s\n", absProjectDir)
	fmt.Printf("  go mod tidy\n")
	fmt.Printf("  bun install\n")
	fmt.Printf("  make dev\n")
	fmt.Println()
}

func isFlag(arg string) bool {
	return len(arg) > 0 && arg[0] == '-'
}

func printUsage() {
	output := cli.NewOutput()
	output.PrintHeader("Bifrost Init")
	fmt.Println()
	fmt.Println("Usage: bifrost-init [options] <project-dir>")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --template <name>  Template to use (minimal, spa, desktop). Default: minimal")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  bifrost-init myapp")
	fmt.Println("  bifrost-init --template spa myapp")
	fmt.Println("  bifrost-init --template desktop myapp")
	fmt.Println()
	fmt.Println("To repair an existing project, use: bifrost-doctor <dir>")
}
