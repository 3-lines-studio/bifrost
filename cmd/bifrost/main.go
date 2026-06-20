package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printRootUsage()
		os.Exit(1)
	}

	sub := os.Args[1]
	args := os.Args[2:]

	switch sub {
	case "init":
		runInit(args)
	case "build":
		runBuild(args)
	case "doctor":
		runDoctor(args)
	case "--help", "-h", "help":
		printRootUsage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand: %s\n\n", sub)
		printRootUsage()
		os.Exit(1)
	}
}

func printRootUsage() {
	fmt.Println("Bifrost CLI")
	fmt.Println()
	fmt.Println("Usage: bifrost <subcommand> [options]")
	fmt.Println()
	fmt.Println("Subcommands:")
	fmt.Println("  init     Scaffold a new Bifrost project from a template")
	fmt.Println("  build    Run the production asset build")
	fmt.Println("  doctor   Repair the .bifrost directory")
	fmt.Println()
	fmt.Println("Use 'bifrost <subcommand> --help' for subcommand-specific help")
}
