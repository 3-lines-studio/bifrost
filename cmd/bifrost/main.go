package main

import (
	"context"
	"flag"
	"fmt"
	"os"

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
	case "version":
		fmt.Println(bifrost.Version)
	case "help", "-h", "--help":
		usage()
		return
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "bifrost:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: bifrost <build|dev|init|version> [options]")
}

func runBuild(args []string) error {
	flags := flag.NewFlagSet("build", flag.ContinueOnError)
	output := flags.String("output", "", "build output directory")
	dir := flags.String("C", ".", "working directory")
	staticWorkers := flags.Int("static-workers", 4, "concurrent static render workers")
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
	if err := builder.Build(context.Background(), builder.Options{Package: packagePath, Dir: *dir, Output: *output, StaticWorkers: *staticWorkers, SourceMaps: *sourceMaps, ViteConfig: *viteConfig, OnDescribe: printRouteTable, Version: bifrost.Version}); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(os.Stdout, "Bifrost build complete")
	return nil
}

func printRouteTable(description protocol.DescribeResult) {
	_, _ = fmt.Fprintln(os.Stdout, "Bifrost routes:")
	for _, route := range description.Spec.Routes {
		_, _ = fmt.Fprintf(os.Stdout, "  %-8s %-24s %s\n", route.Kind, route.Pattern, route.View)
	}
}
