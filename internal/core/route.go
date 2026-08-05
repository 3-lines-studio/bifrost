package core

import (
	"fmt"
	"strings"
)

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
	if route.Pattern == "" {
		return PageConfig{}, fmt.Errorf("bifrost: page pattern cannot be empty")
	}
	if strings.TrimSpace(route.ComponentPath) == "" {
		return PageConfig{}, fmt.Errorf("bifrost: component path cannot be empty for pattern %q", route.Pattern)
	}

	config := PageConfig{
		ComponentPath: route.ComponentPath,
		Mode:          ModeSSR,
	}
	for _, opt := range route.Options {
		if opt == nil {
			return PageConfig{}, fmt.Errorf("bifrost: page %q has a nil option", route.ComponentPath)
		}
		opt(&config)
	}
	if config.modeOptions == 3 {
		return PageConfig{}, fmt.Errorf("bifrost: page %q has conflicting mode options", route.ComponentPath)
	}
	return config, nil
}
