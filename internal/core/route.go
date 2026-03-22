package core

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

func PageConfigFromRoute(route Route) PageConfig {
	config := PageConfig{
		ComponentPath: route.ComponentPath,
		Mode:          ModeSSR,
	}
	for _, opt := range route.Options {
		opt(&config)
	}
	return config
}
