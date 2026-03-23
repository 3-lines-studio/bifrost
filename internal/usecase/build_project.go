package usecase

import (
	"context"

	"github.com/3-lines-studio/bifrost/internal/adapters/framework"
	"github.com/3-lines-studio/bifrost/internal/core"
)

type BuildInput struct {
	MainFile    string
	OriginalCwd string
}

type BuildOutput struct {
	Success bool
	Error   error
}

type BuildError struct {
	Page    string
	Message string
	Details []string
}

type BuildService struct {
	renderer Renderer
	fs       FileSystem
	cli      CLIOutput
	adapter  core.FrameworkAdapter
}

func NewBuildService(renderer Renderer, fs FileSystem, cli CLIOutput, adapter core.FrameworkAdapter) *BuildService {
	if adapter == nil {
		adapter = framework.DefaultAdapter()
	}
	return &BuildService{
		renderer: renderer,
		fs:       fs,
		cli:      cli,
		adapter:  adapter,
	}
}

func (s *BuildService) BuildProject(ctx context.Context, input BuildInput) BuildOutput {
	s.cli.PrintHeader("Bifrost Build")

	run, err := s.newBuildRun(input)
	if err != nil {
		return BuildOutput{
			Success: false,
			Error:   err,
		}
	}
	if err := s.createOutputDirs(run); err != nil {
		return BuildOutput{Success: false, Error: err}
	}
	s.copyPublicAssets(run)
	s.buildSSRBundles(run)
	s.generateClientEntries(run)
	s.buildClientAssets(run)
	s.populateCriticalCSS(ctx, run)
	s.generateClientOnlyHTML(run)
	if err := s.writeManifest(run); err != nil {
		return BuildOutput{Success: false, Error: err}
	}
	if err := s.compileRuntime(run); err != nil {
		return BuildOutput{Success: false, Error: err}
	}
	if err := s.exportStaticPrerender(ctx, run); err != nil {
		return BuildOutput{Success: false, Error: err}
	}
	s.cleanupEntryFiles(run)

	run.report.Render()
	return BuildOutput{Success: !run.report.HasFailures()}
}
