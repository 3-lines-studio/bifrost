package core

type Renderer interface {
	Render(componentPath string, props map[string]any) (RenderedPage, error)
	Build(entrypoints []string, outdir string) error
}
