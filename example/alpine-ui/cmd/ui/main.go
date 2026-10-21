package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type registry struct {
	Shared         []string             `json:"shared"`
	SharedBehavior []string             `json:"sharedBehavior"`
	Components     map[string]component `json:"components"`
}

type component struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Files       []string `json:"files"`
	Deps        []string `json:"deps"`
	Behavior    bool     `json:"behavior"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ui:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: ui <list|add>")
	}
	command := args[0]
	registryPath := "registry.json"
	target := "."
	var positional []string
	for index := 1; index < len(args); index++ {
		switch args[index] {
		case "-registry", "--registry":
			index++
			if index >= len(args) {
				return errors.New("missing registry path")
			}
			registryPath = args[index]
		case "-to", "--to":
			index++
			if index >= len(args) {
				return errors.New("missing target directory")
			}
			target = args[index]
		default:
			positional = append(positional, args[index])
		}
	}
	data, err := os.ReadFile(registryPath)
	if err != nil {
		return err
	}
	var source registry
	if err := json.Unmarshal(data, &source); err != nil {
		return err
	}
	switch command {
	case "list":
		names := make([]string, 0, len(source.Components))
		for name := range source.Components {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			item := source.Components[name]
			fmt.Printf("%-8s %-9s %s\n", name, item.Type, item.Description)
		}
		return nil
	case "add":
		return add(source, positional, target)
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func add(source registry, names []string, target string) error {
	if len(names) == 0 {
		return errors.New("add needs at least one component")
	}
	selected := make(map[string]component)
	var visit func(string) error
	visit = func(name string) error {
		if _, exists := selected[name]; exists {
			return nil
		}
		item, exists := source.Components[name]
		if !exists {
			return fmt.Errorf("unknown component %q", name)
		}
		selected[name] = item
		for _, dependency := range item.Deps {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		return nil
	}
	for _, name := range names {
		if err := visit(name); err != nil {
			return err
		}
	}
	files := make([]string, 0)
	files = append(files, source.Shared...)
	behavior := false
	names = names[:0]
	for name := range selected {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		item := selected[name]
		files = append(files, item.Files...)
		behavior = behavior || item.Behavior
	}
	if behavior {
		files = append(files, source.SharedBehavior...)
	}
	for _, file := range files {
		if err := copyFile(file, filepath.Join(target, file)); err != nil {
			return err
		}
	}
	cssImports := []string{"tokens.css", "base.css"}
	for _, name := range names {
		for _, file := range selected[name].Files {
			if strings.HasSuffix(file, ".css") {
				cssImports = append(cssImports, filepath.Base(file))
			}
		}
	}
	var styles strings.Builder
	for _, name := range cssImports {
		fmt.Fprintf(&styles, "@import %q;\n", "./"+name)
	}
	if err := os.WriteFile(filepath.Join(target, "public", "ui", "ui.css"), []byte(styles.String()), 0o644); err != nil {
		return err
	}
	fmt.Printf("added %s\n", strings.Join(names, ", "))
	fmt.Println("link /ui/ui.css and, for behavior components, /ui/ui.js")
	return nil
}

func copyFile(source, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o644)
}
