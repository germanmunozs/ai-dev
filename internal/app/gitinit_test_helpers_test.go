package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"ai-dev/internal/gitmanager"
	"ai-dev/internal/gitstore"
)

// countingRandom counts the bytes drawn from the injected random source.
type countingRandom struct {
	mu    sync.Mutex
	bytes int
}

func (r *countingRandom) Read(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, err := rand.Read(b)
	r.bytes += n
	return n, err
}

func (r *countingRandom) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bytes
}

type appRefReader struct{ calls int }

func (r *appRefReader) Read(_ context.Context, request gitmanager.GitReadRequest) (gitmanager.GitRunResult, error) {
	r.calls++
	return gitmanager.GitRunResult{}, nil
}

type appArgvBuilder struct{}

func (appArgvBuilder) BuildCommand(c gitmanager.GitCommand) (gitmanager.GitInvocation, error) {
	if c.Kind == gitmanager.GitCommandInit {
		return gitmanager.GitInvocation{Args: []string{"init", "--template=", "--object-format=sha1", "--ref-format=files",
			"--initial-branch=" + string(c.BranchName), "--", string(c.PhysicalRoot)}}, nil
	}
	return gitmanager.GitInvocation{Args: []string{string(c.Kind), string(c.BranchName), c.ConfigKey, c.ConfigValue}}, nil
}

// recordingExecutor completes every operation and records what ran.
type recordingExecutor struct {
	mu       sync.Mutex
	executed int
}

func (e *recordingExecutor) Preflight(context.Context, gitmanager.GitInitializationPlan) *gitmanager.PlanRejection {
	return nil
}

func (e *recordingExecutor) Revalidate(context.Context, gitmanager.GitInitializationPlan, gitmanager.DerivedOperation) *gitmanager.PlanRejection {
	return nil
}
func (e *recordingExecutor) Execute(context.Context, gitmanager.MutationPermit, gitmanager.GitInitializationPlan, gitmanager.DerivedOperation) gitmanager.OperationOutcome {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.executed++
	return gitmanager.OperationCompleted
}
func (e *recordingExecutor) Verify(context.Context, gitmanager.GitInitializationPlan) gitmanager.ApplyEvidence {
	return gitmanager.ApplyEvidence{VerifiedComplete: true}
}

// gitAppFixture is a composition with fake Git, the real store and
// filesystem observation, and a recording executor.
type gitAppFixture struct {
	t         *testing.T
	base      string
	project   string
	home      string
	stateRoot string
	random    *countingRandom
	reader    *appRefReader
	executor  *recordingExecutor
	identify  int
	deps      GitManagementDependencies
}

func newGitAppFixture(t *testing.T) *gitAppFixture {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Plan export and apply need a production filesystem: APFS on macOS,
	// local ext4 on Linux. A tmpfs temporary directory is correctly refused.
	if obs, err := gitstore.ObserveFilesystem(base); err != nil || !obs.Local ||
		obs.FilesystemType != map[string]string{"darwin": "apfs", "linux": "ext4"}[runtime.GOOS] {
		t.Skip("the temporary directory is not on a production filesystem")
	}
	f := &gitAppFixture{t: t, base: base, project: filepath.Join(base, "project"), home: filepath.Join(base, "home"),
		random: &countingRandom{}, reader: &appRefReader{}, executor: &recordingExecutor{}}
	for _, dir := range []string{f.project, f.home} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	env := map[string]string{"HOME": f.home}
	root, err := gitmanager.ResolveStateRoot(runtime.GOOS, func(name string) (string, bool) { value, ok := env[name]; return value, ok })
	if err != nil {
		t.Fatal(err)
	}
	f.stateRoot = string(root.Bytes())
	tool, err := gitmanager.NewGitToolIdentity(gitmanager.GitToolIdentityInput{
		InvocationMode: gitmanager.InvocationModeDirect, InvocationPath: "/usr/bin/git",
		InvocationPathDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", InvocationDevice: "1",
		InvocationInode: "2", EffectiveExecutablePath: "/usr/bin/git",
		EffectiveExecutableDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Version:                   "2.54.0", ExecPath: "/usr/libexec/git-core",
		Capabilities:         []string{"config-end-of-options", "initial-branch", "object-format", "path-format", "ref-format"},
		EnvironmentSelectors: map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.deps = GitManagementDependencies{
		Reader: f.reader, Commands: appArgvBuilder{}, RandomSource: f.random, ObserveFilesystem: gitstore.ObserveFilesystem,
		IdentifyGit: func(context.Context, gitmanager.EnvironmentProfile) (gitmanager.GitToolIdentity, error) {
			f.identify++
			return tool, nil
		},
		LookupEnv:  func(name string) (string, bool) { value, ok := env[name]; return value, ok },
		Environ:    func() []string { return nil },
		WorkingDir: func() (string, error) { return base, nil },
		GOOS:       runtime.GOOS,
		OpenStore: func(root gitmanager.ObservedPath) (gitmanager.GitStore, func(), error) {
			store, err := gitstore.Open(root)
			if err != nil {
				return nil, nil, err
			}
			return store, func() { store.Close() }, nil
		},
		Executor: f.executor,
	}
	return f
}

func (f *gitAppFixture) stateExists() bool {
	_, err := os.Stat(f.stateRoot)
	return err == nil
}

func (f *gitAppFixture) authorizations() int {
	matches, _ := filepath.Glob(filepath.Join(f.stateRoot, "projects", "*", "authorizations", "*.json"))
	return len(matches)
}

// scriptedTerminal answers each prompt with a function of everything the
// assistant has written so far, one line per read.
type scriptedTerminal struct {
	out     *bytes.Buffer
	answers []func(output string) string
	pending []byte
	eof     bool
}

func (s *scriptedTerminal) Read(b []byte) (int, error) {
	if len(s.pending) == 0 {
		if len(s.answers) == 0 {
			s.eof = true
			return 0, io.EOF
		}
		s.pending = []byte(s.answers[0](s.out.String()) + "\n")
		s.answers = s.answers[1:]
	}
	n := copy(b, s.pending)
	s.pending = s.pending[n:]
	return n, nil
}

func literal(answer string) func(string) string { return func(string) string { return answer } }
