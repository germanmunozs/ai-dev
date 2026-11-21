package gitmanager_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/germanmunozs/ai-dev/internal/gitmanager"
	"github.com/germanmunozs/ai-dev/internal/gitprocess"
)

type goFile struct {
	path       string
	production bool
	ast        *ast.File
	source     string
}

// productionAndTests parses every Go file under cmd and internal.
func parsedTree(t *testing.T) []goFile {
	t.Helper()
	root := filepath.Clean(filepath.Join("..", ".."))
	var files []goFile
	for _, dir := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), path, data, parser.ImportsOnly|parser.ParseComments)
			if err != nil {
				return err
			}
			if full, err := parser.ParseFile(token.NewFileSet(), path, data, 0); err == nil {
				parsed = full
			}
			rel, _ := filepath.Rel(root, path)
			files = append(files, goFile{path: filepath.ToSlash(rel), production: !strings.HasSuffix(path, "_test.go"), ast: parsed, source: string(data)})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return files
}

// T112/F008-FR-141, 165: the MutationPermit is built only by the safety
// kernel. No production file outside safety.go writes a permit literal,
// the controlled test constructor exists only in a _test.go file, and no
// adapter can reach the unexported fields through unsafe or reflection.
func TestMutationPermitCannotBeBypassed(t *testing.T) {
	testConstructor := 0
	for _, file := range parsedTree(t) {
		ast.Inspect(file.ast, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.CompositeLit:
				if ident, ok := n.Type.(*ast.Ident); ok && ident.Name == "MutationPermit" && len(n.Elts) > 0 &&
					file.production && file.path != "internal/gitmanager/safety.go" {
					t.Errorf("%s builds a MutationPermit", file.path)
				}
				if sel, ok := n.Type.(*ast.SelectorExpr); ok && sel.Sel.Name == "MutationPermit" && len(n.Elts) > 0 {
					t.Errorf("%s builds a gitmanager.MutationPermit", file.path)
				}
			case *ast.FuncDecl:
				if n.Name.Name == "newTestMutationPermit" {
					testConstructor++
					if file.production {
						t.Errorf("the controlled permit constructor is in production file %s", file.path)
					}
				}
			}
			return true
		})
		if !file.production {
			continue
		}
		for _, imported := range file.ast.Imports {
			// The one exception is the terminal ioctl, which passes a
			// pointer to its own termios value and never sees a permit.
			if path, _ := strconv.Unquote(imported.Path.Value); path == "unsafe" &&
				(file.path != "internal/app/terminal_posix.go" || strings.Contains(file.source, "MutationPermit")) {
				t.Errorf("%s imports unsafe", file.path)
			}
		}
		if strings.Contains(file.source, "reflect.NewAt") || strings.Contains(file.source, "FieldByName(\"session\")") {
			t.Errorf("%s reaches unexported fields by reflection", file.path)
		}
	}
	if testConstructor != 1 {
		t.Fatalf("%d controlled permit constructors, want exactly one", testConstructor)
	}
}

// T112: every mutating dispatcher entrypoint validates the permit before
// it can reach Git or the filesystem: Execute checks it first, and each
// mutating helper is reached only from Execute.
func TestEveryMutationRequiresThePermit(t *testing.T) {
	mutators := map[string]bool{"createObjects": true, "installIndex": true, "createDirectory": true, "writeConfig": true, "run": true}
	for _, file := range parsedTree(t) {
		if !file.production || !strings.HasPrefix(file.path, "internal/gitmanager/") {
			continue
		}
		for _, decl := range file.ast.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Recv == nil {
				continue
			}
			if fn.Name.Name == "Execute" && strings.Contains(exprString(fn.Recv.List[0].Type), "OperationDispatcher") {
				first, ok := fn.Body.List[0].(*ast.IfStmt)
				if !ok || !strings.Contains(file.source[first.Pos()-file.ast.FileStart:first.End()-file.ast.FileStart], "permit.authorizesPlanned") {
					t.Errorf("OperationDispatcher.Execute does not check the permit first")
				}
			}
			if mutators[fn.Name.Name] && strings.Contains(exprString(fn.Recv.List[0].Type), "OperationDispatcher") {
				params := fn.Type.Params.List
				if len(params) < 2 || exprString(params[1].Type) != "MutationPermit" {
					t.Errorf("%s does not take the MutationPermit", fn.Name.Name)
				}
			}
		}
	}
}

func exprString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.StarExpr:
		return "*" + exprString(v.X)
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return exprString(v.X) + "." + v.Sel.Name
	}
	return ""
}

// T112/F008-FR-130–136: no Git row can contact a remote, prompt for a
// credential, run a configured command or interpolate a shell. Every row of
// the closed matrix is built and its subcommand checked against the local
// plumbing set, under an environment that disables prompts and helpers.
func TestNoNetworkShellOrPromptGitOperation(t *testing.T) {
	tool, err := gitmanager.NewGitToolIdentity(gitmanager.GitToolIdentityInput{
		InvocationMode: gitmanager.InvocationModeDirect, InvocationPath: "/usr/bin/git",
		InvocationPathDigest: gitmanager.ContentDigest("sha256:" + strings.Repeat("a", 64)), InvocationDevice: "1", InvocationInode: "2",
		EffectiveExecutablePath: "/usr/bin/git", EffectiveExecutableDigest: gitmanager.ContentDigest("sha256:" + strings.Repeat("a", 64)),
		Version: "2.54.0", ExecPath: "/usr/libexec/git-core",
		Capabilities:         []string{"config-end-of-options", "initial-branch", "object-format", "path-format", "ref-format"},
		EnvironmentSelectors: map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	environment, err := gitmanager.NewEnvironmentProfile(gitmanager.NewObservedPath([]byte("/var/empty/state/git")))
	if err != nil {
		t.Fatal(err)
	}
	oid := gitmanager.ObjectOID(strings.Repeat("a", 40))
	identity := gitmanager.GitIdentity{Name: "A", Email: "a@example.org"}
	params := map[gitprocess.Operation]gitprocess.Parameters{
		gitprocess.OpCheckRefFormat: {BranchName: []byte("main")}, gitprocess.OpObjectExists: {ObjectOID: oid},
		gitprocess.OpObjectType: {ObjectOID: oid}, gitprocess.OpObjectContent: {ObjectOID: oid},
		gitprocess.OpInit:     {BranchName: []byte("main"), PhysicalRoot: []byte("/p")},
		gitprocess.OpHashBlob: {Stdin: []byte("x")}, gitprocess.OpHashTree: {Stdin: []byte("x")},
		gitprocess.OpCommitTree:       {TreeOID: oid, Author: identity, Committer: identity, AuthorDate: time.Unix(1, 0), CommitterDate: time.Unix(1, 0), Stdin: []byte("m\n")},
		gitprocess.OpUpdateRef:        {RefName: []byte("refs/heads/main"), ObjectOID: oid},
		gitprocess.OpConfigWriteLocal: {ConfigKey: "gitflow.branch.master", ConfigValue: "main"},
		gitprocess.OpPreparedIndex:    {TreeOID: oid, IndexFile: []byte("/tmp/index")},
	}
	local := map[string]bool{"rev-parse": true, "symbolic-ref": true, "show-ref": true, "status": true, "diff": true, "check-ref-format": true,
		"cat-file": true, "config": true, "init": true, "hash-object": true, "commit-tree": true, "update-ref": true, "read-tree": true}
	for _, op := range gitprocess.Operations() {
		invocation, err := gitprocess.Build(op, gitprocess.Context{Tool: tool, Environment: environment, Directory: []byte("/p")}, params[op])
		if err != nil {
			t.Fatalf("%s: %v", op, err)
		}
		args := invocation.Args
		i := 0
		for i+1 < len(args) && args[i] == "-c" {
			i += 2
		}
		if !local[args[i]] {
			t.Errorf("%s runs subcommand %q outside the local plumbing set", op, args[i])
		}
		joined := strings.Join(args, " ")
		for _, required := range []string{"protocol.allow=never", "credential.helper=", "core.sshCommand=false", "core.askPass="} {
			if !strings.Contains(joined, "-c "+required) {
				t.Errorf("%s lacks the override %s", op, required)
			}
		}
		env := strings.Join(invocation.Environment, "\n")
		for _, required := range []string{"GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=", "SSH_ASKPASS=", "GCM_INTERACTIVE=never", "GIT_CONFIG_NOSYSTEM=1"} {
			if !strings.Contains("\n"+env+"\n", "\n"+required+"\n") {
				t.Errorf("%s environment lacks %s", op, required)
			}
		}
		if invocation.Executable != "/usr/bin/git" {
			t.Errorf("%s runs %s", op, invocation.Executable)
		}
	}
	// No production file starts a shell. The Feature 010 completion files
	// name "bash" as a completion target, so only that literal is allowed
	// in them; they must still import nothing that can start a process.
	completionTargets := map[string]bool{"internal/app/catalog.go": true, "internal/app/completion.go": true}
	for _, file := range parsedTree(t) {
		if completionTargets[file.path] {
			for _, spec := range file.ast.Imports {
				if path, _ := strconv.Unquote(spec.Path.Value); path == "os" || path == "os/exec" || path == "syscall" ||
					strings.HasSuffix(path, "/internal/system") || strings.HasSuffix(path, "/internal/gitprocess") {
					t.Errorf("%s names a shell and imports %s", file.path, path)
				}
			}
		}
		if file.production && (strings.Contains(file.source, `"/bin/sh"`) || strings.Contains(file.source, `"sh", "-c"`) ||
			(strings.Contains(file.source, `"bash"`) && !completionTargets[file.path])) {
			t.Errorf("%s may start a shell", file.path)
		}
	}
}

// T112/F008-FR-009–010, 144–145: Git Management grants no Workflow State,
// Human Gate or Feature 007 authority: its packages import none of those,
// and the strategy and reconciliation reports grant no mutation authority.
func TestNoWorkflowGateOrFeature007Authority(t *testing.T) {
	forbidden := []string{"github.com/germanmunozs/ai-dev/internal/workflow", "github.com/germanmunozs/ai-dev/internal/gate", "github.com/germanmunozs/ai-dev/internal/gatestore", "github.com/germanmunozs/ai-dev/internal/role",
		"github.com/germanmunozs/ai-dev/internal/selection", "github.com/germanmunozs/ai-dev/internal/agent", "github.com/germanmunozs/ai-dev/internal/execution"}
	for _, file := range parsedTree(t) {
		feature := strings.HasPrefix(file.path, "internal/git") || strings.HasPrefix(file.path, "internal/conventionalcommit/") ||
			file.path == "internal/app/gitapply.go" || file.path == "internal/app/gitplan.go" || file.path == "internal/app/gitreconcile.go" ||
			file.path == "internal/app/gitinit.go" || file.path == "internal/app/gitinit_wizard.go" || file.path == "internal/app/gitcompose.go"
		if !file.production || !feature {
			continue
		}
		for _, imported := range file.ast.Imports {
			path, _ := strconv.Unquote(imported.Path.Value)
			for _, prefix := range forbidden {
				if path == prefix || strings.HasPrefix(path, prefix+"/") {
					t.Errorf("%s imports %s", file.path, path)
				}
			}
		}
	}
	if (gitmanager.GitStrategyReport{}).GrantsMutationAuthority() || (gitmanager.ReconciliationReport{}).GrantsMutationAuthority() {
		t.Fatal("a report grants mutation authority")
	}
}
