package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/3-lines-studio/bifrost/internal/cli"
	"github.com/3-lines-studio/bifrost/internal/initcmd"
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
				cli.PrintHeader("Bifrost Init")
				cli.PrintError("--template requires a value")
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
		cli.PrintHeader("Bifrost Init")
		cli.PrintError("Failed to resolve project directory: %v", err)
		os.Exit(1)
	}

	if err := initcmd.Run(absProjectDir, template); err != nil {
		cli.PrintError("%v", err)
		os.Exit(1)
	}
}

func isFlag(arg string) bool {
	return len(arg) > 0 && arg[0] == '-'
}

func printUsage() {
	cli.PrintHeader("Bifrost Init")
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
