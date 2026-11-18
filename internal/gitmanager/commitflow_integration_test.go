//go:build (darwin && !ios) || (linux && !android)

package gitmanager_test

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-dev/internal/gitmanager"
	"ai-dev/internal/gitprocess"
	"ai-dev/internal/gitstore"
)

const commitAuthorizationID = "123e4567-e89b-42d3-a456-426614174081"

// commitProject is a real project whose .ai-dev/config.json already holds
// the exact rendering of a basic strategy, so the initial-commit group
// runs end to end without the project-configuration write of T093.
type commitProject struct {
	*realGit
	planned gitmanager.PlanResult
	file    []byte
}

func newCommitProject(t *testing.T, files map[string]string, includes []string) *commitProject {
	g := newRealGit(t)
	config, err := gitmanager.MarshalCanonical(map[string]any{
		"schema_version": gitmanager.ProjectConfigSchema, "project_id": "8f0e7c1a-3b2d-4e5f-9a6b-7c8d9e0f1a2b", "strategy": "basic",
		"strategy_parameters": map[string]any{"initial_branch": "main"},
	})
	if err != nil {
		t.Fatal(err)
	}
	files[gitmanager.ProjectConfigPath] = string(config)
	for name, content := range files {
		mode := os.FileMode(0o644)
		if strings.HasSuffix(name, ".sh") {
			mode = 0o755
		}
		if err := os.MkdirAll(filepath.Dir(filepath.Join(g.project, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(g.project, name), []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	p := &commitProject{realGit: g}
	candidates := gitmanager.CommitCandidates(includes)
	inspection, err := g.inspectCandidates(context.Background(), candidates)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range inspection.Incompatibilities() {
		if item.Reason == gitmanager.ReasonFilesystemUnsupported {
			t.Skip("the temporary directory is not on a production filesystem")
		}
	}
	at := time.Date(2026, 11, 18, 10, 30, 0, 0, time.FixedZone("", 2*3600))
	planner := gitmanager.PlannerDependencies{Git: g.reader, Commands: gitprocess.Builder{}, Random: rand.Reader}
	planned, rejection := gitmanager.Plan(context.Background(), planner, gitmanager.StrategyRequest{
		Strategy: "basic", InitializeRepository: true, Branches: map[string]string{"initial-branch": "main"},
		InitialCommit: &gitmanager.CommitRequest{
			MessageBytes: []byte("chore: initial commit\n"), Includes: includes,
			Author: gitmanager.GitIdentity{Name: "Ada Lovelace", Email: "ada@example.org"}, AuthorDate: at,
			Committer: gitmanager.GitIdentity{Name: "Ada Lovelace", Email: "ada@example.org"}, CommitterDate: at,
		},
	}, inspection, g.environment, gitmanager.PlanMetadata{PlanID: "plan-1", GeneratedAt: time.Unix(200, 0).UTC()})
	if rejection != nil {
		t.Fatalf("planning refused: %s: %s %+v", rejection.Reason, rejection.Detail, rejection.Findings)
	}
	if p.file, err = gitmanager.EncodePlanFile(planned.Plan); err != nil {
		t.Fatal(err)
	}
	p.planned = planned
	return p
}

func (p *commitProject) apply(t *testing.T, store gitmanager.GitStore, authorizationID string) gitmanager.GitApplyResult {
	var normal, ignored []string
	for _, file := range p.planned.Plan.ImmutableIntent.AuthorizedFiles() {
		tuple := file.PathUTF8 + "|" + file.Mode + "|" + string(file.ContentSHA256) + "|blob:" + string(file.BlobOID)
		if file.Ignored {
			ignored = append(ignored, tuple)
		} else {
			normal = append(normal, tuple)
		}
	}
	inspect := func(ctx context.Context, _ []byte, candidates [][]byte) (gitmanager.GitInspection, error) {
		return p.inspectCandidates(ctx, candidates)
	}
	planner := gitmanager.PlannerDependencies{Git: p.reader, Commands: gitprocess.Builder{}, Random: rand.Reader}
	return gitmanager.Apply(context.Background(), gitmanager.ApplyDependencies{
		Store: store, Clock: fixedTime(time.Unix(1000, 0).UTC()), Random: rand.Reader, Planner: planner, Tool: p.tool,
		Environment: p.environment, Inspect: inspect,
		Executor: gitmanager.NewOperationDispatcher(gitmanager.DispatcherDependencies{
			Runner: gitprocess.Runner{}, Commands: gitprocess.Builder{}, Inspect: inspect, Random: rand.Reader}),
	}, gitmanager.ApplyRequest{PlanFile: p.file, ConfirmPlan: string(p.planned.Plan.PlanDigest), AuthorizationID: authorizationID,
		Issue: &gitmanager.IssueRequest{AuthorizedBy: "operator", ConfirmInitialCommit: true, Approvals: normal, IgnoredApprovals: ignored}})
}

func (p *commitProject) store(t *testing.T) *gitstore.Store {
	store, err := gitstore.Open(gitmanager.NewObservedPath([]byte(p.state)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// status runs the ordinary read-only status row through the real toolchain.
func (p *commitProject) status(t *testing.T) string {
	result, err := p.reader.Read(context.Background(), gitmanager.GitReadRequest{Row: gitmanager.GitReadStatus, Tool: p.tool,
		Environment: p.environment, Directory: []byte(p.project)})
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("status: %v %+v", err, result)
	}
	return string(result.Stdout)
}

// T081/AC-010/AC-028: the complete flow creates exactly the planned
// objects, the initial ref by CAS and a coherent index; authorized paths
// are clean, an unauthorized file stays untracked, nothing is rolled back.
func TestInitialCommitFlowWithRealGit(t *testing.T) {
	p := newCommitProject(t, map[string]string{
		"README.md": "# project\n", "docs/guide.md": "guide\n", "bin/run.sh": "#!/bin/sh\necho run\n", "notes.txt": "not committed\n",
	}, []string{"README.md", "docs/guide.md", "bin/run.sh"})
	result := p.apply(t, p.store(t), commitAuthorizationID)
	if result.Status != gitmanager.ApplyStatusApplied || result.ExitCode != 0 {
		t.Fatalf("result %+v", result)
	}
	gitDir := filepath.Join(p.project, ".git")
	head, _ := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	ref, _ := os.ReadFile(filepath.Join(gitDir, "refs", "heads", "main"))
	var commit string
	for _, operation := range p.planned.Plan.DerivedOperations() {
		if operation.Kind == gitmanager.OperationKindInitialCommit {
			commit = strings.TrimPrefix(operation.TargetIdentity, "commit:")
			argv := strings.Join(operation.ExactArgv(), " ")
			if !strings.Contains(argv, "-c commit.gpgSign=false commit-tree ") || strings.Contains(argv, " -S") {
				t.Fatalf("commit argv %s", argv)
			}
		}
	}
	// The stored commit is exactly the frozen identities, dates and
	// message, unsigned (F008-FR-096–108).
	compressed, err := os.ReadFile(filepath.Join(gitDir, "objects", commit[:2], commit[2:]))
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	object, _ := io.ReadAll(reader)
	if !bytes.Contains(object, []byte("\x00tree ")) || !bytes.Contains(object, []byte("\nauthor Ada Lovelace <ada@example.org> 1794990600 +0200\n")) ||
		bytes.Contains(object, []byte("gpgsig")) || bytes.Contains(object, []byte("parent ")) || !bytes.HasSuffix(object, []byte("\n\nchore: initial commit\n")) {
		t.Fatalf("commit object %q", object)
	}
	if string(result.CommitOID) != commit || result.IndexTreeOID == "" {
		t.Fatalf("result commit %s tree %s", result.CommitOID, result.IndexTreeOID)
	}
	if string(head) != "ref: refs/heads/main\n" || string(ref) != commit+"\n" {
		t.Fatalf("HEAD %q main %q, planned commit %s", head, ref, commit)
	}
	if _, err := os.Lstat(filepath.Join(gitDir, "logs", "refs", "heads", "main")); !os.IsNotExist(err) {
		t.Fatalf("the initial ref has a reflog: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(gitDir, "index.lock")); !os.IsNotExist(err) {
		t.Fatalf("index.lock remains: %v", err)
	}
	if status := p.status(t); status != "? notes.txt\x00" {
		t.Fatalf("status %q", status)
	}
	if entries, _ := os.ReadDir(filepath.Join(p.state, "prepared")); len(entries) != 0 {
		t.Fatalf("prepared index storage was not released: %v", entries)
	}

	// The same authorization is consumed; a new one finds the plan stale.
	again := p.apply(t, p.store(t), "123e4567-e89b-42d3-a456-426614174082")
	if again.Status != gitmanager.ApplyStatusRefused || again.Reason != gitmanager.ReasonPlanStale {
		t.Fatalf("second apply %+v", again)
	}
}

// T079/AC-008: a secret in the last allowlisted file refuses the plan with
// only its path and category, before any repository exists.
func TestInitialCommitSecretInLastFile(t *testing.T) {
	g := newRealGit(t)
	secret := "-----BEGIN " + "OPENSSH PRIVATE KEY-----\nAAAA\n"
	for name, content := range map[string]string{"a.txt": "fine\n", "z.txt": secret} {
		if err := os.WriteFile(filepath.Join(g.project, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	includes := []string{"a.txt", "z.txt"}
	inspection, err := g.inspectCandidates(context.Background(), gitmanager.CommitCandidates(includes))
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range inspection.Incompatibilities() {
		if item.Reason == gitmanager.ReasonFilesystemUnsupported {
			t.Skip("the temporary directory is not on a production filesystem")
		}
	}
	at := time.Unix(1_800_000_000, 0).UTC()
	_, rejection := gitmanager.Plan(context.Background(),
		gitmanager.PlannerDependencies{Git: g.reader, Commands: gitprocess.Builder{}, Random: rand.Reader},
		gitmanager.StrategyRequest{Strategy: "basic", InitializeRepository: true, Branches: map[string]string{"initial-branch": "main"},
			InitialCommit: &gitmanager.CommitRequest{MessageBytes: []byte("chore: init\n"), Includes: includes,
				Author: gitmanager.GitIdentity{Name: "A", Email: "a@example.org"}, AuthorDate: at,
				Committer: gitmanager.GitIdentity{Name: "A", Email: "a@example.org"}, CommitterDate: at}},
		inspection, g.environment, gitmanager.PlanMetadata{PlanID: "plan-1", GeneratedAt: at})
	if rejection == nil || rejection.Reason != gitmanager.ReasonSensitiveContentSuspected || len(rejection.Findings) != 1 ||
		rejection.Findings[0].Path.DisplayPathUTF8 != "z.txt" || rejection.Findings[0].Category != "private_key" {
		t.Fatalf("rejection %+v", rejection)
	}
	if strings.Contains(rejection.Detail, "OPENSSH") {
		t.Fatal("the rejection quotes the secret")
	}
	if _, err := os.Lstat(filepath.Join(g.project, ".git")); !os.IsNotExist(err) {
		t.Fatalf("planning created .git: %v", err)
	}
}

// T090/T091/T095/AC-011/AC-023/AC-028: in a new project one apply writes the
// eight AVH keys one by one, creates .ai-dev and writes the configuration,
// commits it with the approved files, and only then creates develop at the
// planned commit by a separate CAS. No hooks key is written.
func TestGitFlowNewProjectWithInitialCommit(t *testing.T) {
	g := newRealGit(t)
	if err := os.WriteFile(filepath.Join(g.project, "README.md"), []byte("# flow\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	includes := []string{"README.md"}
	inspection, err := g.inspectCandidates(context.Background(), gitmanager.CommitCandidates(includes))
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range inspection.Incompatibilities() {
		if item.Reason == gitmanager.ReasonFilesystemUnsupported {
			t.Skip("the temporary directory is not on a production filesystem")
		}
	}
	at := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	identity := gitmanager.GitIdentity{Name: "Grace Hopper", Email: "grace@example.org"}
	planner := gitmanager.PlannerDependencies{Git: g.reader, Commands: gitprocess.Builder{}, Random: rand.Reader}
	planned, rejection := gitmanager.Plan(context.Background(), planner, gitmanager.StrategyRequest{
		Strategy: "git-flow", InitializeRepository: true, GitFlowDialect: "avh",
		Branches: map[string]string{"production-branch": "main", "development-branch": "develop"},
		Prefixes: map[string]string{"feature": "feature/", "bugfix": "bugfix/", "release": "release/", "hotfix": "hotfix/", "support": "support/", "version_tag": "v"},
		InitialCommit: &gitmanager.CommitRequest{MessageBytes: []byte("feat: start the project\n"), Includes: includes,
			Author: identity, Committer: identity, AuthorDate: at, CommitterDate: at},
	}, inspection, g.environment, gitmanager.PlanMetadata{PlanID: "plan-flow", GeneratedAt: at})
	if rejection != nil {
		t.Fatalf("planning refused: %s: %s", rejection.Reason, rejection.Detail)
	}
	if len(planned.PendingDependencies) != 0 {
		t.Fatalf("pending %v", planned.PendingDependencies)
	}
	p := &commitProject{realGit: g, planned: planned}
	if p.file, err = gitmanager.EncodePlanFile(planned.Plan); err != nil {
		t.Fatal(err)
	}
	result := p.apply(t, p.store(t), "123e4567-e89b-42d3-a456-426614174091")
	if result.Status != gitmanager.ApplyStatusApplied || result.ExitCode != 0 || !result.DevelopmentBranchCreated {
		t.Fatalf("result %+v", result)
	}
	gitDir := filepath.Join(g.project, ".git")
	main, _ := os.ReadFile(filepath.Join(gitDir, "refs", "heads", "main"))
	develop, _ := os.ReadFile(filepath.Join(gitDir, "refs", "heads", "develop"))
	if len(main) != 41 || string(main) != string(develop) || string(main) != string(result.CommitOID)+"\n" {
		t.Fatalf("main %q develop %q commit %s", main, develop, result.CommitOID)
	}
	config, err := os.ReadFile(filepath.Join(gitDir, "config"))
	if err != nil {
		t.Fatal(err)
	}
	// Exactly the eight AVH keys under [gitflow ...] sections, no hooks.
	var gitflow []string
	section := ""
	for _, line := range strings.Split(string(config), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			section = line
			continue
		}
		if strings.Contains(strings.ToLower(line), "hooks") {
			t.Fatalf("a hooks key was written: %q", line)
		}
		if strings.HasPrefix(section, "[gitflow") && line != "" {
			gitflow = append(gitflow, section+" "+line)
		}
	}
	want := []string{`[gitflow "branch"] master = main`, `[gitflow "branch"] develop = develop`, `[gitflow "prefix"] feature = feature/`,
		`[gitflow "prefix"] bugfix = bugfix/`, `[gitflow "prefix"] release = release/`, `[gitflow "prefix"] hotfix = hotfix/`,
		`[gitflow "prefix"] support = support/`, `[gitflow "prefix"] versiontag = v`}
	if strings.Join(gitflow, "\n") != strings.Join(want, "\n") {
		t.Fatalf("Git Flow configuration:\n%s", strings.Join(gitflow, "\n"))
	}
	if status := p.status(t); status != "" {
		t.Fatalf("status %q", status)
	}
}
