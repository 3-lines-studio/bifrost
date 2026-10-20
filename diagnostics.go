package bifrost

import "slices"

type RouteInfo struct {
	Pattern string
	View    string
	Kind    string
}

type Diagnostics struct {
	SpecHash   string
	Routes     []RouteInfo
	AppPlugins []string
	Production bool
}

func (a *App) Diagnostics() Diagnostics {
	if a == nil {
		return Diagnostics{}
	}
	routes := make([]RouteInfo, len(a.routes))
	for i, route := range a.routes {
		routes[i] = RouteInfo{Pattern: route.pattern, View: route.view, Kind: route.kind.String()}
	}
	return Diagnostics{
		SpecHash:   a.specHash,
		Routes:     routes,
		AppPlugins: slices.Clone(a.appPlugins),
		Production: a.runtime != nil,
	}
}
