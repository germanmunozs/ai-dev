//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// fakeGitReader returns a scripted result per row and records every row.
type fakeGitReader struct {
	results map[GitReadRow]GitRunResult
	calls   []GitReadRequest
}

func (f *fakeGitReader) Read(_ context.Context, request GitReadRequest) (GitRunResult, error) {
	f.calls = append(f.calls, request)
	return f.results[request.Row], nil
}

func (f *fakeGitReader) rows() []GitReadRow {
	var rows []GitReadRow
	for _, call := range f.calls {
		rows = append(rows, call.Row)
	}
	return rows
}

const testCommitOID = "f2ad6c76f0115a6ba5b00456a849810e7ec0af20"

// realStatus is "git status --porcelain=v2 -z --untracked-files=all
// --ignored=matching" captured from Git 2.54 over the files inspectFixture
// creates.
var realStatus = strings.Join([]string{
	"1 .M N... 100644 100644 100755 f2ad6c76f0115a6ba5b00456a849810e7ec0af20 f2ad6c76f0115a6ba5b00456a849810e7ec0af20 mode.sh",
	"1 .M N... 100644 100644 100644 78981922613b2afb6025042ff6bd878ac1994e85 78981922613b2afb6025042ff6bd878ac1994e85 modified.txt",
	"2 R. N... 100644 100644 100644 4286f428e3b19fe84de503916ce0e7dc8deefea1 4286f428e3b19fe84de503916ce0e7dc8deefea1 R100 renamed-new",
	"renamed-old",
	"1 A. N... 000000 100644 100644 0000000000000000000000000000000000000000 b4785957bc986dc39c629de9fac9df46972c00fc staged.txt",
	"1 .T N... 100644 100644 120000 61780798228d17af2d34fce4cfbdf35556832472 61780798228d17af2d34fce4cfbdf35556832472 typechange",
	"? untracked.txt", "! build/", "! debug.log", "",
}, "\x00")

type inspectFixture struct {
	t       *testing.T
	root    string
	home    string
	git     *fakeGitReader
	request InspectRequest
	deps    InspectorDependencies
}

func newInspectFixture(t *testing.T) *inspectFixture {
	base := physicalTempDir(t)
	root := filepath.Join(base, "project")
	home := filepath.Join(base, "home")
	mustMkdir(t, root)
	mustMkdir(t, home)
	git := &fakeGitReader{results: map[GitReadRow]GitRunResult{}}
	f := &inspectFixture{t: t, root: root, home: home, git: git}
	f.request = InspectRequest{
		ProjectDir: root, WorkingDir: "/", Tool: mustToolIdentity(t), Environment: validEnvironmentProfile(),
		User: UserContext{Home: []byte(home)}, InspectionID: "inspection-1", InspectedAt: time.Unix(100, 0).UTC(),
	}
	f.deps = InspectorDependencies{
		Git: git,
		ObserveFilesystem: func(path string) (FilesystemObservation, error) {
			return FilesystemObservation{GOOS: "darwin", FilesystemType: "apfs", MountIdentity: "/ fsid=1:2", DeviceDecimal: "1", InodeDecimal: "2", Local: true}, nil
		},
		layout: func(GitToolIdentity) (toolchainLayout, bool) {
			return toolchainLayout{ConfigFiles: [][]byte{[]byte(filepath.Join(base, "toolchain", "gitconfig"))}}, true
		},
	}
	return f
}

// repository makes the project a normal repository whose Git rows report
// an attached HEAD and the captured status.
func (f *inspectFixture) repository() {
	gitDir := filepath.Join(f.root, ".git")
	fakeGitDir(f.t, gitDir)
	mustWrite(f.t, filepath.Join(f.root, ".gitignore"), "build/\n*.log\n")
	for _, name := range []string{"mode.sh", "modified.txt", "renamed-new", "staged.txt", "untracked.txt", "build/out", "debug.log"} {
		mustWrite(f.t, filepath.Join(f.root, name), name+"\n")
	}
	if err := os.Symlink(filepath.Join(f.root, "modified.txt"), filepath.Join(f.root, "typechange")); err != nil {
		f.t.Fatal(err)
	}
	f.git.results[GitReadDiscover] = GitRunResult{Stdout: []byte(gitDir + "\n" + gitDir + "\n" + f.root + "\n")}
	f.git.results[GitReadHeadSymbolicRef] = GitRunResult{Stdout: []byte("refs/heads/main\n")}
	f.git.results[GitReadHeadCommit] = GitRunResult{Stdout: []byte(testCommitOID + "\n")}
	f.git.results[GitReadShowRefs] = GitRunResult{Stdout: []byte(testCommitOID + " HEAD\n" + testCommitOID + " refs/heads/main\n" +
		testOtherSHA1OID + " refs/tags/v1\n" + testCommitOID + " refs/tags/v1^{}\n")}
	f.git.results[GitReadStatus] = GitRunResult{Stdout: []byte(realStatus), StdoutObserved: int64(len(realStatus))}
}

func (f *inspectFixture) inspect() GitInspection {
	f.t.Helper()
	inspection, err := Inspect(context.Background(), f.deps, f.request)
	if err != nil {
		f.t.Fatal(err)
	}
	return inspection
}

func incompatibilityReasons(inspection GitInspection) []Reason {
	var reasons []Reason
	for _, item := range inspection.Incompatibilities() {
		reasons = append(reasons, item.Reason)
	}
	return reasons
}

func TestInspectAbsentProject(t *testing.T) {
	f := newInspectFixture(t)
	inspection := f.inspect()
	observation := inspection.Observation
	if observation.RepositoryKind != RepositoryKindAbsent || observation.Head.State != HeadStateUnborn ||
		observation.RefsObservationState != ObservationNotApplicable || observation.ConversionSettings.Risk != ConversionRiskNone ||
		observation.FilesystemCapabilitiesObservationState != ObservationObserved {
		t.Fatalf("observation %+v", observation)
	}
	if len(f.git.calls) != 0 {
		t.Fatalf("Git ran for an absent repository: %v", f.git.rows())
	}
	if len(inspection.Incompatibilities()) != 0 || inspection.ProjectIdentity.Validate() != nil {
		t.Fatalf("incompatibilities %v", inspection.Incompatibilities())
	}
	if observation.ProjectConfig.State != ProjectConfigAbsent {
		t.Fatalf("project config %+v", observation.ProjectConfig)
	}
}

func TestInspectNormalRepository(t *testing.T) {
	f := newInspectFixture(t)
	f.repository()
	inspection := f.inspect()
	observation := inspection.Observation
	want := []GitReadRow{GitReadDiscover, GitReadShowRefs, GitReadHeadSymbolicRef, GitReadHeadCommit, GitReadStatus}
	if !reflect.DeepEqual(f.git.rows(), want) {
		t.Fatalf("rows %v", f.git.rows())
	}
	if observation.RepositoryKind != RepositoryKindNormal || observation.Head.State != HeadStateAttached ||
		observation.Head.CommitOID != testCommitOID || observation.ObjectFormat != ObjectFormatSHA1 {
		t.Fatalf("observation %+v", observation)
	}
	refs := observation.Refs()
	if len(refs) != 2 || refs[0].Name.DisplayUTF8 != "refs/heads/main" || refs[1].PeeledOID != testCommitOID {
		t.Fatalf("refs %+v", refs)
	}
	var changes []string
	for _, change := range observation.WorktreeChanges() {
		changes = append(changes, string(change.Path.Bytes())+":"+string(change.Kind))
	}
	wantChanges := []string{"build/:ignored", "debug.log:ignored", "mode.sh:mode_changed", "modified.txt:unstaged", "typechange:type_changed", "untracked.txt:untracked"}
	if !reflect.DeepEqual(changes, wantChanges) {
		t.Fatalf("worktree changes %v", changes)
	}
	if len(observation.WorktreePathIdentities()) != len(wantChanges) {
		t.Fatalf("identities %+v", observation.WorktreePathIdentities())
	}
	// Ignore evaluation agreed with Git for every untracked and ignored path.
	if len(inspection.Incompatibilities()) != 0 || len(observation.IgnoreEvaluations()) != 3 {
		t.Fatalf("incompatibilities %v, evaluations %d", inspection.Incompatibilities(), len(observation.IgnoreEvaluations()))
	}
	// Only read rows ran, with the private environment and the project as
	// working directory.
	for _, call := range f.git.calls {
		if !call.Row.Valid() || string(call.Directory) != f.root {
			t.Fatalf("call %+v", call)
		}
	}
}

func TestInspectStatusIndexChanges(t *testing.T) {
	entries, err := parseStatus([]byte(realStatus))
	if err != nil || len(entries) != 8 {
		t.Fatalf("entries %d %v", len(entries), err)
	}
	// Git reports staged changes but there is no index file: the
	// contradiction makes the index unreadable rather than absent.
	f := newInspectFixture(t)
	f.repository()
	inspection := f.inspect()
	if inspection.Observation.Index.Presence != IndexUnreadable {
		t.Fatalf("index %+v", inspection.Observation.Index)
	}
	// With a real index the staged changes are attached to it.
	f = newInspectFixture(t)
	f.repository()
	data, err := os.ReadFile(filepath.Join("testdata", "index-v2-tree"))
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(f.root, ".git", "index"), string(data))
	index := f.inspect().Observation.Index
	var kinds []string
	for _, change := range index.Changes() {
		kinds = append(kinds, string(change.Path.Bytes())+":"+string(change.Kind))
	}
	if index.Presence != IndexPresent || !reflect.DeepEqual(kinds, []string{"renamed-new:staged", "staged.txt:staged"}) {
		t.Fatalf("index %+v %v", index, kinds)
	}
	if _, err := parseStatus([]byte("x bogus\x00")); err == nil {
		t.Fatal("malformed status accepted")
	}
}

func TestInspectHeadStates(t *testing.T) {
	for name, test := range map[string]struct {
		symbolic, commit GitRunResult
		refs             string
		want             HeadState
		history          bool
	}{
		"unborn":              {GitRunResult{Stdout: []byte("refs/heads/main\n")}, GitRunResult{ExitCode: 128}, "", HeadStateUnborn, false},
		"unborn with history": {GitRunResult{Stdout: []byte("refs/heads/new\n")}, GitRunResult{ExitCode: 128}, testCommitOID + " refs/heads/main\n", HeadStateUnborn, true},
		"detached":            {GitRunResult{ExitCode: 1}, GitRunResult{Stdout: []byte(testCommitOID + "\n")}, testCommitOID + " refs/heads/main\n", HeadStateDetached, true},
		"invalid":             {GitRunResult{ExitCode: 128}, GitRunResult{ExitCode: 128}, "", HeadStateInvalid, false},
	} {
		t.Run(name, func(t *testing.T) {
			f := newInspectFixture(t)
			f.repository()
			f.git.results[GitReadHeadSymbolicRef], f.git.results[GitReadHeadCommit] = test.symbolic, test.commit
			f.git.results[GitReadShowRefs] = GitRunResult{Stdout: []byte(test.refs), ExitCode: map[bool]int{true: 0, false: 1}[test.refs != ""]}
			head := f.inspect().Observation.Head
			if head.State != test.want || head.HistoryPresent != test.history {
				t.Fatalf("head %+v", head)
			}
		})
	}
}

// Git never runs when the configuration cannot be proven inert, nor for a
// repository that is not the project's own.
func TestInspectDoesNotRunGitWhenUnsafe(t *testing.T) {
	t.Run("configuration error", func(t *testing.T) {
		f := newInspectFixture(t)
		f.repository()
		mustWrite(t, filepath.Join(f.home, ".gitconfig"), "[core\n")
		inspection := f.inspect()
		if len(f.git.calls) != 0 || inspection.Observation.Head.State != HeadStateInvalid ||
			!containsValue(incompatibilityReasons(inspection), ReasonAttributesIncompatible) {
			t.Fatalf("rows %v, head %+v, incompatibilities %v", f.git.rows(), inspection.Observation.Head, inspection.Incompatibilities())
		}
	})
	t.Run("ancestor", func(t *testing.T) {
		f := newInspectFixture(t)
		fakeGitDir(t, filepath.Join(filepath.Dir(f.root), ".git"))
		inspection := f.inspect()
		if len(f.git.calls) != 0 || inspection.Observation.RepositoryKind != RepositoryKindAncestor ||
			!containsValue(incompatibilityReasons(inspection), ReasonAncestorRepository) {
			t.Fatalf("rows %v, kind %s", f.git.rows(), inspection.Observation.RepositoryKind)
		}
	})
	t.Run("unknown toolchain layout", func(t *testing.T) {
		f := newInspectFixture(t)
		f.repository()
		f.deps.layout = func(GitToolIdentity) (toolchainLayout, bool) { return toolchainLayout{}, false }
		if f.inspect(); len(f.git.calls) != 0 {
			t.Fatalf("rows %v", f.git.rows())
		}
	})
}

func TestInspectDetectsDisagreements(t *testing.T) {
	t.Run("ignore classification", func(t *testing.T) {
		f := newInspectFixture(t)
		f.repository()
		mustWrite(t, filepath.Join(f.root, ".gitignore"), "build/\n")
		inspection := f.inspect()
		if !containsValue(incompatibilityReasons(inspection), ReasonAttributesIncompatible) {
			t.Fatalf("incompatibilities %v", inspection.Incompatibilities())
		}
	})
	t.Run("repository location", func(t *testing.T) {
		f := newInspectFixture(t)
		f.repository()
		f.git.results[GitReadDiscover] = GitRunResult{Stdout: []byte("/elsewhere/.git\n/elsewhere/.git\n/elsewhere\n")}
		if !containsValue(incompatibilityReasons(f.inspect()), ReasonRepositoryStateIncompatible) {
			t.Fatal("location disagreement not reported")
		}
	})
}

func TestInspectRedactsAndClassifiesConfiguration(t *testing.T) {
	f := newInspectFixture(t)
	f.repository()
	mustWrite(t, filepath.Join(f.root, ".git", "config"), strings.Join([]string{
		"[remote \"origin\"]", "\turl = https://alice:tok3n@example.com/r.git",
		"[gitflow \"branch\"]", "\tmaster = main", "\tdevelop = develop",
		"[gitflow \"prefix\"]", "\tfeature = feature/", "\tbugfix = bugfix/", "\trelease = release/", "\thotfix = hotfix/",
		"\tsupport = support/", "\tversiontag = v", "",
	}, "\n"))
	inspection := f.inspect()
	remotes := inspection.RemotesRedacted()
	if len(remotes) != 1 || remotes[0].FetchURLs()[0].DisplayRedacted != "https://<redacted>@example.com/r.git" {
		t.Fatalf("remotes %+v", remotes)
	}
	for _, entry := range inspection.Observation.EffectiveConfiguration() {
		if strings.Contains(entry.ValueRedacted, "tok3n") || strings.Contains(entry.ValueRedacted, "alice") {
			t.Fatalf("secret in %+v", entry)
		}
	}
	flow := inspection.Observation.GitFlowEvidence
	if flow.Classification != GitFlowCompleteAVH || flow.ProductionBranch != "main" || flow.DevelopmentBranch != "develop" {
		t.Fatalf("Git Flow evidence %+v", flow)
	}
}

func TestClassifyGitFlowProfiles(t *testing.T) {
	evaluate := func(content string) GitFlowEvidenceInput {
		f := newConfigFixture(t)
		f.write(filepath.Join(f.gitDir, "config"), content)
		config := f.evaluate()
		var local *InspectedSourceIdentity
		for i := range config.Sources {
			if config.Sources[i].Scope == "local" {
				local = &config.Sources[i]
			}
		}
		in, ok := classifyGitFlow(config, local)
		if !ok {
			t.Fatalf("not classified: %s", content)
		}
		if _, err := NewGitFlowEvidence(in); err != nil {
			t.Fatalf("%s: %v", content, err)
		}
		return in
	}
	next := gitFlowNextProfile("main", "develop", map[string]string{"feature": "feature/", "bugfix": "bugfix/", "release": "release/", "hotfix": "hotfix/", "support": "support/"}, "v")
	var lines []string
	for key, value := range next {
		parts := strings.Split(key, ".")
		if len(parts) == 2 {
			lines = append(lines, "[gitflow]\n\t"+parts[1]+" = "+value)
		} else {
			lines = append(lines, "[gitflow \""+parts[1]+"."+parts[2]+"\"]\n\t"+parts[3]+" = "+value)
		}
	}
	for name, test := range map[string]struct {
		content string
		want    GitFlowClassification
	}{
		"absent":         {"[core]\n\tbare = false\n", GitFlowAbsent},
		"partial next":   {"[gitflow]\n\tversion = 1.0\n\tinitialized = true\n", GitFlowPartial},
		"complete next":  {strings.Join(lines, "\n") + "\n", GitFlowCompleteGitFlowNext},
		"partial AVH":    {"[gitflow \"branch\"]\n\tmaster = main\n", GitFlowPartial},
		"mixed":          {"[gitflow \"branch\"]\n\tmaster = main\n[gitflow]\n\tversion = 1.0\n", GitFlowMixed},
		"unrecognized":   {"[gitflow]\n\tsomething = x\n", GitFlowUnrecognized},
		"contradictory":  {"[gitflow]\n\tversion = 1.0\n\tversion = 2.0\n", GitFlowContradictory},
		"instance bases": {"[gitflow]\n\tversion = 1.0\n[gitflow \"branch.feature/x\"]\n\tbase = develop\n", GitFlowPartial},
	} {
		if got := evaluate(test.content); got.Classification != test.want {
			t.Errorf("%s: %s (%v), want %s", name, got.Classification, got.Issues, test.want)
		}
	}
}

// Indexes written by Git 2.54: version 2 with a cache-tree extension, and
// the same index in version 4, which is not parsed.
func TestReadIndexFromGit(t *testing.T) {
	dir := physicalTempDir(t)
	for name, check := range map[string]func(IndexObservationInput, string){
		"index-v2-tree": func(in IndexObservationInput, issue string) {
			if in.Presence != IndexPresent || in.Version != 2 || in.EntryCountDecimal != "2" || !reflect.DeepEqual(in.Extensions, []string{"TREE"}) || issue != "" {
				t.Fatalf("v2 %+v %q", in, issue)
			}
		},
		"index-v4": func(in IndexObservationInput, issue string) {
			if in.Presence != IndexUnreadable || issue == "" {
				t.Fatalf("v4 %+v %q", in, issue)
			}
		},
	} {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		mustWrite(t, filepath.Join(dir, name, "index"), string(data))
		in, issue := readIndex(filepath.Join(dir, name), 20)
		check(in, issue)
	}
	truncated, _ := os.ReadFile(filepath.Join("testdata", "index-v2-tree"))
	mustWrite(t, filepath.Join(dir, "truncated", "index"), string(truncated[:40]))
	if in, _ := readIndex(filepath.Join(dir, "truncated"), 20); in.Presence != IndexUnreadable {
		t.Fatalf("truncated %+v", in)
	}
	if in, _ := readIndex(filepath.Join(dir, "none"), 20); in.Presence != IndexAbsent {
		t.Fatalf("missing %+v", in)
	}
}
