package app

import (
	"fmt"
	"os"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/core"
)

func shouldPrintRouteTable() bool {
	if os.Getenv("BIFROST_NO_ROUTE_TABLE") == "1" {
		return false
	}
	if os.Getenv("BIFROST_ROUTE_TABLE") == "1" {
		return true
	}
	return isStdoutTerminal()
}

func isStdoutTerminal() bool {
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) == os.ModeCharDevice
}

func printRouteTable(routes []core.Route, configs []core.PageConfig) {
	if len(routes) == 0 {
		return
	}

	type row struct {
		pattern   string
		component string
		mode      string
	}

	rows := make([]row, len(routes))
	maxPattern := len("PATTERN")
	maxComponent := len("COMPONENT")
	maxMode := len("MODE")

	for i, route := range routes {
		modeLabel := configs[i].Mode.BuildLabel()
		rows[i] = row{
			pattern:   route.Pattern,
			component: route.ComponentPath,
			mode:      modeLabel,
		}
		if len(route.Pattern) > maxPattern {
			maxPattern = len(route.Pattern)
		}
		if len(route.ComponentPath) > maxComponent {
			maxComponent = len(route.ComponentPath)
		}
		if len(modeLabel) > maxMode {
			maxMode = len(modeLabel)
		}
	}

	fmt.Println()
	fmt.Println("Bifrost routes:")
	fmt.Printf("  %-*s  %-*s  %-*s\n", maxPattern, "PATTERN", maxComponent, "COMPONENT", maxMode, "MODE")
	fmt.Printf("  %s  %s  %s\n",
		strings.Repeat("-", maxPattern),
		strings.Repeat("-", maxComponent),
		strings.Repeat("-", maxMode))

	for _, r := range rows {
		fmt.Printf("  %-*s  %-*s  %-*s\n", maxPattern, r.pattern, maxComponent, r.component, maxMode, r.mode)
	}
}
