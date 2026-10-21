package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

type goArity struct {
	name    string
	params  int
	results int
}

func validateGoArity(dir string, expected ...goArity) error {
	fset := token.NewFileSet()
	packages, err := parser.ParseDir(fset, dir, func(info os.FileInfo) bool {
		return !info.IsDir() && strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		return err
	}
	functions := make(map[string]*ast.FuncDecl)
	for _, pkg := range packages {
		for _, file := range pkg.Files {
			for _, declaration := range file.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if !ok || function.Recv != nil {
					continue
				}
				functions[function.Name.Name] = function
			}
		}
	}
	var problems []error
	for _, item := range expected {
		function, exists := functions[item.name]
		if !exists {
			continue
		}
		params := fieldCount(function.Type.Params)
		results := fieldCount(function.Type.Results)
		if params != item.params || results != item.results {
			position := fset.Position(function.Pos())
			problems = append(problems, fmt.Errorf("%s: %s must have %d parameters and %d results, got %d and %d", filepath.ToSlash(position.Filename), item.name, item.params, item.results, params, results))
		}
	}
	return errors.Join(problems...)
}

func fieldCount(fields *ast.FieldList) int {
	if fields == nil {
		return 0
	}
	count := 0
	for _, field := range fields.List {
		if len(field.Names) == 0 {
			count++
			continue
		}
		count += len(field.Names)
	}
	return count
}
