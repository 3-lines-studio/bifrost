package usecase

import "context"

type BuildInput struct {
	MainFile   string
	ModuleRoot string
	AppRoot    string
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
	cli      CLIOutput
}

func NewBuildService(renderer Renderer, cli CLIOutput) *BuildService {
	return &BuildService{
		renderer: renderer,
		cli:      cli,
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
	if err := s.copyPublicAssets(run); err != nil {
		return BuildOutput{Success: false, Error: err}
	}
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
