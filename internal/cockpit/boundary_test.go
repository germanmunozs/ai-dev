package cockpit

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// forbiddenImports are the packages the presentation-only cockpit may not
// use: no process, file, signal, network or raw system access (plan, R-03).
// Forbidding os outright is stricter than any per-call rule (review F5).
var forbiddenImports = []string{"os", "os/exec", "os/signal", "net", "net/http", "syscall", "unsafe"}

// checkCockpitBoundary reports a forbidden import, any import of an
// internal/ package (the cockpit consumes only its own plain data), and a
// call to a write-capable function. The write rule is a per-file
// heuristic, like the other boundary tests: it catches direct calls, and
// review covers the rest.
func checkCockpitBoundary(filename string, source any) []error {
	file, err := parser.ParseFile(token.NewFileSet(), filename, source, 0)
	if err != nil {
		return []error{err}
	}
	var findings []error
	for _, spec := range file.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		for _, forbidden := range forbiddenImports {
			if path == forbidden {
				findings = append(findings, fmt.Errorf("%s imports %s", filename, path))
			}
		}
		if strings.Contains(path, "/internal/") {
			findings = append(findings, fmt.Errorf("%s imports %s", filename, path))
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			if selector, ok := call.Fun.(*ast.SelectorExpr); ok && writeCapable(selector.Sel.Name, call.Args) {
				findings = append(findings, fmt.Errorf("%s calls %s", filename, selector.Sel.Name))
			}
		}
		return true
	})
	return findings
}

func writeCapable(name string, args []ast.Expr) bool {
	switch name {
	case "WriteFile", "Create", "Mkdir", "MkdirAll", "Rename", "Remove", "RemoveAll":
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

func TestCockpitBoundary(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, finding := range checkCockpitBoundary(name, source) {
			t.Error(finding)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no cockpit source checked")
	}
}

// TestCockpitBoundaryFires proves each rule reports its case.
func TestCockpitBoundaryFires(t *testing.T) {
	cases := map[string]string{
		"exec":     `package cockpit; import "os/exec"; var _ = exec.Command`,
		"net":      `package cockpit; import "net"; var _ = net.Dial`,
		"http":     `package cockpit; import "net/http"; var _ = http.Get`,
		"syscall":  `package cockpit; import "syscall"; var _ = syscall.Kill`,
		"os":       `package cockpit; import "os"; var _ = os.StartProcess`,
		"signal":   `package cockpit; import "os/signal"; var _ = signal.Notify`,
		"unsafe":   `package cockpit; import "unsafe"; var _ = unsafe.Pointer(nil)`,
		"internal": `package cockpit; import "github.com/germanmunozs/ai-dev/internal/workflow"; var _ = workflow.Draft`,
		"write":    `package cockpit; import "os"; func f() { os.WriteFile("x", nil, 0o600) }`,
		"create":   `package cockpit; import "os"; func f() { os.Create("x") }`,
		"open":     `package cockpit; import "os"; func f() { os.OpenFile("x", os.O_RDWR, 0) }`,
		"mkdir":    `package cockpit; import "os"; func f() { os.MkdirAll("x", 0o700) }`,
		"rename":   `package cockpit; import "os"; func f() { os.Rename("x", "y") }`,
		"remove":   `package cockpit; import "os"; func f() { os.Remove("x") }`,
	}
	for name, source := range cases {
		if len(checkCockpitBoundary(name+".go", source)) == 0 {
			t.Errorf("%s: no finding", name)
		}
	}
	clean := `package cockpit; import ("io/fs"; "strings"); func f(f fs.FS) { fs.ReadFile(f, "x"); strings.ToUpper("x") }`
	if findings := checkCockpitBoundary("clean.go", clean); len(findings) != 0 {
		t.Errorf("clean source: %v", findings)
	}
}
