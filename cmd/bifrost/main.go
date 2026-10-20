package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/3-lines-studio/bifrost"
	"github.com/3-lines-studio/bifrost/internal/builder"
	"github.com/3-lines-studio/bifrost/internal/protocol"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "build":
		err = runBuild(os.Args[2:])
	case "dev":
		err = runDev(os.Args[2:])
	case "init":
		err = runInit(os.Args[2:])
	case "routes":
		err = runRoutes(os.Args[2:])
	case "version":
		fmt.Println(bifrost.Version)
	case "help", "-h", "--help":
		usage()
		return
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		reportError(err)
		os.Exit(1)
	}
}

func reportError(err error) {
	message := err.Error()
	if strings.HasPrefix(message, "bifrost:") {
		fmt.Fprintln(os.Stderr, message)
		return
	}
	fmt.Fprintln(os.Stderr, "bifrost:", message)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: bifrost <build|dev|init|routes|version> [options]")
}

func runBuild(args []string) error {
	flags := flag.NewFlagSet("build", flag.ContinueOnError)
	output := flags.String("output", "", "build output directory")
	dir := flags.String("C", ".", "working directory")
	staticWorkers := flags.Int("static-workers", 4, "concurrent static render workers")
	renderConcurrency := flags.Int("render-concurrency", 0, "production renderer processes (default two)")
	renderQueue := flags.Int("render-queue", 0, "requests waiting on renderer capacity (default 64)")
	sourceMaps := flags.Bool("sourcemaps", false, "include inline production source maps")
	viteConfig := flags.String("vite-config", "", "path to the Vite configuration file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	packagePath := "."
	if flags.NArg() > 0 {
		packagePath = flags.Arg(0)
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("build accepts one package path")
	}
	var app *appRouter
	options := builder.Options{Package: packagePath, Dir: *dir, Output: *output, StaticWorkers: *staticWorkers, SourceMaps: *sourceMaps, ViteConfig: *viteConfig, OnDescribe: func(description protocol.DescribeResult) {
		if err := printRouteTable(description, app.routeRows(), app.routeParams(), false); err != nil {
			fmt.Fprintln(os.Stderr, "bifrost: route table:", err)
		}
	}, Version: bifrost.Version}
	projectRoot, routeRoot, isAppRouter, err := appRouterRoots(*dir, packagePath)
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("bun"); err != nil {
		return errors.New("bifrost: Bun is required for the build; install it from https://bun.sh")
	}
	if err := requireViteInstalled(projectRoot); err != nil {
		return err
	}
	if isAppRouter {
		prepared, err := prepareAppRouter(context.Background(), projectRoot, routeRoot, *renderConcurrency, *renderQueue)
		if err != nil {
			return err
		}
		app = &prepared
		if err := buildAppRouter(context.Background(), prepared, options, true); err != nil {
			return err
		}
	} else if err := builder.Build(context.Background(), options); err != nil {
		return appRouterHint(projectRoot, err)
	}
	_, _ = fmt.Fprintln(os.Stdout, "Bifrost build complete")
	return nil
}

type routeRow struct {
	Kind    string   `json:"kind"`
	Pattern string   `json:"pattern"`
	Source  string   `json:"source"`
	Params  []string `json:"params,omitempty"`
}

func routeTable(description protocol.DescribeResult, extra []routeRow, params map[string][]string) []routeRow {
	rows := make([]routeRow, 0, len(description.Spec.Routes)+len(extra))
	for _, route := range description.Spec.Routes {
		rows = append(rows, routeRow{Kind: route.Kind, Pattern: route.Pattern, Source: route.View, Params: params[route.Pattern]})
	}
	return append(rows, extra...)
}

func printRouteTable(description protocol.DescribeResult, extra []routeRow, params map[string][]string, asJSON bool) error {
	rows := routeTable(description, extra, params)
	if asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetEscapeHTML(false)
		return encoder.Encode(rows)
	}
	_, _ = fmt.Fprintln(os.Stdout, "Bifrost routes:")
	for _, row := range rows {
		_, _ = fmt.Fprintf(os.Stdout, "  %-12s %-30s %s\n", row.Kind, row.Pattern, row.Source)
	}
	return nil
}
