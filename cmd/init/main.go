package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/3-lines-studio/bifrost/internal/cli"
	"github.com/3-lines-studio/bifrost/internal/initcmd"
)

func main() {
	// Reorder os.Args so flags come before positional args,
	// since Go's flag package stops parsing at the first non-flag arg.
	reorderArgs()

	tmpl := flag.String("template", "static", "Project template: static, full, or desktop")
	module := flag.String("module", "", "Go module name (defaults to directory name)")
	flag.Usage = func() {
		cli.PrintHeader("Bifrost Init")
		fmt.Println()
		cli.PrintInfo("Usage: bifrost-init <project-dir> [--template=static|full|desktop] [--module=<name>]")
		fmt.Println()
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() < 1 {
		cli.PrintHeader("Bifrost Init")
		cli.PrintError("Missing project directory argument")
		fmt.Println()
		cli.PrintInfo("Usage: bifrost-init <project-dir> [--template=static|full|desktop] [--module=<name>]")
		cli.PrintStep(cli.EmojiInfo, "Example: bifrost-init my-app --template=full")
		os.Exit(1)
	}

	projectDir := flag.Arg(0)

	// Locate the example/ directory relative to the bifrost repo root.
	// When run via `go run ./cmd/init`, the working directory is the repo root.
	exampleFS := os.DirFS("example")

	if err := initcmd.Run(projectDir, initcmd.Options{
		Template:   *tmpl,
		ModuleName: *module,
		ScaffoldFS: exampleFS,
	}); err != nil {
		cli.PrintError("%v", err)
		os.Exit(1)
	}
}

// reorderArgs moves flag arguments before positional arguments in os.Args
// so that `bifrost-init /tmp/dir --template=full` works the same as
// `bifrost-init --template=full /tmp/dir`.
func reorderArgs() {
	if len(os.Args) <= 1 {
		return
	}
	var flags, positional []string
	for _, arg := range os.Args[1:] {
		if arg == "--" {
			positional = append(positional, arg)
			break
		}
		if len(arg) > 0 && arg[0] == '-' {
			flags = append(flags, arg)
		} else {
			positional = append(positional, arg)
		}
	}
	os.Args = append([]string{os.Args[0]}, append(flags, positional...)...)
}
