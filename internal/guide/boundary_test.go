package guide

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

// allowedGuideImports is the complete import allowlist for production files.
// Everything else, including os, os/exec, net, syscall, internal/system and
// the execution, gate and Git packages, is rejected (F007-FR-007, R-07).
// Because os is not allowed, guide cannot read the environment or any
// configuration file (F007-FR-014).
var allowedGuideImports = map[string]bool{
	"io/fs":                     true,
	"strings":                   true,
	"unicode":                   true,
	"ai-dev/internal/sdd":       true,
	"ai-dev/internal/selection": true,
	"ai-dev/internal/agent":     true,
	"ai-dev/internal/role":      true,
	"ai-dev/internal/workflow":  true,
}

// forbiddenGuideSelectors are allowed-package identifiers guide must not use:
// constructors that would let it build its own discovery path (the registry
// is built by internal/app from the injected executor), and 004's transition
// evaluation (F007-FR-003).
var forbiddenGuideSelectors = map[string]map[string]bool{
	"ai-dev/internal/agent":     {"NewRegistry": true, "Registry": true},
	"ai-dev/internal/selection": {"NewSelector": true},
	"ai-dev/internal/workflow": {
		"Allowed":            true,
		"EvaluateTransition": true,
		"TransitionReport":   true,
		"TransitionValid":    true,
		"TransitionInvalid":  true,
		"TargetUnsupported":  true,
	},
}

func checkGuideSource(filename string, src any) []error {
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
		if !allowedGuideImports[importPath] {
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
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		if forbiddenGuideSelectors[aliases[ident.Name]][selector.Sel.Name] {
			findings = append(findings, fmt.Errorf("%s: forbidden selector %s.%s", filename, ident.Name, selector.Sel.Name))
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
		for _, finding := range checkGuideSource(name, nil) {
			t.Error(finding)
		}
	}
	sort.Strings(scanned)
	want := []string{"doc.go", "guide.go", "safety.go", "templates.go"}
	if !reflect.DeepEqual(scanned, want) {
		t.Fatalf("scanned production files = %v, want exactly %v", scanned, want)
	}
}

func TestPackageBoundarySelfCheck(t *testing.T) {
	bad := map[string]string{
		"os":                    `package guide; import "os"; var _ = os.Getenv`,
		"os exec":               `package guide; import "os/exec"; var _ = exec.Command`,
		"net":                   `package guide; import "net"; var _ net.Conn`,
		"net http":              `package guide; import "net/http"; var _ = http.Get`,
		"syscall":               `package guide; import "syscall"; var _ = syscall.SIGPIPE`,
		"system":                `package guide; import "ai-dev/internal/system"; var _ system.Executor`,
		"execution":             `package guide; import "ai-dev/internal/execution"; var _ execution.Runner`,
		"instruction":           `package guide; import "ai-dev/internal/instruction"; var _ = instruction.X`,
		"gate":                  `package guide; import "ai-dev/internal/gate"; var _ gate.Store`,
		"gatestore":             `package guide; import "ai-dev/internal/gatestore"; var _ = gatestore.New`,
		"gitmanager":            `package guide; import "ai-dev/internal/gitmanager"; var _ = gitmanager.X`,
		"gitprocess":            `package guide; import "ai-dev/internal/gitprocess"; var _ = gitprocess.X`,
		"gitstore":              `package guide; import "ai-dev/internal/gitstore"; var _ = gitstore.X`,
		"app":                   `package guide; import "ai-dev/internal/app"; var _ = app.Run`,
		"report":                `package guide; import "ai-dev/internal/report"; var _ = report.WriteText`,
		"new registry":          `package guide; import "ai-dev/internal/agent"; var _ = agent.NewRegistry`,
		"aliased new registry":  `package guide; import a "ai-dev/internal/agent"; var _ = a.NewRegistry`,
		"registry type":         `package guide; import "ai-dev/internal/agent"; var _ agent.Registry`,
		"new selector":          `package guide; import "ai-dev/internal/selection"; var _ = selection.NewSelector`,
		"transition evaluation": `package guide; import "ai-dev/internal/workflow"; var _ = workflow.EvaluateTransition`,
		"aliased transition":    `package guide; import wf "ai-dev/internal/workflow"; var _ = wf.Allowed`,
		"blank internal import": `package guide; import _ "ai-dev/internal/system"`,
		"blank allowed import":  `package guide; import _ "ai-dev/internal/sdd"`,
		"dot internal import":   `package guide; import . "ai-dev/internal/agent"`,
	}
	if len(bad) != 24 {
		t.Fatalf("known-bad snippets = %d, want 24", len(bad))
	}
	for name, src := range bad {
		t.Run(name, func(t *testing.T) {
			if findings := checkGuideSource(name+".go", src); len(findings) == 0 {
				t.Fatal("checker accepted known-bad source")
			}
		})
	}

	good := `package guide
import (
	"io/fs"
	"strings"
	"unicode"
	"ai-dev/internal/agent"
	"ai-dev/internal/role"
	"ai-dev/internal/sdd"
	"ai-dev/internal/selection"
	"ai-dev/internal/workflow"
)
func good(root fs.FS, id string, s interface{ Select(string) selection.Report }) (agent.ID, role.Role, workflow.State, bool) {
	r := sdd.Evaluate(root, strings.TrimSpace(id))
	_ = s.Select(string(r.Role))
	return agent.ClaudeCode, r.Role, r.State, unicode.Is(unicode.Cf, 'x')
}`
	if findings := checkGuideSource("good.go", good); len(findings) != 0 {
		t.Fatalf("checker rejected known-good source: %v", findings)
	}
}
