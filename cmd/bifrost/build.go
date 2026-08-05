package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/adapters/cli"
	"github.com/3-lines-studio/bifrost/internal/adapters/framework"
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

func parseFlags(args []string) (mainFile string, goBuildOutput string, remaining []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--go-build" {
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				goBuildOutput = args[i+1]
				i++
			} else {
				goBuildOutput = "./tmp/app"
			}
			continue
		}

		if after, ok := strings.CutPrefix(arg, "--go-build="); ok {
			goBuildOutput = after
			continue
		}

		if mainFile == "" && !strings.HasPrefix(arg, "-") {
			mainFile = arg
		} else {
			remaining = append(remaining, arg)
		}
	}

	return mainFile, goBuildOutput, remaining
}

func printBuildUsage() {
	output := cli.NewOutput()
	output.PrintHeader("Bifrost Build")
	fmt.Println()
	fmt.Println("Usage: bifrost build <main.go> [flags]")
	fmt.Println("Examples:")
	fmt.Println("  bifrost build ./main.go")
	fmt.Println("  bifrost build ./main.go --go-build")
	fmt.Println("  bifrost build ./main.go --go-build=./myapp")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  --go-build[=path]  Run go build after asset build (default: ./tmp/app)")
}

func ensureBifrostDir(fsAdapter fs.FileSystem, dir string) error {
	if err := fsAdapter.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	gitkeep := filepath.Join(dir, ".gitkeep")
	if !fsAdapter.FileExists(gitkeep) {
		if err := fsAdapter.WriteFile(gitkeep, []byte("# This file ensures .bifrost directory exists for go:embed\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func runBuild(args []string) {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		printBuildUsage()
		os.Exit(0)
	}

	mainFile, goBuildOutput, _ := parseFlags(args)

	if mainFile == "" {
		printBuildUsage()
		output := cli.NewOutput()
		output.PrintError("Missing main.go file argument")
		os.Exit(1)
	}

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

	bifrostDir := filepath.Join(projectDir, ".bifrost")
	if err := ensureBifrostDir(fsAdapter, bifrostDir); err != nil {
		output.PrintHeader("Bifrost Build")
		output.PrintError("Failed to prepare .bifrost directory: %v", err)
		os.Exit(1)
	}

	adapter := framework.DefaultAdapter()

	runtime, err := process.NewRenderer(core.ModeDev, framework.RuntimeSource(core.ModeDev, core.FrameworkReact), "BIFROST_PROD=1")
	if err != nil {
		output.PrintHeader("Bifrost Build")
		output.PrintError("Failed to initialize build engine: %v", err)
		os.Exit(1)
	}
	defer func() { _ = runtime.Stop() }()

	buildService := usecase.NewBuildService(runtime, fsAdapter, output, adapter)

	input := usecase.BuildInput{
		MainFile:   mainFileAbs,
		ModuleRoot: goModRoot,
		AppRoot:    filepath.Dir(mainFileAbs),
	}

	result := buildService.BuildProject(context.Background(), input)
	if result.Error != nil {
		output.PrintError("%v", result.Error)
		os.Exit(1)
	}
	if !result.Success {
		os.Exit(1)
	}

	if goBuildOutput != "" {
		output.PrintStep("", "Running go build -o %s %s", goBuildOutput, mainFileAbs)
		goBuildOutputAbs := goBuildOutput
		if !filepath.IsAbs(goBuildOutput) {
			goBuildOutputAbs = filepath.Join(goModRoot, goBuildOutput)
		}
		if err := os.MkdirAll(filepath.Dir(goBuildOutputAbs), 0755); err != nil {
			output.PrintError("Failed to create output directory: %v", err)
			os.Exit(1)
		}
		goBuild := exec.Command("go", "build", "-o", goBuildOutputAbs, filepath.Dir(mainFileAbs))
		goBuild.Dir = goModRoot
		goBuild.Stdout = os.Stdout
		goBuild.Stderr = os.Stderr
		if err := goBuild.Run(); err != nil {
			output.PrintError("Go build failed: %v", err)
			os.Exit(1)
		}
		output.PrintSuccess("Go binary built: %s", goBuildOutputAbs)
	}

}
