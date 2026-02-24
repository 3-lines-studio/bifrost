package main

import (
	"os"
	"path/filepath"

	"github.com/3-lines-studio/bifrost/internal/adapters/cli"
	"github.com/3-lines-studio/bifrost/internal/adapters/fs"
)

func main() {
	projectDir := "."
	if len(os.Args) > 1 {
		projectDir = os.Args[1]
	}

	absProjectDir, err := filepath.Abs(projectDir)
	if err != nil {
		output := cli.NewOutput()
		output.PrintHeader("Bifrost Doctor")
		output.PrintError("Failed to resolve project directory: %v", err)
		os.Exit(1)
	}

	output := cli.NewOutput()
	fsAdapter := fs.NewOSFileSystem()

	output.PrintHeader("Bifrost Doctor")

	bifrostDir := filepath.Join(absProjectDir, ".bifrost")
	if err := fsAdapter.MkdirAll(bifrostDir, 0755); err != nil {
		output.PrintError("Failed to create .bifrost directory: %v", err)
		os.Exit(1)
	}

	gitkeepPath := filepath.Join(bifrostDir, ".gitkeep")
	if !fsAdapter.FileExists(gitkeepPath) {
		if err := fsAdapter.WriteFile(gitkeepPath, []byte("# This file ensures .bifrost directory exists for go:embed\n"), 0644); err != nil {
			output.PrintError("Failed to create .gitkeep: %v", err)
			os.Exit(1)
		}
		output.PrintSuccess("Created %s", gitkeepPath)
	}

	output.PrintDone("Repair complete!")
}
