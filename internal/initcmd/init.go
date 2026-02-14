package initcmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/3-lines-studio/bifrost/internal/cli"
)

func Run(projectDir string) error {
	cli.PrintHeader("Bifrost Init")

	bifrostDir := filepath.Join(projectDir, ".bifrost")
	gitkeepPath := filepath.Join(bifrostDir, ".gitkeep")

	cli.PrintStep(cli.EmojiFolder, "Creating .bifrost directory...")
	if err := os.MkdirAll(bifrostDir, 0755); err != nil {
		return fmt.Errorf("failed to create .bifrost directory: %w", err)
	}

	if _, err := os.Stat(gitkeepPath); os.IsNotExist(err) {
		if err := os.WriteFile(gitkeepPath, []byte("# This file ensures .bifrost directory exists for go:embed\n"), 0644); err != nil {
			return fmt.Errorf("failed to create .gitkeep: %w", err)
		}
		cli.PrintSuccess("Created %s", gitkeepPath)
	} else {
		cli.PrintWarning("Already exists: %s", gitkeepPath)
	}

	cli.PrintDone("Initialization complete!")

	fmt.Println()
	cli.PrintStep(cli.EmojiInfo, "Next steps:")
	fmt.Println()
	fmt.Printf("  %s Add this to your main.go:\n", cli.ColorCyan+"1."+cli.ColorReset)
	fmt.Println()
	fmt.Printf("     %s//go:embed all:.bifrost%s\n", cli.ColorGray, cli.ColorReset)
	fmt.Printf("     %svar bifrostFS embed.FS%s\n", cli.ColorGray, cli.ColorReset)
	fmt.Println()
	fmt.Printf("  %s Initialize Bifrost with:\n", cli.ColorCyan+"2."+cli.ColorReset)
	fmt.Println()
	fmt.Printf("     %sr, err := bifrost.New(%s\n", cli.ColorGray, cli.ColorReset)
	fmt.Printf("         %sbifrost.WithAssetsFS(bifrostFS),%s\n", cli.ColorGray, cli.ColorReset)
	fmt.Printf("     %s)%s\n", cli.ColorGray, cli.ColorReset)
	fmt.Println()

	return nil
}
