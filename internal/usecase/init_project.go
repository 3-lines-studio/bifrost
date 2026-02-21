package usecase

import (
	"fmt"
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
	fs  FileSystem
	cli CLIOutput
}

func NewInitService(fs FileSystem, cli CLIOutput) *InitService {
	return &InitService{
		fs:  fs,
		cli: cli,
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
