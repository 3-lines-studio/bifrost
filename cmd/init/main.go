package main

import (
	"fmt"
	"os"

	"github.com/3-lines-studio/bifrost/internal/cli"
	"github.com/3-lines-studio/bifrost/internal/initcmd"
)

func main() {
	if len(os.Args) < 2 {
		cli.PrintHeader("Bifrost Init")
		cli.PrintError("Missing project directory argument")
		fmt.Println()
		cli.PrintInfo("Usage: bifrost-init <project-dir>")
		cli.PrintStep(cli.EmojiInfo, "Example: bifrost-init .")
		os.Exit(1)
	}

	projectDir := os.Args[1]
	if projectDir == "" {
		projectDir = "."
	}

	if err := initcmd.Run(projectDir); err != nil {
		cli.PrintError("%v", err)
		os.Exit(1)
	}
}
