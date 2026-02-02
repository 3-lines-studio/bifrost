package bifrost

import (
	"fmt"
	"os"
	"path/filepath"
)

func InitCmd() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: init <project-dir>\n")
		fmt.Fprintf(os.Stderr, "Example: init .\n")
		os.Exit(1)
	}

	projectDir := os.Args[1]
	if projectDir == "" {
		projectDir = "."
	}

	bifrostDir := filepath.Join(projectDir, ".bifrost")
	gitkeepPath := filepath.Join(bifrostDir, ".gitkeep")

	if err := os.MkdirAll(bifrostDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create .bifrost directory: %v\n", err)
		os.Exit(1)
	}

	if _, err := os.Stat(gitkeepPath); os.IsNotExist(err) {
		if err := os.WriteFile(gitkeepPath, []byte("# This file ensures .bifrost directory exists for go:embed\n"), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to create .gitkeep: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Created %s\n", gitkeepPath)
	} else {
		fmt.Printf("Already exists: %s\n", gitkeepPath)
	}

	fmt.Println("\nInitialization complete!")
	fmt.Println("You can now add the following to your main.go:")
	fmt.Println("")
	fmt.Println("  //go:embed all:.bifrost")
	fmt.Println("  var bifrostFS embed.FS")
	fmt.Println("")
	fmt.Println("  r, err := bifrost.New(bifrost.WithAssetsFS(bifrostFS))")
}
