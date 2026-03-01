package main

import (
	"context"
	"fmt"
	"os"
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

func parseFlags(args []string) (mainFile string, fw core.Framework, remaining []string) {
	fw = core.FrameworkReact

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--framework" || arg == "-f" {
			if i+1 < len(args) {
				fw = core.FrameworkFromString(strings.ToLower(args[i+1]))
				i++
			}
			continue
		}

		if after, ok := strings.CutPrefix(arg, "--framework="); ok {
			fw = core.FrameworkFromString(strings.ToLower(after))
			continue
		}

		if mainFile == "" && !strings.HasPrefix(arg, "-") {
			mainFile = arg
		} else {
			remaining = append(remaining, arg)
		}
	}

	return mainFile, fw, remaining
}

func getAdapter(fw core.Framework) core.FrameworkAdapter {
	switch fw {
	case core.FrameworkSvelte:
		return framework.NewSvelteAdapter()
	case core.FrameworkReact:
		return framework.NewReactAdapter()
	default:
		return framework.NewReactAdapter()
	}
}

func main() {
	mainFile, fw, _ := parseFlags(os.Args[1:])

	if mainFile == "" {
		output := cli.NewOutput()
		output.PrintHeader("Bifrost Build")
		output.PrintError("Missing main.go file argument")
		fmt.Println()
		output.PrintStep("", "Usage: bifrost-build [flags] <main.go>")
		output.PrintStep("", "Example: bifrost-build ./main.go")
		fmt.Println()
		output.PrintStep("", "Flags:")
		output.PrintStep("", "  -f, --framework <name>  Framework to use (react, svelte)")
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
	adapter := getAdapter(fw)

	runtime, err := process.NewRenderer(core.ModeDev, adapter.DevRendererSource())
	if err != nil {
		output.PrintHeader("Bifrost Build")
		output.PrintError("Failed to initialize build engine: %v", err)
		os.Exit(1)
	}
	defer func() { _ = runtime.Stop() }()

	buildService := usecase.NewBuildService(runtime, fsAdapter, output, adapter)

	input := usecase.BuildInput{
		MainFile:    mainFileAbs,
		OriginalCwd: goModRoot,
	}

	result := buildService.BuildProject(context.Background(), input)
	if result.Error != nil {
		output.PrintError("%v", result.Error)
		os.Exit(1)
	}

}
