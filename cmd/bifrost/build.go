package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/adapters/cli"
	esbuildadapter "github.com/3-lines-studio/bifrost/internal/adapters/esbuild"
	"github.com/3-lines-studio/bifrost/internal/adapters/fs"
	"github.com/3-lines-studio/bifrost/internal/adapters/process"
	quickjsrenderer "github.com/3-lines-studio/bifrost/internal/adapters/quickjs"
	"github.com/3-lines-studio/bifrost/internal/adapters/react"
	sobekrenderer "github.com/3-lines-studio/bifrost/internal/adapters/sobek"
	"github.com/3-lines-studio/bifrost/internal/core"
	"github.com/3-lines-studio/bifrost/internal/usecase"
	"github.com/evanw/esbuild/pkg/api"
)

//go:embed sobek-default.pgo
var sobekDefaultPGO []byte

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

func resolveSobekPGO(cwd string) (string, func(), error) {
	configured := strings.TrimSpace(os.Getenv("BIFROST_SOBEK_PGO"))
	if strings.EqualFold(configured, "off") || configured == "0" {
		return "", func() {}, nil
	}
	if configured != "" && configured != "1" && !strings.EqualFold(configured, "default") {
		path := configured
		if !filepath.IsAbs(path) {
			path = filepath.Join(cwd, path)
		}
		if _, err := os.Stat(path); err != nil {
			return "", func() {}, fmt.Errorf("configured profile %q: %w", path, err)
		}
		return path, func() {}, nil
	}

	profile, err := os.CreateTemp("", "bifrost-sobek-*.pgo")
	if err != nil {
		return "", func() {}, err
	}
	path := profile.Name()
	cleanup := func() { _ = os.Remove(path) }
	if _, err := profile.Write(sobekDefaultPGO); err != nil {
		_ = profile.Close()
		cleanup()
		return "", func() {}, err
	}
	if err := profile.Close(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return path, cleanup, nil
}

func runBuild(args []string) {
	os.Exit(runBuildCommand(args))
}

func runBuildCommand(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		printBuildUsage()
		return 0
	}

	mainFile, goBuildOutput, _ := parseFlags(args)

	if mainFile == "" {
		printBuildUsage()
		output := cli.NewOutput()
		output.PrintError("Missing main.go file argument")
		return 1
	}

	originalCwd, err := os.Getwd()
	if err != nil {
		output := cli.NewOutput()
		output.PrintHeader("Bifrost Build")
		output.PrintError("Failed to get current working directory: %v", err)
		return 1
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
		return 1
	}

	type buildRuntime interface {
		usecase.Renderer
		Stop() error
	}
	var runtime buildRuntime
	selectedRuntime := core.NormalizeJSRuntime(os.Getenv("BIFROST_JS_RUNTIME"))
	useSobekBuild := selectedRuntime == core.JSRuntimeSobek
	useInProcessBuild := selectedRuntime != core.JSRuntimeBun
	if useInProcessBuild {
		builder := esbuildadapter.NewBuilder(core.ModeProd)
		if useSobekBuild {
			runtime, err = sobekrenderer.NewRenderer(core.ModeProd, 0, builder)
		} else {
			esmBuilder := esbuildadapter.NewBuilder(core.ModeProd, esbuildadapter.WithSSRFormat(api.FormatESModule))
			runtime, err = quickjsrenderer.NewRenderer(core.ModeProd, 0, esmBuilder)
		}
	} else {
		runtime, err = process.NewRenderer(
			core.ModeDev,
			react.RuntimeSource(core.ModeDev),
			"BIFROST_PROD=1",
			"BIFROST_DEV=0",
		)
	}
	if err != nil {
		output.PrintHeader("Bifrost Build")
		output.PrintError("Failed to initialize build engine: %v", err)
		return 1
	}
	defer func() { _ = runtime.Stop() }()

	buildService := usecase.NewBuildService(runtime, output)

	input := usecase.BuildInput{
		MainFile:   mainFileAbs,
		ModuleRoot: goModRoot,
		AppRoot:    filepath.Dir(mainFileAbs),
	}

	result := buildService.BuildProject(context.Background(), input)
	if result.Error != nil {
		output.PrintError("%v", result.Error)
		return 1
	}
	if !result.Success {
		return 1
	}

	if goBuildOutput != "" {
		output.PrintStep("", "Running go build -o %s %s", goBuildOutput, mainFileAbs)
		goBuildOutputAbs := goBuildOutput
		if !filepath.IsAbs(goBuildOutput) {
			goBuildOutputAbs = filepath.Join(goModRoot, goBuildOutput)
		}
		if err := os.MkdirAll(filepath.Dir(goBuildOutputAbs), 0755); err != nil {
			output.PrintError("Failed to create output directory: %v", err)
			return 1
		}
		goBuildArgs := []string{"build"}
		pgoCleanup := func() {}
		if useSobekBuild {
			pgoPath, cleanup, pgoErr := resolveSobekPGO(originalCwd)
			if pgoErr != nil {
				output.PrintError("Failed to prepare Sobek PGO profile: %v", pgoErr)
				return 1
			}
			pgoCleanup = cleanup
			if pgoPath != "" {
				goBuildArgs = append(goBuildArgs, "-pgo="+pgoPath)
			}
		}
		defer pgoCleanup()
		goBuildArgs = append(goBuildArgs, "-o", goBuildOutputAbs, filepath.Dir(mainFileAbs))
		goBuild := exec.Command("go", goBuildArgs...)
		goBuild.Dir = goModRoot
		goBuild.Stdout = os.Stdout
		goBuild.Stderr = os.Stderr
		if err := goBuild.Run(); err != nil {
			output.PrintError("Go build failed: %v", err)
			return 1
		}
		output.PrintSuccess("Go binary built: %s", goBuildOutputAbs)
	}

	return 0
}
