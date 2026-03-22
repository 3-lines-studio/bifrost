package core

// Route is a user-defined page route (pattern + component + options).
type Route struct {
	Pattern       string
	ComponentPath string
	Options       []PageOption
}

// Page builds a Route value (pattern, component path, optional PageOption values).
func Page(pattern string, componentPath string, opts ...PageOption) Route {
	return Route{
		Pattern:       pattern,
		ComponentPath: componentPath,
		Options:       opts,
	}
}

// PageConfigFromRoute applies options and returns the effective PageConfig (default mode SSR).
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
