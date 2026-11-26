package watch

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The watch command cannot write state: neither this package nor the
// watcher adapter depends on a store, the Git manager, agent execution, the
// reports (which depend on both) or the executor (F015-FR-019, F015-FR-020,
// research R-03, owner decision 1).
var forbiddenDeps = []string{
	"github.com/germanmunozs/ai-dev/internal/gatestore",
	"github.com/germanmunozs/ai-dev/internal/configstore",
	"github.com/germanmunozs/ai-dev/internal/gitstore",
	"github.com/germanmunozs/ai-dev/internal/gitmanager",
	"github.com/germanmunozs/ai-dev/internal/gitprocess",
	"github.com/germanmunozs/ai-dev/internal/execution",
	"github.com/germanmunozs/ai-dev/internal/report",
	"github.com/germanmunozs/ai-dev/internal/system",
	"os/exec",
}

func TestWatchDependencies(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "windows"} {
		cmd := exec.Command("go", "list", "-deps", "./internal/watch", "./internal/fswatch")
		cmd.Dir = filepath.Join("..", "..")
		cmd.Env = append(os.Environ(), "GOOS="+goos)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("GOOS=%s go list: %v\n%s", goos, err, out)
		}
		deps := map[string]bool{}
		for _, line := range strings.Fields(string(out)) {
			deps[line] = true
		}
		for _, forbidden := range forbiddenDeps {
			if deps[forbidden] {
				t.Errorf("GOOS=%s: internal/watch or internal/fswatch depends on %s", goos, forbidden)
			}
		}
	}
}

// writeCalls are the calls that change the file system.
var writeCalls = map[string]map[string]bool{
	"os": {"Create": true, "OpenFile": true, "WriteFile": true, "Remove": true, "RemoveAll": true, "Rename": true,
		"Mkdir": true, "MkdirAll": true, "MkdirTemp": true, "CreateTemp": true, "Chmod": true, "Chtimes": true,
		"Chown": true, "Lchown": true, "Link": true, "Symlink": true, "Truncate": true},
	"syscall": {"Flock": true, "Unlink": true, "Rmdir": true, "Rename": true, "Mkdir": true, "Creat": true,
		"Truncate": true, "Ftruncate": true, "Write": true, "Pwrite": true, "Link": true, "Symlink": true,
		"Chmod": true, "Fchmod": true, "Chown": true, "Fchown": true, "Lchown": true,
		// Raw system calls could do any of the above (review F11).
		"Syscall": true, "Syscall6": true, "Syscall9": true, "RawSyscall": true, "RawSyscall6": true},
}

// writeMethods are method names that change a file, whatever the receiver
// (review F11); none is used by these packages.
var writeMethods = map[string]bool{"Chmod": true, "Chown": true, "Truncate": true, "WriteAt": true, "Chtimes": true}

// openFlags are the only flags a syscall.Open in these packages may use.
var openFlags = map[string]bool{"O_EVTONLY": true, "O_SYMLINK": true, "O_CLOEXEC": true, "O_RDONLY": true, "O_NONBLOCK": true}

func checkWriteCalls(filename string, src any) []error {
	file, err := parser.ParseFile(token.NewFileSet(), filename, src, 0)
	if err != nil {
		return []error{err}
	}
	aliases := map[string]string{}
	for _, spec := range file.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		name := filepath.Base(path)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		aliases[name] = path
	}
	var findings []error
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		pkg := aliases[id.Name]
		if writeCalls[pkg][sel.Sel.Name] {
			findings = append(findings, fmt.Errorf("%s: write call %s.%s", filename, pkg, sel.Sel.Name))
		}
		if pkg == "" && writeMethods[sel.Sel.Name] {
			findings = append(findings, fmt.Errorf("%s: write method %s.%s", filename, id.Name, sel.Sel.Name))
		}
		if pkg == "syscall" && sel.Sel.Name == "Open" && len(call.Args) >= 2 {
			findings = append(findings, checkOpenFlags(filename, call.Args[1], aliases)...)
		}
		return true
	})
	return findings
}

// checkOpenFlags accepts only an expression built with | from allowed
// syscall flag selectors; a variable, constant or literal is refused, since
// its value cannot be checked here (review F11).
func checkOpenFlags(filename string, expr ast.Expr, aliases map[string]string) []error {
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return checkOpenFlags(filename, e.X, aliases)
	case *ast.BinaryExpr:
		if e.Op != token.OR {
			return []error{fmt.Errorf("%s: syscall.Open flags combined with %s", filename, e.Op)}
		}
		return append(checkOpenFlags(filename, e.X, aliases), checkOpenFlags(filename, e.Y, aliases)...)
	case *ast.SelectorExpr:
		if x, ok := e.X.(*ast.Ident); ok && aliases[x.Name] == "syscall" && openFlags[e.Sel.Name] {
			return nil
		}
		return []error{fmt.Errorf("%s: syscall.Open flag %s", filename, e.Sel.Name)}
	}
	return []error{fmt.Errorf("%s: syscall.Open flags not built from allowed selectors", filename)}
}

func TestWatchWritesNothingInSource(t *testing.T) {
	for _, dir := range []string{".", filepath.Join("..", "fswatch")} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			for _, finding := range checkWriteCalls(filepath.Join(dir, name), nil) {
				t.Error(finding)
			}
		}
	}
}

func TestWriteCallCheckSelf(t *testing.T) {
	bad := map[string]string{
		"remove":     `package p; import "os"; func f() { os.Remove("x") }`,
		"aliased":    `package p; import o "os"; func f() { o.WriteFile("x", nil, 0) }`,
		"flock":      `package p; import "syscall"; func f() { syscall.Flock(0, 0) }`,
		"open write": `package p; import "syscall"; func f() { syscall.Open("x", syscall.O_RDWR|syscall.O_CLOEXEC, 0) }`,
		"open lit":   `package p; import "syscall"; func f() { syscall.Open("x", 2, 0) }`,
		"truncate":   `package p; import "os"; func f() { os.Truncate("x", 0) }`,
		"open var":   `package p; import "syscall"; func f() { flags := syscall.O_RDWR; syscall.Open("x", flags, 0) }`,
		"raw call":   `package p; import "syscall"; func f() { syscall.Syscall(0, 0, 0, 0) }`,
		"method":     `package p; import "os"; func f(file *os.File) { file.Truncate(0) }`,
	}
	for name, src := range bad {
		if len(checkWriteCalls(name+".go", src)) == 0 {
			t.Errorf("accepted %s", name)
		}
	}
	good := `package p; import ("os"; "syscall"); func f() { os.Lstat("x"); syscall.Open("x", syscall.O_EVTONLY|syscall.O_SYMLINK|(syscall.O_CLOEXEC|syscall.O_NONBLOCK), 0) }`
	if findings := checkWriteCalls("good.go", good); len(findings) != 0 {
		t.Fatalf("good rejected: %v", findings)
	}
}
