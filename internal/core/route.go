package core

import "fmt"

type Route struct {
	Pattern       string
	ComponentPath string
	Options       []PageOption
}

func Page(pattern string, componentPath string, opts ...PageOption) Route {
	return Route{
		Pattern:       pattern,
		ComponentPath: componentPath,
		Options:       opts,
	}
}

func PageConfigFromRoute(route Route) (PageConfig, error) {
	config := PageConfig{
		ComponentPath: route.ComponentPath,
		Mode:          ModeSSR,
	}
	for _, opt := range route.Options {
		opt(&config)
	}
	if config.modeOptions == 3 {
		return PageConfig{}, fmt.Errorf("bifrost: page %q has conflicting mode options", route.ComponentPath)
	}
	return config, nil
}
