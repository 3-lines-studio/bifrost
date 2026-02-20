package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/3-lines-studio/bifrost/internal/adapters/cli"
	"github.com/3-lines-studio/bifrost/internal/adapters/fs"
	"github.com/3-lines-studio/bifrost/internal/adapters/process"
	"github.com/3-lines-studio/bifrost/internal/core"
	"github.com/3-lines-studio/bifrost/internal/usecase"
)

func findGoModRoot(startDir string) string {
	dir := startDir
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return startDir
}

func main() {
	if len(os.Args) < 2 {
		output := cli.NewOutput()
		output.PrintHeader("Bifrost Build")
		output.PrintError("Missing main.go file argument")
		fmt.Println()
		output.PrintStep("", "Usage: bifrost-build <main.go>")
		output.PrintStep("", "Example: bifrost-build ./main.go")
		os.Exit(1)
	}

	mainFile := os.Args[1]

	originalCwd, err := os.Getwd()
	if err != nil {
		output := cli.NewOutput()
		output.PrintHeader("Bifrost Build")
		output.PrintError("Failed to get current working directory: %v", err)
		os.Exit(1)
	}

	mainFileAbs := mainFile
	if !filepath.IsAbs(mainFile) {
		mainFileAbs = filepath.Join(originalCwd, mainFile)
	}

	projectDir := filepath.Dir(mainFileAbs)
	goModRoot := findGoModRoot(projectDir)

	fsAdapter := fs.NewOSFileSystem()
	output := cli.NewOutput()

	runtime, err := process.NewRenderer(core.ModeDev)
	if err != nil {
		output.PrintHeader("Bifrost Build")
		output.PrintError("Failed to initialize build engine: %v", err)
		os.Exit(1)
	}
	defer func() { _ = runtime.Stop() }()

	buildService := usecase.NewBuildService(runtime, fsAdapter, output)

	input := usecase.BuildInput{
		MainFile:    mainFileAbs,
		OriginalCwd: goModRoot,
	}

	result := buildService.BuildProject(nil, input)
	if result.Error != nil {
		output.PrintError("%v", result.Error)
		os.Exit(1)
	}

	output.PrintDone("Build completed successfully")
}
