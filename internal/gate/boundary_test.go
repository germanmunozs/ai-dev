package gate

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

var gateImports = map[string]bool{
	"io/fs": true, "strings": true, "time": true,
	"ai-dev/internal/workflow": true, "ai-dev/internal/sdd": true,
}

var gateWorkflowSelectors = map[string]bool{
	"EvaluateTransition": true, "FeatureDir": true, "Recognized": true,
	"State": true, "Outcome": true,
	"Draft": true, "Specified": true, "Planned": true, "Tasked": true,
	"Implemented": true, "Approved": true, "Abandoned": true,
	"FeatureUnknown": true, "StateInvalid": true, "TargetUnsupported": true,
	"TransitionInvalid": true, "TransitionValid": true,
}

func checkGateSource(filename string, src any) []error {
	file, err := parser.ParseFile(token.NewFileSet(), filename, src, 0)
	if err != nil {
		return []error{err}
	}
	aliases := map[string]string{}
	var findings []error
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			findings = append(findings, err)
			continue
		}
		if !gateImports[path] {
			findings = append(findings, fmt.Errorf("forbidden import %q", path))
		}
		name := filepath.Base(path)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		aliases[name] = path
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.GoStmt:
			findings = append(findings, fmt.Errorf("go statement forbidden"))
		case *ast.SelectorExpr:
			id, ok := n.X.(*ast.Ident)
			if !ok {
				return true
			}
			switch aliases[id.Name] {
			case "ai-dev/internal/sdd":
				if n.Sel.Name != "MissingArtifacts" {
					findings = append(findings, fmt.Errorf("forbidden sdd selector %s", n.Sel.Name))
				}
			case "ai-dev/internal/workflow":
				if !gateWorkflowSelectors[n.Sel.Name] {
					findings = append(findings, fmt.Errorf("forbidden workflow selector %s", n.Sel.Name))
				}
			}
		}
		return true
	})
	return findings
}

func TestBoundaryGatePackage(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		for _, finding := range checkGateSource(name, nil) {
			t.Error(finding)
		}
	}
}

func TestBoundaryGateSelfCheck(t *testing.T) {
	bad := map[string]string{
		"os":               `package gate; import "os"; var _ = os.Stdout`,
		"sdd evaluate":     `package gate; import "ai-dev/internal/sdd"; var _ = sdd.Evaluate`,
		"workflow allowed": `package gate; import "ai-dev/internal/workflow"; var _ = workflow.Allowed`,
		"workflow resolve": `package gate; import "ai-dev/internal/workflow"; var _ = workflow.ResolveRecord`,
		"go":               `package gate; func f(){ go f() }`,
	}
	for name, src := range bad {
		if len(checkGateSource(name+".go", src)) == 0 {
			t.Errorf("accepted %s", name)
		}
	}
	good := `package gate; import ("io/fs"; "time"; "ai-dev/internal/sdd"; "ai-dev/internal/workflow"); func f(r fs.FS){ _=time.Now(); _=sdd.MissingArtifacts(r,"f",workflow.Planned); _=workflow.EvaluateTransition(r,"f","planned") }`
	if findings := checkGateSource("good.go", good); len(findings) != 0 {
		t.Fatalf("good rejected: %v", findings)
	}
}
