package gatestore

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	posixPlatformConstraint = "(darwin && !ios) || (linux && !android)"
	otherPlatformConstraint = "!((darwin && !ios) || (linux && !android))"
)

var storeImports = map[string]bool{
	"bytes": true, "crypto/rand": true, "encoding/hex": true, "encoding/json": true, "errors": true,
	"fmt": true, "io/fs": true, "os": true, "path": true, "strings": true, "time": true,
	"github.com/germanmunozs/ai-dev/internal/gate": true, "github.com/germanmunozs/ai-dev/internal/workflow": true,
}

var forbiddenOSWrites = map[string]bool{
	"Create": true, "WriteFile": true, "OpenFile": true, "Rename": true,
	"Remove": true, "Mkdir": true, "Chmod": true,
}

func checkGatestoreSource(filename string, source any) []error {
	file, err := parser.ParseFile(token.NewFileSet(), filename, source, parser.ParseComments)
	if err != nil {
		return []error{err}
	}
	aliases := map[string]string{}
	var findings []error
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			findings = append(findings, err)
			continue
		}
		if importPath == "syscall" {
			if filepath.Base(filename) != "lock_posix.go" {
				findings = append(findings, fmt.Errorf("syscall imported by %s", filename))
			}
		} else if !storeImports[importPath] {
			findings = append(findings, fmt.Errorf("%s forbidden import %q", filename, importPath))
		}
		name := filepath.Base(importPath)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		aliases[name] = importPath
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.GoStmt:
			findings = append(findings, fmt.Errorf("%s contains go statement", filename))
		case *ast.SelectorExpr:
			id, ok := n.X.(*ast.Ident)
			if !ok || aliases[id.Name] != "os" {
				return true
			}
			if forbiddenOSWrites[n.Sel.Name] {
				findings = append(findings, fmt.Errorf("%s calls os.%s", filename, n.Sel.Name))
			}
			if n.Sel.Name == "Getenv" || n.Sel.Name == "Environ" {
				findings = append(findings, fmt.Errorf("%s calls os.%s", filename, n.Sel.Name))
			}
		}
		return true
	})
	return findings
}

func TestBoundaryGatestoreImportsAndWrites(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		for _, finding := range checkGatestoreSource(name, nil) {
			t.Error(finding)
		}
	}
}

func parseConstraintLine(source string) (constraint.Expr, error) {
	scanner := bufio.NewScanner(strings.NewReader(source))
	if !scanner.Scan() {
		return nil, fmt.Errorf("empty source")
	}
	return constraint.Parse(scanner.Text())
}

func checkPlatformSources(sources map[string]string) []error {
	want := map[string]string{
		"commit_posix.go": posixPlatformConstraint,
		"lock_posix.go":   posixPlatformConstraint,
		"new_other.go":    otherPlatformConstraint,
	}
	constraints := map[string]constraint.Expr{}
	var findings []error
	declarations := []string{}
	protected := map[string]bool{"posixStore": true, "defaultOps": true, "flockDir": true, "classifyLock": true, "syscall": true}
	gooses := []string{"darwin", "linux", "ios", "android", "windows", "freebsd", "openbsd", "netbsd", "dragonfly", "solaris", "illumos", "aix", "plan9", "js", "wasip1"}
	goarches := []string{"386", "amd64", "arm", "arm64", "loong64", "mips", "mips64", "mips64le", "mipsle", "ppc64", "ppc64le", "riscv64", "s390x", "wasm"}

	for name, source := range sources {
		file, err := parser.ParseFile(token.NewFileSet(), name, source, 0)
		if err != nil {
			findings = append(findings, err)
			continue
		}
		expr, constraintErr := parseConstraintLine(source)
		if expected, tagged := want[name]; tagged {
			if constraintErr != nil || expr.String() != expected {
				findings = append(findings, fmt.Errorf("%s constraint=%v want %q", name, expr, expected))
			} else {
				constraints[name] = expr
			}
		} else if constraintErr == nil {
			constraints[name] = expr
		}

		usesProtected := false
		ast.Inspect(file, func(node ast.Node) bool {
			if id, ok := node.(*ast.Ident); ok && protected[id.Name] {
				usesProtected = true
			}
			return true
		})
		if usesProtected && (constraintErr != nil || expr.String() != posixPlatformConstraint) {
			findings = append(findings, fmt.Errorf("%s references POSIX implementation without exact constraint", name))
		}

		for _, decl := range file.Decls {
			function, ok := decl.(*ast.FuncDecl)
			if !ok || function.Name.Name != "newPlatformStore" {
				continue
			}
			declarations = append(declarations, name)
			if name == "new_other.go" && !isUnsupportedStoreReturn(function) {
				findings = append(findings, fmt.Errorf("new_other.go newPlatformStore must only return unsupportedStore{}"))
			}
		}

		stem := strings.TrimSuffix(name, ".go")
		for _, suffix := range append(gooses, goarches...) {
			if strings.HasSuffix(stem, "_"+suffix) {
				findings = append(findings, fmt.Errorf("%s uses a GOOS/GOARCH filename suffix", name))
			}
		}
	}
	for name := range want {
		if _, ok := sources[name]; !ok {
			findings = append(findings, fmt.Errorf("missing platform file %s", name))
		}
	}

	if len(declarations) != 2 || !containsString(declarations, "commit_posix.go") || !containsString(declarations, "new_other.go") {
		findings = append(findings, fmt.Errorf("newPlatformStore declarations=%v", declarations))
	}
	posix, posixOK := constraints["commit_posix.go"]
	other, otherOK := constraints["new_other.go"]
	if posixOK && otherOK {
		for _, goos := range gooses {
			tags := platformTags(goos)
			selectedPosix := posix.Eval(func(tag string) bool { return tags[tag] })
			selectedOther := other.Eval(func(tag string) bool { return tags[tag] })
			if selectedPosix == selectedOther {
				findings = append(findings, fmt.Errorf("constraints are not complementary for %s", goos))
			}
			wantPosix := goos == "darwin" || goos == "linux"
			if selectedPosix != wantPosix {
				findings = append(findings, fmt.Errorf("commit_posix.go selection for %s=%t want %t", goos, selectedPosix, wantPosix))
			}
			if selectedOther == wantPosix {
				findings = append(findings, fmt.Errorf("new_other.go selection for %s=%t want %t", goos, selectedOther, !wantPosix))
			}
		}
	}
	return findings
}

func isUnsupportedStoreReturn(function *ast.FuncDecl) bool {
	if function.Body == nil || len(function.Body.List) != 1 {
		return false
	}
	ret, ok := function.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false
	}
	literal, ok := ret.Results[0].(*ast.CompositeLit)
	if !ok || len(literal.Elts) != 0 {
		return false
	}
	id, ok := literal.Type.(*ast.Ident)
	return ok && id.Name == "unsupportedStore"
}

func platformTags(goos string) map[string]bool {
	tags := map[string]bool{goos: true}
	if goos == "ios" {
		tags["darwin"] = true
	}
	if goos == "android" {
		tags["linux"] = true
	}
	if goos == "illumos" {
		tags["solaris"] = true
	}
	return tags
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestBoundaryGatestorePlatformSelection(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	sources := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		data, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		sources[entry.Name()] = string(data)
	}
	for _, finding := range checkPlatformSources(sources) {
		t.Error(finding)
	}
}

func TestBoundaryGatestorePlatformSelectionGoList(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		goos      string
		wantPosix bool
	}{
		{"darwin", true},
		{"linux", true},
		{"ios", false},
		{"android", false},
		{"windows", false},
	} {
		t.Run(tc.goos, func(t *testing.T) {
			cmd := exec.Command(goTool, "list", "-f", `{{join .GoFiles ","}}`, ".")
			cmd.Env = platformGoEnv(tc.goos, "arm64")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("go list: %v\n%s", err, output)
			}
			files := strings.Split(strings.TrimSpace(string(output)), ",")
			hasCommit := containsString(files, "commit_posix.go")
			hasLock := containsString(files, "lock_posix.go")
			hasFallback := containsString(files, "new_other.go")
			if hasCommit != tc.wantPosix || hasLock != tc.wantPosix || hasFallback == tc.wantPosix {
				t.Fatalf("files=%v, wantPosix=%t", files, tc.wantPosix)
			}
		})
	}
}

func platformGoEnv(goos, goarch string) []string {
	env := make([]string, 0, len(os.Environ())+4)
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "GOOS=") || strings.HasPrefix(value, "GOARCH=") || strings.HasPrefix(value, "CGO_ENABLED=") || strings.HasPrefix(value, "GOFLAGS=") {
			continue
		}
		env = append(env, value)
	}
	return append(env, "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0", "GOFLAGS=")
}

func checkRepositorySource(filename string, source any) []error {
	file, err := parser.ParseFile(token.NewFileSet(), filename, source, 0)
	if err != nil {
		return []error{err}
	}
	var findings []error
	rel := filepath.ToSlash(filename)
	imports := map[string]bool{}
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			findings = append(findings, err)
			continue
		}
		imports[importPath] = true
		if importPath == "github.com/germanmunozs/ai-dev/internal/gatestore" && rel != "cmd/ai-dev/main.go" {
			findings = append(findings, fmt.Errorf("gatestore imported by %s", rel))
		}
		if importPath == "github.com/germanmunozs/ai-dev/internal/gate" {
			// internal/watch/picture.go only calls gate.ReadHistory, the
			// read-only reader of 006 Amendment A3; it holds no gate.Store
			// (Feature 015, F015-FR-019).
			allowed := strings.HasPrefix(rel, "internal/gatestore/") || rel == "internal/report/workflowgate.go" || rel == "internal/app/workflowgate.go" || rel == "internal/app/app.go" ||
				rel == "internal/watch/picture.go"
			if !allowed {
				findings = append(findings, fmt.Errorf("gate imported by %s", rel))
			}
		}
	}
	legacy := []string{"workflow", "sdd", "role", "selection", "execution", "agent", "checks", "doctor", "instruction", "system"}
	for _, pkg := range legacy {
		if strings.HasPrefix(rel, "internal/"+pkg+"/") && (imports["github.com/germanmunozs/ai-dev/internal/gate"] || imports["github.com/germanmunozs/ai-dev/internal/gatestore"]) {
			findings = append(findings, fmt.Errorf("legacy package %s imports gate persistence", rel))
		}
	}

	hasRecordReference := false
	hasWriteCall := false
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.Ident:
			if n.Name == "RecordFileName" {
				hasRecordReference = true
			}
		case *ast.BasicLit:
			if n.Kind == token.STRING {
				value, _ := strconv.Unquote(n.Value)
				if value == "workflow-state.json" {
					hasRecordReference = true
				}
			}
		case *ast.CallExpr:
			selector, ok := n.Fun.(*ast.SelectorExpr)
			if ok && isWriteCapableCall(selector.Sel.Name, n.Args) {
				hasWriteCall = true
			}
		}
		return true
	})
	if !strings.HasPrefix(rel, "internal/gatestore/") && hasRecordReference && hasWriteCall {
		findings = append(findings, fmt.Errorf("%s can write the workflow state record outside gatestore", rel))
	}
	return findings
}

func isWriteCapableCall(name string, args []ast.Expr) bool {
	if name == "WriteFile" || name == "Rename" || name == "Create" {
		return true
	}
	if name != "OpenFile" || len(args) < 2 {
		return false
	}
	writeFlag := false
	ast.Inspect(args[1], func(node ast.Node) bool {
		identifier := ""
		switch n := node.(type) {
		case *ast.Ident:
			identifier = n.Name
		case *ast.SelectorExpr:
			identifier = n.Sel.Name
		}
		switch identifier {
		case "O_WRONLY", "O_RDWR", "O_APPEND", "O_CREATE", "O_TRUNC":
			writeFlag = true
		}
		return !writeFlag
	})
	return writeFlag
}

func TestBoundarySoleWritePathImports(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	err := filepath.WalkDir(root, func(filename string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(filename, ".go") || strings.HasSuffix(filename, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		for _, finding := range checkRepositorySource(rel, data) {
			t.Error(finding)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBoundaryGatestoreSelfCheck(t *testing.T) {
	bad := map[string]string{
		"forbidden import": `package gatestore; import "net"; var _ = net.IP{}`,
		"os write":         `package gatestore; import "os"; func f(){ _ = os.WriteFile("x", nil, 0600) }`,
		"environment":      `package gatestore; import "os"; var _ = os.Getenv("USER")`,
		"go statement":     `package gatestore; func f(){ go f() }`,
		"syscall location": `package gatestore; import "syscall"; var _ = syscall.EINTR`,
	}
	for name, source := range bad {
		if findings := checkGatestoreSource(name+".go", source); len(findings) == 0 {
			t.Errorf("accepted %s", name)
		}
	}
	good := `package gatestore; import ("io/fs"; "os"; "github.com/germanmunozs/ai-dev/internal/gate"); func f(root *os.Root, file *os.File, mode fs.FileMode) gate.CommitStatus { _ = root.Rename("a", "b"); _ = file.Chmod(mode); return gate.Committed }`
	if findings := checkGatestoreSource("good.go", good); len(findings) != 0 {
		t.Fatalf("good source rejected: %v", findings)
	}

	goodPlatform := map[string]string{
		"commit_posix.go": "//go:build (darwin && !ios) || (linux && !android)\npackage gatestore\ntype posixStore struct{}\nfunc defaultOps(){}\nfunc newPlatformStore(string) any{return &posixStore{}}",
		"lock_posix.go":   "//go:build (darwin && !ios) || (linux && !android)\npackage gatestore\nimport \"syscall\"\nfunc flockDir() { _ = syscall.EINTR }\nfunc classifyLock(){}",
		"new_other.go":    "//go:build !((darwin && !ios) || (linux && !android))\npackage gatestore\ntype unsupportedStore struct{}\nfunc newPlatformStore(string) any{return unsupportedStore{}}",
		"store.go":        "package gatestore",
	}
	if findings := checkPlatformSources(goodPlatform); len(findings) != 0 {
		t.Fatalf("good platform sources rejected: %v", findings)
	}
	badReturn := cloneSources(goodPlatform)
	badReturn["new_other.go"] = "//go:build !((darwin && !ios) || (linux && !android))\npackage gatestore\ntype posixStore struct{}\nfunc newPlatformStore(string) any{return &posixStore{}}"
	if findings := checkPlatformSources(badReturn); len(findings) == 0 {
		t.Fatal("accepted new_other.go returning posixStore")
	}
	badConstraint := cloneSources(goodPlatform)
	badConstraint["new_other.go"] = "//go:build !darwin\npackage gatestore\ntype unsupportedStore struct{}\nfunc newPlatformStore(string) any{return unsupportedStore{}}"
	if findings := checkPlatformSources(badConstraint); len(findings) == 0 {
		t.Fatal("accepted non-complementary platform constraint")
	}
	legacyPosix := cloneSources(goodPlatform)
	legacyPosix["commit_posix.go"] = "//go:build darwin || linux\npackage gatestore\ntype posixStore struct{}\nfunc defaultOps(){}\nfunc newPlatformStore(string) any{return &posixStore{}}"
	if findings := checkPlatformSources(legacyPosix); len(findings) == 0 {
		t.Fatal("accepted legacy POSIX platform constraint")
	}
	legacyFallback := cloneSources(goodPlatform)
	legacyFallback["new_other.go"] = "//go:build !(darwin || linux)\npackage gatestore\ntype unsupportedStore struct{}\nfunc newPlatformStore(string) any{return unsupportedStore{}}"
	if findings := checkPlatformSources(legacyFallback); len(findings) == 0 {
		t.Fatal("accepted legacy fallback platform constraint")
	}

	badRepository := map[string]string{
		"wrong gatestore importer": `package app; import _ "github.com/germanmunozs/ai-dev/internal/gatestore"`,
		"wrong gate importer":      `package sdd; import _ "github.com/germanmunozs/ai-dev/internal/gate"`,
		"outside writer":           `package workflow; import "os"; const RecordFileName = "workflow-state.json"; func f(){ _ = os.WriteFile(RecordFileName,nil,0600) }`,
		"outside open writer":      `package workflow; import "os"; const RecordFileName = "workflow-state.json"; func f(){ _, _ = os.OpenFile(RecordFileName,os.O_WRONLY,0600) }`,
	}
	paths := map[string]string{
		"wrong gatestore importer": "internal/app/bad.go",
		"wrong gate importer":      "internal/sdd/bad.go",
		"outside writer":           "internal/workflow/bad.go",
		"outside open writer":      "internal/workflow/open_bad.go",
	}
	for name, source := range badRepository {
		if findings := checkRepositorySource(paths[name], source); len(findings) == 0 {
			t.Errorf("accepted %s", name)
		}
	}
	if findings := checkRepositorySource("cmd/ai-dev/main.go", `package main; import _ "github.com/germanmunozs/ai-dev/internal/gatestore"`); len(findings) != 0 {
		t.Fatalf("good repository source rejected: %v", findings)
	}
	if findings := checkRepositorySource("internal/workflow/read.go", `package workflow; import "os"; const RecordFileName = "workflow-state.json"; func f(){ _, _ = os.OpenFile(RecordFileName,os.O_RDONLY,0) }`); len(findings) != 0 {
		t.Fatalf("read-only OpenFile rejected: %v", findings)
	}
}

func cloneSources(source map[string]string) map[string]string {
	clone := make(map[string]string, len(source))
	for name, content := range source {
		clone[name] = content
	}
	return clone
}
