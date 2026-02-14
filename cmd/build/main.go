package main

import (
	"fmt"
	"os"

	"github.com/3-lines-studio/bifrost/internal/build"
	"github.com/3-lines-studio/bifrost/internal/cli"
)

func main() {
	if len(os.Args) < 2 {
		cli.PrintHeader("Bifrost Build")
		cli.PrintError("Missing main.go file argument")
		fmt.Println()
		cli.PrintInfo("Usage: bifrost-build <main.go>")
		cli.PrintStep(cli.EmojiInfo, "Example: bifrost-build ./main.go")
		os.Exit(1)
	}

	mainFile := os.Args[1]

	originalCwd, err := os.Getwd()
	if err != nil {
		cli.PrintHeader("Bifrost Build")
		cli.PrintError("Failed to get current working directory: %v", err)
		os.Exit(1)
	}

	engine, err := build.NewEngine()
	if err != nil {
		cli.PrintHeader("Bifrost Build")
		cli.PrintError("Failed to initialize build engine: %v", err)
		os.Exit(1)
	}
	defer engine.Close()

	if err := engine.BuildProject(mainFile, originalCwd); err != nil {
		cli.PrintError("%v", err)
		os.Exit(1)
	}
}
