package main

import (
	"reflect"
	"testing"

	"github.com/3-lines-studio/bifrost/internal/protocol"
)

func TestRouteTableFillsParamsAndKeepsGoRows(t *testing.T) {
	description := protocol.DescribeResult{Spec: protocol.Spec{Routes: []protocol.RouteSpec{
		{Pattern: "/{$}", Kind: "server", View: ".bifrost/views/page-0.tsx"},
		{Pattern: "/docs/{slug...}", Kind: "server", View: ".bifrost/views/page-1.tsx"},
	}}}
	params := map[string][]string{"/docs/{slug...}": {"slug"}}
	extra := []routeRow{{Kind: "api", Pattern: "GET /api/search", Source: "api/search/route.go"}}

	want := []routeRow{
		{Kind: "server", Pattern: "/{$}", Source: ".bifrost/views/page-0.tsx"},
		{Kind: "server", Pattern: "/docs/{slug...}", Source: ".bifrost/views/page-1.tsx", Params: []string{"slug"}},
		{Kind: "api", Pattern: "GET /api/search", Source: "api/search/route.go"},
	}
	if rows := routeTable(description, extra, params); !reflect.DeepEqual(rows, want) {
		t.Fatalf("routeTable() = %+v, want %+v", rows, want)
	}
}
