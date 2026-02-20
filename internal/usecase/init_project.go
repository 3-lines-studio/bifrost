package usecase

import (
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/3-lines-studio/bifrost/internal/core"
)

type InitInput struct {
	ProjectDir string
	Template   string
	ModuleName string
}

type InitOutput struct {
	Success bool
	Error   error
}

type InitService struct {
	fs        FileReader
	fw        FileWriter
	templates TemplateSource
	cli       CLIOutput
}

func NewInitService(fs FileReader, fw FileWriter, templates TemplateSource, cli CLIOutput) *InitService {
	return &InitService{
		fs:        fs,
		fw:        fw,
		templates: templates,
		cli:       cli,
	}
}

func (s *InitService) InitProject(input InitInput) InitOutput {
	s.cli.PrintHeader("Bifrost Init")

	if s.fs.FileExists(input.ProjectDir) {
		entries, err := s.fs.ReadDir(input.ProjectDir)
		if err != nil {
			return InitOutput{
				Success: false,
				Error:   fmt.Errorf("failed to read directory: %w", err),
			}
		}

		hasFiles := false
		for _, entry := range entries {
			if entry.Name() != "." && entry.Name() != ".." {
				hasFiles = true
				break
			}
		}

		if hasFiles {
			return InitOutput{
				Success: false,
				Error:   fmt.Errorf("directory is not empty"),
			}
		}
	}

	s.cli.PrintSuccess("Project initialized")
	return InitOutput{
		Success: true,
	}
}

func (s *InitService) copyTemplateFiles(templateFS fs.FS, projectDir string, moduleName string) error {
	return fs.WalkDir(templateFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return s.fw.MkdirAll(filepath.Join(projectDir, path), 0755)
		}

		data, err := fs.ReadFile(templateFS, path)
		if err != nil {
			return err
		}

		newPath, isTemplate := core.ProcessFilename(path, core.TemplateData{Module: moduleName})
		data = core.ProcessContent(data, isTemplate, core.TemplateData{Module: moduleName})

		return s.fw.WriteFile(filepath.Join(projectDir, newPath), data, 0644)
	})
}
