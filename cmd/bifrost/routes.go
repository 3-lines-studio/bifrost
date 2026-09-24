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
	projectRoot, routeRoot, isAppRouter, err := appRouterRoots(*dir, packagePath)
	if err != nil {
		return err
	}
	var app *appRouter
	if isAppRouter {
		prepared, err := prepareAppRouter(ctx, projectRoot, routeRoot, 0, 0)
		if err != nil {
			return err
		}
		app = &prepared
		options.Package = prepared.Package
		options.Dir = prepared.WorkDir
	}
	description, err := builder.Describe(ctx, options)
	if err != nil {
		return appRouterHint(projectRoot, err)
	}
	printRouteTable(description, app.routeRows())
	return nil
}
