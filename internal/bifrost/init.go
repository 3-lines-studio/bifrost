package bifrost

import (
	"fmt"
	"os"
	"path/filepath"
)

func InitCmd() {
	if len(os.Args) < 2 {
		PrintHeader("Bifrost Init")
		PrintError("Missing project directory argument")
		fmt.Println()
		PrintInfo("Usage: bifrost-init <project-dir>")
		PrintStep(EmojiInfo, "Example: bifrost-init .")
		os.Exit(1)
	}

	projectDir := os.Args[1]
	if projectDir == "" {
		projectDir = "."
	}

	PrintHeader("Bifrost Init")

	bifrostDir := filepath.Join(projectDir, ".bifrost")
	gitkeepPath := filepath.Join(bifrostDir, ".gitkeep")

	PrintStep(EmojiFolder, "Creating .bifrost directory...")
	if err := os.MkdirAll(bifrostDir, 0755); err != nil {
		PrintError("Failed to create .bifrost directory: %v", err)
		os.Exit(1)
	}

	if _, err := os.Stat(gitkeepPath); os.IsNotExist(err) {
		if err := os.WriteFile(gitkeepPath, []byte("# This file ensures .bifrost directory exists for go:embed\n"), 0644); err != nil {
			PrintError("Failed to create .gitkeep: %v", err)
			os.Exit(1)
		}
		PrintSuccess("Created %s", gitkeepPath)
	} else {
		PrintWarning("Already exists: %s", gitkeepPath)
	}

	PrintDone("Initialization complete!")

	fmt.Println()
	PrintStep(EmojiInfo, "Next steps:")
	fmt.Println()
	fmt.Printf("  %s Add this to your main.go:\n", ColorCyan+"1."+ColorReset)
	fmt.Println()
	fmt.Printf("     %s//go:embed all:.bifrost%s\n", ColorGray, ColorReset)
	fmt.Printf("     %svar bifrostFS embed.FS%s\n", ColorGray, ColorReset)
	fmt.Println()
	fmt.Printf("  %s Initialize Bifrost with:\n", ColorCyan+"2."+ColorReset)
	fmt.Println()
	fmt.Printf("     %sr, err := bifrost.New(%s\n", ColorGray, ColorReset)
	fmt.Printf("         %sbifrost.WithAssetsFS(bifrostFS),%s\n", ColorGray, ColorReset)
	fmt.Printf("     %s)%s\n", ColorGray, ColorReset)
	fmt.Println()
}
