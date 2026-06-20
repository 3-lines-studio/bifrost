package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/3-lines-studio/bifrost/internal/adapters/cli"
	"github.com/3-lines-studio/bifrost/internal/adapters/fs"
)

func printDoctorUsage() {
	output := cli.NewOutput()
	output.PrintHeader("Bifrost Doctor")
	fmt.Println()
	fmt.Println("Usage: bifrost doctor [project-dir]")
	fmt.Println()
	fmt.Println("Repairs the .bifrost directory in the given project directory (default: .)")
}

func runDoctor(args []string) {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		printDoctorUsage()
		os.Exit(0)
	}

	projectDir := "."
	if len(args) > 0 {
		projectDir = args[0]
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
