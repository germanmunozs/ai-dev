package sdd

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var allowedSDDImports = map[string]bool{
	"io/fs":                    true,
	"strings":                  true,
	"ai-dev/internal/workflow": true,
	"ai-dev/internal/role":     true,
}

var forbiddenWorkflowSelectors = map[string]bool{
	"Allowed":            true,
	"EvaluateTransition": true,
	"TransitionReport":   true,
	"TransitionValid":    true,
	"TransitionInvalid":  true,
	"TargetUnsupported":  true,
}

var allowedRoleSelectors = map[string]bool{
	"Role":           true,
	"Specification":  true,
	"Planning":       true,
	"Implementation": true,
	"Review":         true,
}

func checkSDDSource(filename string, src any) []error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return []error{fmt.Errorf("parse %s: %w", filename, err)}
	}

	aliases := make(map[string]string)
	var findings []error
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			findings = append(findings, fmt.Errorf("%s: invalid import path %s", filename, spec.Path.Value))
			continue
		}
		if !allowedSDDImports[importPath] {
			findings = append(findings, fmt.Errorf("%s: forbidden import %q", filename, importPath))
		}
		local := filepath.Base(importPath)
		if spec.Name != nil {
			local = spec.Name.Name
		}
		if strings.HasPrefix(importPath, "ai-dev/internal/") && (local == "." || local == "_") {
			findings = append(findings, fmt.Errorf("%s: rejected %s import of %q", filename, local, importPath))
			continue
		}
		aliases[local] = importPath
	}

	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.SelectorExpr:
			ident, ok := n.X.(*ast.Ident)
			if !ok {
				return true
			}
			switch aliases[ident.Name] {
			case "ai-dev/internal/workflow":
				if forbiddenWorkflowSelectors[n.Sel.Name] {
					findings = append(findings, fmt.Errorf("%s: forbidden workflow selector %s", filename, n.Sel.Name))
				}
			case "ai-dev/internal/role":
				if !allowedRoleSelectors[n.Sel.Name] {
					findings = append(findings, fmt.Errorf("%s: forbidden role selector %s", filename, n.Sel.Name))
				}
			}
		case *ast.BasicLit:
			if n.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(n.Value)
			if err == nil && (value == "specs" || strings.HasPrefix(value, "specs/")) {
				findings = append(findings, fmt.Errorf("%s: forbidden feature-path literal %q", filename, value))
			}
		}
		return true
	})
	return findings
}

func TestPackageBoundary(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var scanned []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		scanned = append(scanned, name)
		for _, finding := range checkSDDSource(name, nil) {
			t.Error(finding)
		}
	}
	sort.Strings(scanned)
	want := []string{"activity.go", "evaluate.go", "outcome.go", "rules.go", "validate.go"}
	if !reflect.DeepEqual(scanned, want) {
		t.Fatalf("scanned production files = %v, want exactly %v", scanned, want)
	}
}

func TestPackageBoundarySelfCheck(t *testing.T) {
	bad := map[string]string{
		"os":                    `package sdd; import "os"; var _ = os.Stdout`,
		"os exec":               `package sdd; import "os/exec"; var _ = exec.Command`,
		"os signal":             `package sdd; import "os/signal"; var _ = signal.Notify`,
		"net":                   `package sdd; import "net"; var _ net.Conn`,
		"syscall":               `package sdd; import "syscall"; var _ = syscall.SIGPIPE`,
		"selection":             `package sdd; import "ai-dev/internal/selection"; var _ selection.Report`,
		"aliased workflow":      `package sdd; import wf "ai-dev/internal/workflow"; var _ = wf.Allowed`,
		"aliased remediation":   `package sdd; import r "ai-dev/internal/role"; var _ = r.Remediation`,
		"default remediation":   `package sdd; import "ai-dev/internal/role"; var _ = role.Remediation`,
		"blank internal import": `package sdd; import _ "ai-dev/internal/selection"`,
		"blank allowed import":  `package sdd; import _ "ai-dev/internal/workflow"`,
		"dot internal import":   `package sdd; import . "ai-dev/internal/workflow"`,
		"specs literal":         `package sdd; const root = "specs"`,
	}
	if len(bad) != 13 {
		t.Fatalf("known-bad snippets = %d, want 13", len(bad))
	}
	for name, src := range bad {
		t.Run(name, func(t *testing.T) {
			if findings := checkSDDSource(name+".go", src); len(findings) == 0 {
				t.Fatal("checker accepted known-bad source")
			}
		})
	}

	good := `package sdd
import (
	"io/fs"
	"strings"
	"ai-dev/internal/workflow"
	"ai-dev/internal/role"
)
func good(root fs.FS, id string) role.Role {
	_, _ = workflow.FeatureDir(strings.TrimSpace(id))
	_ = workflow.EvaluateState(root, id)
	return role.Planning
}`
	if findings := checkSDDSource("good.go", good); len(findings) != 0 {
		t.Fatalf("checker rejected known-good source: %v", findings)
	}
}
