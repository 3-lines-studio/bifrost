package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/3-lines-studio/bifrost/internal/builder"
)

func runRoutes(args []string) error {
	flags := flag.NewFlagSet("routes", flag.ContinueOnError)
	dir := flags.String("C", ".", "working directory")
	viteConfig := flags.String("vite-config", "", "path to the Vite configuration file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	packagePath := "."
	if flags.NArg() > 0 {
		packagePath = flags.Arg(0)
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("routes accepts one package path")
	}
	ctx := context.Background()
	options := builder.Options{Package: packagePath, Dir: *dir, ViteConfig: *viteConfig}
	projectRoot, routeRoot, convention, err := conventionRoots(*dir, packagePath)
	if err != nil {
		return err
	}
	if convention {
		app, err := prepareConventionApp(ctx, projectRoot, routeRoot)
		if err != nil {
			return err
		}
		options.Package = app.Package
		options.Dir = app.WorkDir
	}
	description, err := builder.Describe(ctx, options)
	if err != nil {
		return conventionHint(projectRoot, err)
	}
	printRouteTable(description)
	return nil
}
