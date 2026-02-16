package initcmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/3-lines-studio/bifrost/internal/cli"
	"github.com/3-lines-studio/bifrost/internal/templates"
)

func Run(projectDir string, templateName string) error {
	cli.PrintHeader("Bifrost Init")

	if _, err := os.Stat(projectDir); err == nil {
		entries, err := os.ReadDir(projectDir)
		if err != nil {
			return fmt.Errorf("failed to read directory: %w", err)
		}
		if len(entries) > 0 {
			return fmt.Errorf("directory '%s' already exists and is not empty", projectDir)
		}
	}

	templateFS, err := templates.GetTemplate(templateName)
	if err != nil {
		if errors.Is(err, templates.ErrInvalidTemplate) {
			return fmt.Errorf("invalid template '%s'", templateName)
		}
		return err
	}

	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return fmt.Errorf("failed to create project directory: %w", err)
	}

	moduleName := templates.DeriveModuleName(projectDir)

	data := templates.TemplateData{
		Module: moduleName,
	}

	createdCount := 0

	err = fs.WalkDir(templateFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			targetDir := filepath.Join(projectDir, path)
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", targetDir, err)
			}
			return nil
		}

		content, err := fs.ReadFile(templateFS, path)
		if err != nil {
			return fmt.Errorf("failed to read template file %s: %w", path, err)
		}

		targetPath, isTemplate := templates.ProcessFilename(path, data)
		targetPath = filepath.Join(projectDir, targetPath)

		processedContent := templates.ProcessContent(content, isTemplate, data)

		targetDir := filepath.Dir(targetPath)
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", targetDir, err)
		}

		if err := os.WriteFile(targetPath, processedContent, 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", targetPath, err)
		}

		if isTemplate {
			cli.PrintFile(targetPath + " (generated)")
		} else {
			cli.PrintFile(targetPath)
		}
		createdCount++

		return nil
	})

	if err != nil {
		return err
	}

	if err := ensureBifrostDir(projectDir); err != nil {
		return err
	}
	createdCount++

	cli.PrintDone("Created %d files using '%s' template", createdCount, templateName)

	fmt.Println()
	cli.PrintStep(cli.EmojiInfo, "Next steps:")
	fmt.Println()
	fmt.Printf("  # Install air\n")
	fmt.Printf("  go install github.com/air-verse/air@latest\n")
	fmt.Printf("  cd %s\n", projectDir)
	fmt.Printf("  go mod tidy\n")
	fmt.Printf("  bun install\n")
	fmt.Printf("  make dev\n")
	fmt.Println()

	return nil
}

func RepairBifrostDir(projectDir string) error {
	cli.PrintHeader("Bifrost Doctor")

	if err := ensureBifrostDir(projectDir); err != nil {
		return err
	}

	cli.PrintDone("Repair complete!")

	return nil
}

func ensureBifrostDir(projectDir string) error {
	bifrostDir := filepath.Join(projectDir, ".bifrost")
	gitkeepPath := filepath.Join(bifrostDir, ".gitkeep")

	if err := os.MkdirAll(bifrostDir, 0755); err != nil {
		return fmt.Errorf("failed to create .bifrost directory: %w", err)
	}

	if _, err := os.Stat(gitkeepPath); os.IsNotExist(err) {
		if err := os.WriteFile(gitkeepPath, []byte("# This file ensures .bifrost directory exists for go:embed\n"), 0644); err != nil {
			return fmt.Errorf("failed to create .gitkeep: %w", err)
		}
		cli.PrintSuccess("Created %s", gitkeepPath)
	}

	return nil
}
