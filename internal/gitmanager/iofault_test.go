//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// stepCall names the n-th call (from 1) of one filesystem step.
type stepCall struct {
	step string
	n    int
}

// stepFaults wraps the OS steps, records every call and injects one fault:
// before the step runs, or after a step with an effect has run, which is
// a failure whose effect happened and cannot be verified.
type stepFaults struct {
	counts map[string]int
	trace  []stepCall
	fault  stepCall
	err    error
	after  bool
	hit    bool
}

// effectful are the steps that change the filesystem, so a failure can be
// reported after their effect.
var effectful = map[string]bool{"chmod": true, "write": true, "sync": true, "mkdir": true, "rename": true, "link": true, "remove": true, "sync-dir": true}

func (s *stepFaults) before(step string) (stepCall, error) {
	if s.counts == nil {
		s.counts = map[string]int{}
	}
	s.counts[step]++
	call := stepCall{step, s.counts[step]}
	s.trace = append(s.trace, call)
	if call == s.fault && !s.after {
		s.hit = true
		return call, s.err
	}
	return call, nil
}

func (s *stepFaults) afterEffect(call stepCall, err error) error {
	if err == nil && call == s.fault && s.after {
		s.hit = true
		return s.err
	}
	return err
}

func (s *stepFaults) ops() fileOps {
	base := osFiles
	effect := func(step string, run func() error) error {
		call, err := s.before(step)
		if err != nil {
			return err
		}
		return s.afterEffect(call, run())
	}
	return fileOps{
		openFile: func(path string, flag int, perm os.FileMode) (*os.File, error) {
			if _, err := s.before("open"); err != nil {
				return nil, err
			}
			return base.openFile(path, flag, perm)
		},
		fstat: func(f *os.File) (syscall.Stat_t, error) {
			if _, err := s.before("fstat"); err != nil {
				return syscall.Stat_t{}, err
			}
			return base.fstat(f)
		},
		readFile: func(path string) ([]byte, error) {
			if _, err := s.before("read"); err != nil {
				return nil, err
			}
			return base.readFile(path)
		},
		chmod: func(f *os.File, mode os.FileMode) error {
			return effect("chmod", func() error { return base.chmod(f, mode) })
		},
		write: func(f *os.File, data []byte) (int, error) {
			var n int
			err := effect("write", func() (err error) { n, err = base.write(f, data); return err })
			return n, err
		},
		sync: func(f *os.File) error { return effect("sync", func() error { return base.sync(f) }) },
		mkdir: func(path string, perm os.FileMode) error {
			return effect("mkdir", func() error { return base.mkdir(path, perm) })
		},
		rename: func(oldpath, newpath string) error {
			return effect("rename", func() error { return base.rename(oldpath, newpath) })
		},
		link: func(oldpath, newpath string) error {
			return effect("link", func() error { return base.link(oldpath, newpath) })
		},
		remove:  func(path string) error { return effect("remove", func() error { return base.remove(path) }) },
		syncDir: func(path string) error { return effect("sync-dir", func() error { return base.syncDir(path) }) },
	}
}

// ioFault is one injected fault of the matrix.
type ioFault struct {
	call  stepCall
	err   error
	after bool
}

func (f ioFault) String() string {
	mode := "before"
	if f.after {
		mode = "after"
	}
	return fmt.Sprintf("%s#%d %s %v", f.call.step, f.call.n, mode, f.err)
}

// ioFaultMatrix runs a baseline that records every step, then yields one
// fault per recorded call and errno before the step, and one unverifiable
// failure after every step with an effect. ENOSPC and EDQUOT are the
// exhausted-space cases of T127.
func ioFaultMatrix(t *testing.T, baseline func(fileOps)) []ioFault {
	t.Helper()
	var record stepFaults
	baseline(record.ops())
	if len(record.trace) == 0 {
		t.Fatal("the baseline ran no filesystem step")
	}
	var faults []ioFault
	for _, call := range record.trace {
		for _, errno := range []error{syscall.EIO, syscall.ENOSPC, syscall.EDQUOT} {
			faults = append(faults, ioFault{call: call, err: errno})
		}
		if effectful[call.step] {
			faults = append(faults, ioFault{call: call, err: syscall.EIO, after: true})
		}
	}
	return faults
}

// T124/T127/AC-017/AC-025: a fault at every step of the index.lock
// protocol never yields an assumed success, a failure leaves no effect, and
// nothing ai-dev created survives the audited release unless a release
// failure or the indeterminate outcome reports it.
func TestIndexProtocolIOFaults(t *testing.T) {
	data := []byte("prepared index bytes")
	run := func(t *testing.T, files fileOps) (gitDir string, outcome OperationOutcome, releaseErr error) {
		gitDir = physicalTempDir(t)
		lock, err := acquireIndexLock(files, gitDir)
		if err != nil {
			return gitDir, installOutcome(nil, err), nil
		}
		outcome = OperationCompleted
		if err := lock.install(gitDir, data); err != nil {
			outcome = installOutcome(lock, err)
		}
		return gitDir, outcome, lock.release(context.Background())
	}
	faults := ioFaultMatrix(t, func(files fileOps) {
		if _, outcome, err := run(t, files); outcome != OperationCompleted || err != nil {
			t.Fatalf("baseline %s %v", outcome, err)
		}
	})
	for _, fault := range faults {
		t.Run(fault.String(), func(t *testing.T) {
			injected := stepFaults{fault: fault.call, err: fault.err, after: fault.after}
			gitDir, outcome, releaseErr := run(t, injected.ops())
			if !injected.hit {
				t.Fatal("the fault was not reached")
			}
			index, indexErr := os.ReadFile(filepath.Join(gitDir, "index"))
			_, lockErr := os.Lstat(filepath.Join(gitDir, "index.lock"))
			switch outcome {
			case OperationCompleted:
				t.Fatal("a fault produced an assumed success")
			case OperationFailed:
				if !os.IsNotExist(indexErr) {
					t.Fatalf("a failed installation left an index %q", index)
				}
			}
			if indexErr == nil && !bytes.Equal(index, data) {
				t.Fatalf("installed index %q", index)
			}
			if lockErr == nil && releaseErr == nil && outcome != OperationIndeterminate {
				t.Fatalf("%s left index.lock without reporting it", outcome)
			}
		})
	}
}

// T124/T127/AC-017: the prepared index lives in a fresh private directory
// that the release removes whatever step fails, unless the release reports
// that it could not.
func TestPreparedIndexIOFaults(t *testing.T) {
	run := func(t *testing.T, files fileOps) (prepared string, err, releaseErr error) {
		root := physicalTempDir(t)
		prepared = filepath.Join(root, "prepared")
		mustMkdir(t, prepared)
		p, err := newPreparedIndex(files, []byte(root), rand.Reader)
		if err != nil {
			return prepared, err, nil
		}
		// Git's read-tree writes the index; it is not a step of ai-dev.
		mustWrite(t, p.path, "index")
		_, err = p.read()
		return prepared, err, p.release(context.Background())
	}
	faults := ioFaultMatrix(t, func(files fileOps) {
		if _, err, releaseErr := run(t, files); err != nil || releaseErr != nil {
			t.Fatalf("baseline %v %v", err, releaseErr)
		}
	})
	for _, fault := range faults {
		t.Run(fault.String(), func(t *testing.T) {
			injected := stepFaults{fault: fault.call, err: fault.err, after: fault.after}
			prepared, err, releaseErr := run(t, injected.ops())
			if !injected.hit {
				t.Fatal("the fault was not reached")
			}
			if err == nil && releaseErr == nil {
				t.Fatal("a fault was neither returned nor reported by the release")
			}
			if entries, _ := os.ReadDir(prepared); len(entries) != 0 && releaseErr == nil {
				t.Fatalf("prepared storage holds %d entries without a reported release failure", len(entries))
			}
		})
	}
}

// T124/T127/AC-011: a fault at every step of the two project
// configuration operations, through the dispatcher and the audited
// release. A failed operation leaves its target absent, a configuration
// is never written with other bytes, and no temporary survives unless its
// release failed or the outcome is indeterminate. Execution stops at the
// first operation that does not complete, as the kernel does.
func TestProjectConfigIOFaults(t *testing.T) {
	type result struct {
		root       string
		outcome    OperationOutcome
		kind       OperationKind
		releaseErr error
	}
	var want []byte
	run := func(t *testing.T, files fileOps) result {
		f := newDispatcherFixture(t, basicNewRepo())
		f.d.deps.Random = rand.Reader
		f.d.fileOps = &files
		want = renderedConfig(t, f.plan)
		r := result{root: f.root, outcome: OperationCompleted}
		for _, kind := range []OperationKind{OperationKindCreateProjectConfigDirectory, OperationKindProjectConfigWrite} {
			operation := f.operation(kind)
			permit := plannedPermit(t, f.plan, operation.OperationID)
			r.kind, r.outcome = kind, f.d.Execute(context.Background(), permit, f.plan, operation)
			for _, resource := range permit.session.resources {
				r.releaseErr = errors.Join(r.releaseErr, resource.Release(context.Background()))
			}
			if r.outcome != OperationCompleted {
				break
			}
		}
		return r
	}
	faults := ioFaultMatrix(t, func(files fileOps) {
		if r := run(t, files); r.outcome != OperationCompleted || r.releaseErr != nil {
			t.Fatalf("baseline %s %s %v", r.kind, r.outcome, r.releaseErr)
		}
	})
	for _, fault := range faults {
		t.Run(fault.String(), func(t *testing.T) {
			injected := stepFaults{fault: fault.call, err: fault.err, after: fault.after}
			r := run(t, injected.ops())
			if !injected.hit {
				t.Fatal("the fault was not reached")
			}
			directory, config := filepath.Join(r.root, ".ai-dev"), filepath.Join(r.root, ProjectConfigPath)
			data, configErr := os.ReadFile(config)
			if r.outcome == OperationCompleted {
				t.Fatalf("a fault in %s produced an assumed success", r.kind)
			}
			if r.outcome == OperationFailed {
				target := map[OperationKind]string{OperationKindCreateProjectConfigDirectory: directory, OperationKindProjectConfigWrite: config}[r.kind]
				if _, err := os.Lstat(target); !os.IsNotExist(err) {
					t.Fatalf("failed %s left %s", r.kind, target)
				}
			}
			if configErr == nil && !bytes.Equal(data, want) {
				t.Fatalf("the configuration holds %q", data)
			}
			entries, _ := os.ReadDir(directory)
			for _, entry := range entries {
				if strings.HasSuffix(entry.Name(), ".tmp") && r.releaseErr == nil && r.outcome != OperationIndeterminate {
					t.Fatalf("%s left temporary %s without reporting it", r.outcome, entry.Name())
				}
			}
		})
	}
}

// indexWritingRunner stands in for Git's read-tree: it writes data as the
// index of the attempt's private directory below prepared and exits with
// exit.
type indexWritingRunner struct {
	data     []byte
	exit     int
	prepared string
}

func (r *indexWritingRunner) Run(context.Context, GitInvocation) (GitRunResult, error) {
	if r.data != nil {
		entries, err := os.ReadDir(r.prepared)
		if err != nil || len(entries) != 1 {
			return GitRunResult{}, fmt.Errorf("prepared storage %v: %v", entries, err)
		}
		if err := os.WriteFile(filepath.Join(r.prepared, entries[0].Name(), "index"), r.data, 0o600); err != nil {
			return GitRunResult{}, err
		}
	}
	return GitRunResult{ExitCode: r.exit}, nil
}

// T126/AC-017/AC-025: every refusal of the prepared-index operation before
// installation fails with the normal index untouched, and the audited
// release leaves neither index.lock nor prepared storage behind.
func TestInstallIndexRefusals(t *testing.T) {
	for name, test := range map[string]struct {
		setup  func(t *testing.T, root, state string)
		runner GitRunner
		permit bool
	}{
		"no permit":                      {runner: &indexWritingRunner{}},
		"foreign index.lock":             {setup: func(t *testing.T, root, _ string) { mustWrite(t, filepath.Join(root, ".git", "index.lock"), "foreign") }, runner: &indexWritingRunner{}, permit: true},
		"normal index appeared":          {setup: func(t *testing.T, root, _ string) { mustWrite(t, filepath.Join(root, ".git", "index"), "appeared") }, runner: &indexWritingRunner{}, permit: true},
		"no prepared storage":            {setup: func(t *testing.T, _, state string) { os.Remove(filepath.Join(state, "prepared")) }, runner: &indexWritingRunner{}, permit: true},
		"read-tree fails":                {runner: &indexWritingRunner{exit: 128}, permit: true},
		"no prepared index":              {runner: &indexWritingRunner{}, permit: true},
		"prepared index of another tree": {runner: &indexWritingRunner{data: encodeIndex(2, nil)}, permit: true},
	} {
		t.Run(name, func(t *testing.T) {
			f, state := newIndexDispatcher(t, test.runner)
			if test.setup != nil {
				test.setup(t, f.root, state)
			}
			index := f.operation(OperationKindPreparedIndex)
			permit := MutationPermit{}
			if test.permit {
				permit = plannedPermit(t, f.plan, index.OperationID)
			}
			if outcome := f.d.installIndex(context.Background(), permit, f.plan, index); outcome != OperationFailed {
				t.Fatalf("outcome %s", outcome)
			}
			if permit.session != nil {
				for _, resource := range permit.session.resources {
					if err := resource.Release(context.Background()); err != nil {
						t.Fatalf("release %s: %v", resource.Role, err)
					}
				}
			}
			if data, err := os.ReadFile(filepath.Join(f.root, ".git", "index")); err == nil && name != "normal index appeared" {
				t.Fatalf("an index was installed: %q", data)
			}
			if data, err := os.ReadFile(filepath.Join(f.root, ".git", "index.lock")); err == nil && string(data) != "foreign" {
				t.Fatalf("an owned index.lock remains: %q", data)
			}
			if entries, _ := os.ReadDir(filepath.Join(state, "prepared")); len(entries) != 0 {
				t.Fatalf("prepared storage holds %v", entries)
			}
		})
	}
}

// newIndexDispatcher plans a basic initialization with an initial commit of
// a.txt against a writable private state root, creates the planned .git
// directory and returns a dispatcher whose Git is runner.
func newIndexDispatcher(t *testing.T, runner GitRunner) (*dispatcherFixture, string) {
	t.Helper()
	f := &dispatcherFixture{planFixture: newPlanFixture(t)}
	mustWrite(t, filepath.Join(f.root, "a.txt"), "a\n")
	f.request.Candidates = CommitCandidates([]string{"a.txt"})
	req := basicNewRepo()
	req.InitialCommit = commitRequest("a.txt")
	state := physicalTempDir(t)
	mustMkdir(t, filepath.Join(state, "prepared"))
	if writer, ok := runner.(*indexWritingRunner); ok {
		writer.prepared = filepath.Join(state, "prepared")
	}
	result, rejection := Plan(context.Background(), f.deps, req, f.inspect(), environmentProfileFor(state),
		PlanMetadata{PlanID: "plan-1", GeneratedAt: time.Unix(200, 0).UTC()})
	if rejection != nil {
		t.Fatalf("plan: %s", rejection.Detail)
	}
	f.plan = result.Plan
	f.d = NewOperationDispatcher(DispatcherDependencies{Runner: runner, Commands: f.planFixture.deps.Commands, Random: rand.Reader})
	mustMkdir(t, filepath.Join(f.root, ".git"))
	return f, state
}

// plannedIndex is the version-2 index of the plan's authorized files, as
// Git's read-tree of the planned root tree writes it.
func plannedIndex(t *testing.T, plan GitInitializationPlan) []byte {
	t.Helper()
	var entries []rawEntry
	for _, file := range plan.ImmutableIntent.AuthorizedFiles() {
		mode := uint32(0o100644)
		if file.Mode == "100755" {
			mode = 0o100755
		}
		entries = append(entries, rawEntry{path: file.PathUTF8, mode: mode, oid: file.BlobOID})
	}
	return encodeIndex(2, entries)
}

// T124/T126/AC-017/AC-025: a failure while installing the verified prepared
// index, through the dispatcher: a rename that did not happen is failed and
// leaves no index, and a rename or directory sync reported failed after the
// index may be in place is indeterminate.
func TestInstallIndexInstallationFaults(t *testing.T) {
	for _, test := range []struct {
		step  string
		after bool
		want  OperationOutcome
	}{
		{"rename", false, OperationFailed},
		{"rename", true, OperationIndeterminate},
		{"sync-dir", false, OperationIndeterminate},
	} {
		t.Run(fmt.Sprintf("%s after=%v", test.step, test.after), func(t *testing.T) {
			runner := &indexWritingRunner{}
			f, state := newIndexDispatcher(t, runner)
			runner.data = plannedIndex(t, f.plan)
			faults := stepFaults{fault: stepCall{test.step, 1}, err: syscall.EIO, after: test.after}
			files := faults.ops()
			f.d.fileOps = &files
			index := f.operation(OperationKindPreparedIndex)
			permit := plannedPermit(t, f.plan, index.OperationID)
			if outcome := f.d.installIndex(context.Background(), permit, f.plan, index); outcome != test.want || !faults.hit {
				t.Fatalf("outcome %s, fault reached %v", outcome, faults.hit)
			}
			for _, resource := range permit.session.resources {
				resource.Release(context.Background())
			}
			_, indexErr := os.Lstat(filepath.Join(f.root, ".git", "index"))
			if test.want == OperationFailed && !os.IsNotExist(indexErr) {
				t.Fatal("a failed installation left an index")
			}
			if _, err := os.Lstat(filepath.Join(f.root, ".git", "index.lock")); !os.IsNotExist(err) {
				t.Fatal("index.lock remains after the release")
			}
			if entries, _ := os.ReadDir(filepath.Join(state, "prepared")); len(entries) != 0 {
				t.Fatalf("prepared storage holds %v", entries)
			}
		})
	}
}

type failingRandom struct{}

func (failingRandom) Read([]byte) (int, error) { return 0, errors.New("no entropy") }

// T126: the guards before any effect: no or a failing random source, no
// configuration directory, an ownership that cannot be registered (the
// temporary is removed by its identity) and an operation without a permit.
func TestProjectConfigAndPreparedIndexGuards(t *testing.T) {
	root := physicalTempDir(t)
	mustMkdir(t, filepath.Join(root, "prepared"))
	for _, random := range []RandomSource{nil, failingRandom{}} {
		if _, err := newPreparedIndex(osFiles, []byte(root), random); err == nil {
			t.Fatalf("prepared index with random %T", random)
		}
	}
	accept := func(*configTemporary) error { return nil }
	if err := writeProjectConfig(osFiles, root, []byte("{}\n"), rand.Reader, accept); !errors.Is(err, errLiteralFile) {
		t.Fatalf("configuration without .ai-dev: %v", err)
	}
	mustMkdir(t, filepath.Join(root, ".ai-dev"))
	for _, random := range []RandomSource{nil, failingRandom{}} {
		if err := writeProjectConfig(osFiles, root, []byte("{}\n"), random, accept); err == nil {
			t.Fatalf("configuration with random %T", random)
		}
	}
	refuse := func(*configTemporary) error { return errMutationNotPermitted }
	if err := writeProjectConfig(osFiles, root, []byte("{}\n"), rand.Reader, refuse); !errors.Is(err, errMutationNotPermitted) {
		t.Fatalf("unregistered temporary: %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(root, ".ai-dev")); len(entries) != 0 {
		t.Fatalf("an unregistered temporary remains: %v", entries)
	}
	f := newDispatcherFixture(t, basicNewRepo())
	for _, kind := range []OperationKind{OperationKindCreateProjectConfigDirectory, OperationKindProjectConfigWrite} {
		operation := f.operation(kind)
		var outcome OperationOutcome
		if kind == OperationKindCreateProjectConfigDirectory {
			outcome = f.d.createDirectory(context.Background(), MutationPermit{}, f.plan, operation)
		} else {
			outcome = f.d.writeConfig(context.Background(), MutationPermit{}, f.plan, operation)
		}
		if outcome != OperationFailed {
			t.Fatalf("%s without a permit: %s", kind, outcome)
		}
	}
	if _, err := os.Lstat(filepath.Join(f.root, ".ai-dev")); !os.IsNotExist(err) {
		t.Fatal("an operation without a permit created .ai-dev")
	}
}
