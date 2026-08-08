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
	if config.optionFlags&optionLoader != 0 && config.PropsLoader == nil {
		return PageConfig{}, fmt.Errorf("bifrost: page %q has a nil props loader", route.ComponentPath)
	}
	if config.optionFlags&optionPreLoader != 0 && config.PreLoader == nil {
		return PageConfig{}, fmt.Errorf("bifrost: page %q has a nil pre loader", route.ComponentPath)
	}
	if config.optionFlags&optionStaticData != 0 && config.StaticDataLoader == nil {
		return PageConfig{}, fmt.Errorf("bifrost: page %q has a nil static data loader", route.ComponentPath)
	}
	if config.optionFlags&(optionStatic|optionStaticData) == optionStatic|optionStaticData {
		return PageConfig{}, fmt.Errorf("bifrost: page %q cannot use both WithStatic and WithStaticData", route.ComponentPath)
	}
	if config.optionFlags&optionLoader != 0 && config.Mode != ModeSSR {
		return PageConfig{}, fmt.Errorf("bifrost: page %q can only use WithLoader in SSR mode", route.ComponentPath)
	}
	if config.optionFlags&optionPreLoader != 0 && config.Mode != ModeSSR {
		return PageConfig{}, fmt.Errorf("bifrost: page %q can only use WithPreLoader in SSR mode", route.ComponentPath)
	}
	return config, nil
}
