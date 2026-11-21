package configstore

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const configstoreImport = "github.com/germanmunozs/ai-dev/internal/configstore"

// checkConfigWriteBoundary reports a file outside internal/configstore that
// can write .ai-dev/agents.json, a file other than main that imports
// configstore, and configstore naming config.json, which belongs to
// Feature 008 (F011 R-03). .ai-dev itself is shared: gitmanager creates it
// for config.json, so the directory alone is not a finding.
//
// Limits (review L6): the import rule is exact, but the write rule is a
// per-file heuristic, like gatestore's. It does not follow a file name built
// at run time ("agents"+".json"), held in another file of the package, or
// written through a helper (io.Copy, Truncate, Symlink). It catches the
// direct cases; review covers the rest.
func checkConfigWriteBoundary(filename string, source any) []error {
	file, err := parser.ParseFile(token.NewFileSet(), filename, source, 0)
	if err != nil {
		return []error{err}
	}
	rel := filepath.ToSlash(filename)
	var findings []error
	for _, spec := range file.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		if path == configstoreImport && rel != "cmd/ai-dev/main.go" {
			findings = append(findings, fmt.Errorf("configstore imported by %s", rel))
		}
	}
	references, writes, names008 := false, false, false
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.BasicLit:
			value, _ := strconv.Unquote(n.Value)
			if n.Kind == token.STRING && strings.Contains(value, "agents.json") {
				references = true
			}
			if n.Kind == token.STRING && strings.Contains(value, "config.json") {
				names008 = true
			}
		case *ast.SelectorExpr:
			if n.Sel.Name == "FileName" {
				if pkg, ok := n.X.(*ast.Ident); ok && pkg.Name == "projectconfig" {
					references = true
				}
			}
		case *ast.CallExpr:
			if selector, ok := n.Fun.(*ast.SelectorExpr); ok && writeCapable(selector.Sel.Name, n.Args) {
				writes = true
			}
		}
		return true
	})
	if references && writes && !strings.HasPrefix(rel, "internal/configstore/") {
		findings = append(findings, fmt.Errorf("%s can write .ai-dev/agents.json outside configstore", rel))
	}
	if names008 && strings.HasPrefix(rel, "internal/configstore/") {
		findings = append(findings, fmt.Errorf("%s names config.json, which Feature 008 owns", rel))
	}
	return findings
}

func writeCapable(name string, args []ast.Expr) bool {
	switch name {
	case "WriteFile", "Rename", "Create", "Mkdir", "MkdirAll":
		return true
	case "OpenFile":
		if len(args) < 2 {
			return false
		}
		found := false
		ast.Inspect(args[1], func(node ast.Node) bool {
			var id string
			switch n := node.(type) {
			case *ast.Ident:
				id = n.Name
			case *ast.SelectorExpr:
				id = n.Sel.Name
			}
			switch id {
			case "O_WRONLY", "O_RDWR", "O_APPEND", "O_CREATE", "O_TRUNC":
				found = true
			}
			return !found
		})
		return found
	}
	return false
}

func TestConfigWriteBoundary(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "specs" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, finding := range checkConfigWriteBoundary(rel, source) {
			t.Error(finding)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The checker flags each kind of violation and accepts reads and main.
func TestConfigWriteBoundarySelfCheck(t *testing.T) {
	bad := map[string]string{
		"literal writer":   `package app; import "os"; func f(){ _ = os.WriteFile(".ai-dev/agents.json", nil, 0600) }`,
		"constant writer":  `package app; import ("os"; "x/projectconfig"); func f(){ _ = os.Rename("t", projectconfig.FileName) }`,
		"open writer":      `package app; import "os"; func f(){ _, _ = os.OpenFile("agents.json", os.O_WRONLY|os.O_CREATE, 0600) }`,
		"configstore user": `package app; import _ "github.com/germanmunozs/ai-dev/internal/configstore"`,
	}
	for name, source := range bad {
		if findings := checkConfigWriteBoundary("internal/app/x.go", source); len(findings) == 0 {
			t.Errorf("%s: not flagged", name)
		}
	}
	good := map[string]string{
		"read-only open":     `package app; import "os"; func f(){ _, _ = os.OpenFile(".ai-dev/agents.json", os.O_RDONLY, 0) }`,
		"008 writes its own": `package gitmanager; import "os"; func f(){ _ = os.Mkdir(".ai-dev", 0755); _ = os.WriteFile(".ai-dev/config.json", nil, 0644) }`,
		"main import":        `package main; import _ "github.com/germanmunozs/ai-dev/internal/configstore"`,
	}
	for name, source := range good {
		file := "internal/app/x.go"
		if name == "main import" {
			file = "cmd/ai-dev/main.go"
		}
		if findings := checkConfigWriteBoundary(file, source); len(findings) != 0 {
			t.Errorf("%s: flagged %v", name, findings)
		}
	}
	if findings := checkConfigWriteBoundary("internal/configstore/write_posix.go", bad["literal writer"]); len(findings) != 0 {
		t.Errorf("configstore itself flagged: %v", findings)
	}
	if findings := checkConfigWriteBoundary("internal/configstore/write_posix.go", `package configstore; const x = ".ai-dev/config.json"`); len(findings) == 0 {
		t.Error("configstore naming config.json was not flagged")
	}
}
