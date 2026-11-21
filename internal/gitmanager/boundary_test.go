package gitmanager

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/germanmunozs/ai-dev"

type featureSource struct {
	filename       string
	packagePath    string
	production     bool
	imports        []string
	localImports   []string
	processCapable bool
	processAPIs    []string
}

func TestFeature008ImportBoundaries(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, finding := range checkFeature008Tree(repositoryRoot) {
		t.Error(finding)
	}
}

// checkFeature008Tree enforces production dependencies separately from test
// dependencies. Production gitmanager may use only the standard library and
// conventionalcommit; production conventionalcommit may use only the standard
// library. Tests may import adapters for integration, but process execution is
// allowed only in gitprocess tests and in the controlled hostile-sentinel
// harness in testhelpers_test.go.
func checkFeature008Tree(root string) []error {
	sources, findings := loadGoSources(root)
	packages := make(map[string][]featureSource)
	for _, source := range sources {
		packages[source.packagePath] = append(packages[source.packagePath], source)
		if isFeature008Package(source.packagePath) {
			findings = append(findings, checkFeatureSource(source)...)
		}
	}

	for packagePath := range packages {
		if packagePath == modulePath+"/internal/gitmanager" || strings.HasPrefix(packagePath, modulePath+"/internal/gitmanager/") {
			findings = append(findings, checkIndirectManagerDependencies(packagePath, packages)...)
		}
	}
	return findings
}

func loadGoSources(root string) ([]featureSource, []error) {
	internalRoot := filepath.Join(root, "internal")
	var sources []featureSource
	var findings []error
	err := filepath.WalkDir(internalRoot, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		relative, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		source, sourceFindings := inspectGoSource(filepath.ToSlash(relative), filename)
		findings = append(findings, sourceFindings...)
		if source.packagePath != "" {
			sources = append(sources, source)
		}
		return nil
	})
	if err != nil {
		findings = append(findings, err)
	}
	return sources, findings
}

func inspectGoSource(relative, filename string) (featureSource, []error) {
	parsed, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
	if err != nil {
		return featureSource{}, []error{fmt.Errorf("parse %s: %w", relative, err)}
	}
	directory := filepath.ToSlash(filepath.Dir(relative))
	source := featureSource{
		filename:    relative,
		packagePath: modulePath + "/" + directory,
		production:  !strings.HasSuffix(relative, "_test.go"),
	}
	aliases := make(map[string]string)
	var findings []error
	for _, imported := range parsed.Imports {
		importPath, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			findings = append(findings, fmt.Errorf("%s: invalid import %s", relative, imported.Path.Value))
			continue
		}
		source.imports = append(source.imports, importPath)
		if strings.HasPrefix(importPath, modulePath+"/") {
			source.localImports = append(source.localImports, importPath)
		}
		alias := filepath.Base(importPath)
		if imported.Name != nil {
			alias = imported.Name.Name
		}
		aliases[alias] = importPath
		if importPath == "os/exec" {
			source.processCapable = true
			source.processAPIs = append(source.processAPIs, "os/exec")
		} else if imported.Name != nil && imported.Name.Name == "." && (importPath == "os" || importPath == "syscall" || importPath == "golang.org/x/sys/unix") {
			source.processCapable = true
			source.processAPIs = append(source.processAPIs, "ambiguous dot import "+importPath)
		}
	}

	ast.Inspect(parsed, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		importPath := aliases[identifier.Name]
		forbidden := false
		switch importPath {
		case "os":
			forbidden = selector.Sel.Name == "StartProcess"
		case "syscall", "golang.org/x/sys/unix":
			forbidden = selector.Sel.Name == "Exec" || selector.Sel.Name == "ForkExec" || selector.Sel.Name == "StartProcess"
		}
		if forbidden {
			source.processCapable = true
			source.processAPIs = append(source.processAPIs, importPath+"."+selector.Sel.Name)
		}
		return true
	})
	return source, findings
}

func checkFeatureSource(source featureSource) []error {
	var findings []error
	manager := source.packagePath == modulePath+"/internal/gitmanager" || strings.HasPrefix(source.packagePath, modulePath+"/internal/gitmanager/")
	conventional := source.packagePath == modulePath+"/internal/conventionalcommit" || strings.HasPrefix(source.packagePath, modulePath+"/internal/conventionalcommit/")
	gitprocess := source.packagePath == modulePath+"/internal/gitprocess" || strings.HasPrefix(source.packagePath, modulePath+"/internal/gitprocess/")

	if source.production && (manager || conventional) {
		for _, importPath := range source.imports {
			allowed := isStandardImport(importPath)
			if manager && (importPath == modulePath+"/internal/conventionalcommit" || strings.HasPrefix(importPath, modulePath+"/internal/conventionalcommit/")) {
				allowed = true
			}
			if !allowed {
				findings = append(findings, fmt.Errorf("%s: forbidden production import %q", source.filename, importPath))
			}
		}
	}
	approvedOSExec := onlyOSExec(source.processAPIs) && (gitprocess || sentinelHarnessException(source))
	if source.processCapable && !approvedOSExec {
		findings = append(findings, fmt.Errorf("%s: process execution %v is allowed only in gitprocess or the controlled sentinel harness", source.filename, source.processAPIs))
	}
	return findings
}

func onlyOSExec(apis []string) bool {
	if len(apis) == 0 {
		return false
	}
	for _, api := range apis {
		if api != "os/exec" {
			return false
		}
	}
	return true
}

func isStandardImport(importPath string) bool {
	if importPath == "C" {
		return false
	}
	if strings.HasPrefix(importPath, modulePath+"/") {
		return false
	}
	first := strings.SplitN(importPath, "/", 2)[0]
	return !strings.Contains(first, ".")
}

func sentinelHarnessException(source featureSource) bool {
	return !source.production && source.filename == "internal/gitmanager/testhelpers_test.go"
}

func checkIndirectManagerDependencies(start string, packages map[string][]featureSource) []error {
	visited := make(map[string]bool)
	var findings []error
	var visit func(string, []string)
	visit = func(current string, chain []string) {
		if visited[current] {
			return
		}
		visited[current] = true
		for _, source := range packages[current] {
			if !source.production {
				continue
			}
			if current != start && source.processCapable {
				findings = append(findings, fmt.Errorf("%s: indirect process dependency through %s", start, strings.Join(append(chain, current), " -> ")))
			}
			for _, dependency := range source.localImports {
				if dependency == modulePath+"/internal/gitprocess" || strings.HasPrefix(dependency, modulePath+"/internal/gitprocess/") ||
					dependency == modulePath+"/internal/gitstore" || strings.HasPrefix(dependency, modulePath+"/internal/gitstore/") {
					findings = append(findings, fmt.Errorf("%s: forbidden indirect adapter dependency through %s", start, strings.Join(append(chain, current, dependency), " -> ")))
				}
				if strings.HasPrefix(dependency, modulePath+"/") {
					visit(dependency, append(chain, current))
				}
			}
		}
	}
	visit(start, nil)
	return findings
}

func isFeature008Package(packagePath string) bool {
	for _, root := range []string{"gitmanager", "gitprocess", "gitstore", "conventionalcommit"} {
		prefix := modulePath + "/internal/" + root
		if packagePath == prefix || strings.HasPrefix(packagePath, prefix+"/") {
			return true
		}
	}
	return false
}

func TestFeature008BoundaryMutationMatrix(t *testing.T) {
	mutations := []struct {
		id          string
		description string
		files       map[string]string
	}{
		{"M1", "gitmanager imports gitprocess", map[string]string{"internal/gitmanager/bad.go": `package gitmanager; import _ "github.com/germanmunozs/ai-dev/internal/gitprocess"`}},
		{"M2", "gitmanager imports os/exec", map[string]string{"internal/gitmanager/bad.go": `package gitmanager; import "os/exec"; var _ = exec.Command`}},
		{"M3", "gitmanager reaches os/exec through internal/system", map[string]string{
			"internal/gitmanager/bad.go":  `package gitmanager; import _ "github.com/germanmunozs/ai-dev/internal/system"`,
			"internal/system/executor.go": `package system; import "os/exec"; var _ = exec.Command`,
		}},
		{"M4", "gitmanager imports report", map[string]string{"internal/gitmanager/bad.go": `package gitmanager; import _ "github.com/germanmunozs/ai-dev/internal/report"`}},
		{"M5", "gitmanager imports gatestore", map[string]string{"internal/gitmanager/bad.go": `package gitmanager; import _ "github.com/germanmunozs/ai-dev/internal/gatestore"`}},
		{"M6", "gitmanager subpackage imports os/exec", map[string]string{"internal/gitmanager/sub/bad.go": `package sub; import "os/exec"; var _ = exec.Command`}},
		{"M7", "conventionalcommit imports gitmanager", map[string]string{"internal/conventionalcommit/bad.go": `package conventionalcommit; import _ "github.com/germanmunozs/ai-dev/internal/gitmanager"`}},
		{"M8", "gitmanager calls os.StartProcess", map[string]string{"internal/gitmanager/bad.go": `package gitmanager; import "os"; func f(){ _, _ = os.StartProcess("x", nil, nil) }`}},
		{"M9", "gitmanager calls syscall.Exec", map[string]string{"internal/gitmanager/bad.go": `package gitmanager; import "syscall"; func f(){ _ = syscall.Exec("x", nil, nil) }`}},
		{"M10", "gitstore imports os/exec", map[string]string{"internal/gitstore/bad.go": `package gitstore; import "os/exec"; var _ = exec.Command`}},
		{"M11", "unapproved gitmanager test imports os/exec", map[string]string{"internal/gitmanager/bad_test.go": `package gitmanager; import "os/exec"; var _ = exec.Command`}},
	}
	if len(mutations) != 11 {
		t.Fatalf("mutation cases=%d, want 11", len(mutations))
	}
	for _, mutation := range mutations {
		mutation := mutation
		t.Run(mutation.id, func(t *testing.T) {
			root := writeBoundaryTree(t, mutation.files)
			findings := checkFeature008Tree(root)
			if len(findings) == 0 {
				t.Fatalf("detector accepted %s: %s", mutation.id, mutation.description)
			}
			t.Logf("%s detected as expected: %s; findings=%v", mutation.id, mutation.description, findings)
		})
	}
}

func TestFeature008BoundaryAllowsApprovedGraph(t *testing.T) {
	root := writeBoundaryTree(t, nil)
	if findings := checkFeature008Tree(root); len(findings) != 0 {
		t.Fatalf("approved graph rejected: %v", findings)
	}
}

func writeBoundaryTree(t *testing.T, mutations map[string]string) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"internal/gitmanager/manager.go":          `package gitmanager; import ("context"; _ "github.com/germanmunozs/ai-dev/internal/conventionalcommit"); var _ = context.Background`,
		"internal/gitmanager/testhelpers_test.go": `package gitmanager; import "os/exec"; var _ = exec.Command`,
		"internal/conventionalcommit/validate.go": `package conventionalcommit; import "strings"; var _ = strings.TrimSpace`,
		"internal/gitprocess/runner.go":           `package gitprocess; import "os/exec"; var _ = exec.Command`,
		"internal/gitstore/store.go":              `package gitstore; import _ "github.com/germanmunozs/ai-dev/internal/gitmanager"`,
		"internal/report/report.go":               `package report`,
		"internal/gatestore/store.go":             `package gatestore`,
		"internal/system/safe.go":                 `package system`,
	}
	for name, source := range mutations {
		files[name] = source
	}
	paths := make([]string, 0, len(files))
	for name := range files {
		paths = append(paths, name)
	}
	sort.Strings(paths)
	for _, name := range paths {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(files[name]), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
