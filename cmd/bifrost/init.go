package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/3-lines-studio/bifrost/internal/adapters/cli"
	"github.com/3-lines-studio/bifrost/internal/adapters/fs"
	"github.com/3-lines-studio/bifrost/internal/core"
	"github.com/3-lines-studio/bifrost/internal/usecase"
)

func runInit(args []string) {
	template := "minimal"
	var projectDir string

	if len(args) < 1 {
		printInitUsage()
		os.Exit(1)
	}

	if args[0] == "--help" || args[0] == "-h" {
		printInitUsage()
		os.Exit(0)
	}

	argIdx := 0
	for argIdx < len(args) {
		arg := args[argIdx]

		if arg == "--template" {
			if argIdx+1 >= len(args) {
				output := cli.NewOutput()
				output.PrintHeader("Bifrost Init")
				output.PrintError("--template requires a value")
				os.Exit(1)
			}
			template = args[argIdx+1]
			argIdx += 2
			continue
		}

		if projectDir == "" && !isFlag(arg) {
			projectDir = arg
		}
		argIdx++
	}

	if projectDir == "" {
		printInitUsage()
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

	initService := usecase.NewInitService(fsAdapter, output)

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
	output.PrintStep("", "Next steps:")
	fmt.Println()
	fmt.Printf("  cd %s\n", absProjectDir)
	fmt.Printf("  go mod tidy\n")
	fmt.Printf("  bun install\n")
	fmt.Printf("  bifrost dev ./main.go\n")
	fmt.Println()
}

func isFlag(arg string) bool {
	return len(arg) > 0 && arg[0] == '-'
}

func printInitUsage() {
	output := cli.NewOutput()
	output.PrintHeader("Bifrost Init")
	fmt.Println()
	fmt.Println("Usage: bifrost init [options] <project-dir>")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --template <name>  Template to use (minimal, spa). Default: minimal")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  bifrost init myapp")
	fmt.Println("  bifrost init --template spa myapp")
	fmt.Println()
	fmt.Println("To repair an existing project, use: bifrost doctor <dir>")
}
