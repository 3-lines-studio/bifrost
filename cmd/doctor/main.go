package main

import (
	"os"
	"path/filepath"

	"github.com/3-lines-studio/bifrost/internal/cli"
	"github.com/3-lines-studio/bifrost/internal/initcmd"
)

func main() {
	projectDir := "."
	if len(os.Args) > 1 {
		projectDir = os.Args[1]
	}

	absProjectDir, err := filepath.Abs(projectDir)
	if err != nil {
		cli.PrintHeader("Bifrost Doctor")
		cli.PrintError("Failed to resolve project directory: %v", err)
		os.Exit(1)
	}

	if err := initcmd.RepairBifrostDir(absProjectDir); err != nil {
		cli.PrintError("%v", err)
		os.Exit(1)
	}
}
