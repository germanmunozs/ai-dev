//go:build (darwin && !ios) || (linux && !android)

package gitmanager_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-dev/internal/gitmanager"
	"ai-dev/internal/gitprocess"
	"ai-dev/internal/gitstore"
)

const initAuthorizationID = "123e4567-e89b-42d3-a456-426614174078"

// realGit is the real stack: located and identified Git, the operation
// matrix, the real store and the production dispatcher.
type realGit struct {
	t                    *testing.T
	project, home, state string
	tool                 gitmanager.GitToolIdentity
	environment          gitmanager.EnvironmentProfile
	reader               gitmanager.GitReader
}

func newRealGit(t *testing.T) *realGit {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	g := &realGit{t: t, project: filepath.Join(base, "project"), home: filepath.Join(base, "home"),
		state: filepath.Join(base, "state", "git"), reader: gitprocess.Reader{Runner: gitprocess.Runner{}}}
	for _, dir := range []string{g.project, g.home} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if g.environment, err = gitmanager.NewEnvironmentProfile(gitmanager.NewObservedPath([]byte(g.state))); err != nil {
		t.Fatal(err)
	}
	path, mode, err := gitprocess.LocateGit(os.Getenv("PATH"))
	if err != nil {
		t.Skipf("no usable Git: %v", err)
	}
	if g.tool, err = gitprocess.Identify(context.Background(), mode, path, nil, g.environment, []byte("/")); err != nil {
		t.Skipf("Git cannot be identified: %v", err)
	}
	for _, capability := range []string{"initial-branch", "object-format", "ref-format"} {
		if !strings.Contains(strings.Join(g.tool.Capabilities(), ","), capability) {
			t.Skipf("Git %s lacks %s", g.tool.Version, capability)
		}
	}
	return g
}

func (g *realGit) inspect(ctx context.Context) (gitmanager.GitInspection, error) {
	return gitmanager.Inspect(ctx, gitmanager.InspectorDependencies{Git: g.reader, ObserveFilesystem: gitstore.ObserveFilesystem},
		gitmanager.InspectRequest{ProjectDir: g.project, WorkingDir: "/", Tool: g.tool, Environment: g.environment,
			User: gitmanager.UserContext{Home: []byte(g.home)}, InspectionID: "inspection-1", InspectedAt: time.Unix(100, 0).UTC()})
}

func (g *realGit) apply(store gitmanager.GitStore, file []byte, digest gitmanager.PlanDigest, authorizationID string) gitmanager.GitApplyResult {
	planner := gitmanager.PlannerDependencies{Git: g.reader, Commands: gitprocess.Builder{}, Random: rand.Reader}
	inspect := func(ctx context.Context, _ []byte) (gitmanager.GitInspection, error) { return g.inspect(ctx) }
	return gitmanager.Apply(context.Background(), gitmanager.ApplyDependencies{
		Store: store, Clock: fixedTime(time.Unix(1000, 0).UTC()), Random: rand.Reader, Planner: planner, Tool: g.tool,
		Environment: g.environment, Inspect: inspect,
		Executor: gitmanager.NewOperationDispatcher(gitmanager.DispatcherDependencies{
			Runner: gitprocess.Runner{}, Commands: gitprocess.Builder{}, Inspect: inspect}),
	}, gitmanager.ApplyRequest{PlanFile: file, ConfirmPlan: string(digest), AuthorizationID: authorizationID,
		Issue: &gitmanager.IssueRequest{AuthorizedBy: "operator"}})
}

// T073/AC-007: a hostile template, default branch, default object and ref
// format and shared-repository setting, in the user's configuration and in
// the process environment, do not alter the authorized initialization.
func TestDeterministicInitializationUnderHostileAmbientConfiguration(t *testing.T) {
	g := newRealGit(t)
	template := filepath.Join(g.home, "template")
	if err := os.MkdirAll(filepath.Join(template, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"hooks/post-checkout": "#!/bin/sh\nexit 1\n", "description": "HOSTILE\n", "info/exclude": "*\n"} {
		_ = os.MkdirAll(filepath.Dir(filepath.Join(template, name)), 0o755)
		if err := os.WriteFile(filepath.Join(template, name), []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	hostile := "[init]\n\tdefaultBranch = hostile\n\ttemplateDir = " + template + "\n\tdefaultObjectFormat = sha256\n\tdefaultRefFormat = reftable\n" +
		"[core]\n\tsharedRepository = group\n"
	if err := os.WriteFile(filepath.Join(g.home, ".gitconfig"), []byte(hostile), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"GIT_TEMPLATE_DIR": template, "GIT_DEFAULT_HASH": "sha256", "GIT_DEFAULT_REF_FORMAT": "reftable",
		"GIT_CONFIG_GLOBAL": filepath.Join(g.home, ".gitconfig"), "GIT_CONFIG_PARAMETERS": "'init.defaultbranch'='hostile'"} {
		t.Setenv(name, value)
	}

	inspection, err := g.inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range inspection.Incompatibilities() {
		if item.Reason == gitmanager.ReasonFilesystemUnsupported {
			t.Skip("the temporary directory is not on a production filesystem")
		}
	}
	planner := gitmanager.PlannerDependencies{Git: g.reader, Commands: gitprocess.Builder{}, Random: rand.Reader}
	planned, rejection := gitmanager.Plan(context.Background(), planner,
		gitmanager.StrategyRequest{Strategy: "basic", InitializeRepository: true, Branches: map[string]string{"initial-branch": "trunk-x"}},
		inspection, g.environment, gitmanager.PlanMetadata{PlanID: "plan-1", GeneratedAt: time.Unix(200, 0).UTC()})
	if rejection != nil {
		t.Fatalf("planning refused: %s: %s", rejection.Reason, rejection.Detail)
	}
	file, err := gitmanager.EncodePlanFile(planned.Plan)
	if err != nil {
		t.Fatal(err)
	}
	store, err := gitstore.Open(gitmanager.NewObservedPath([]byte(g.state)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// The initialization completes and verifies; the project configuration
	// write is not dispatched yet (T093), so the attempt stops there with
	// its verified effect and never rolls the repository back.
	result := g.apply(store, file, planned.Plan.PlanDigest, initAuthorizationID)
	if result.Status != gitmanager.ApplyStatusPartiallyApplied || result.Reason != gitmanager.ReasonOperationFailed || result.ExitCode != 1 {
		t.Fatalf("result %+v", result)
	}
	gitDir := filepath.Join(g.project, ".git")
	if head, err := os.ReadFile(filepath.Join(gitDir, "HEAD")); err != nil || string(head) != "ref: refs/heads/trunk-x\n" {
		t.Fatalf("HEAD %q %v", head, err)
	}
	for _, absent := range []string{"description", "hooks", "info", "logs", "reftable"} {
		if _, err := os.Lstat(filepath.Join(gitDir, absent)); !os.IsNotExist(err) {
			t.Errorf(".git/%s exists: %v", absent, err)
		}
	}
	// Only the keys git init itself writes; no shared mode, no object or
	// ref format extension.
	allowed := map[string]bool{"repositoryformatversion": true, "filemode": true, "bare": true, "logallrefupdates": true,
		"ignorecase": true, "precomposeunicode": true, "symlinks": true}
	config, err := os.ReadFile(filepath.Join(gitDir, "config"))
	if err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(config))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "[core]" || line == "" {
			continue
		}
		key, _, _ := strings.Cut(line, "=")
		if !allowed[strings.TrimSpace(key)] {
			t.Errorf("unexpected configuration line %q", line)
		}
	}
	// Format version 0 admits no extensions: SHA-1 objects and files refs.
	if !strings.Contains(string(config), "repositoryformatversion = 0\n") {
		t.Fatalf("configuration %q", config)
	}
	if info, err := os.Stat(filepath.Join(gitDir, "refs", "heads")); err != nil || !info.IsDir() {
		t.Fatalf("files ref store: %v", err)
	}

	// The repository now exists: the same plan is stale and nothing is
	// reinitialized.
	again := g.apply(store, file, planned.Plan.PlanDigest, "123e4567-e89b-42d3-a456-426614174079")
	if again.Status != gitmanager.ApplyStatusRefused || again.Reason != gitmanager.ReasonPlanStale {
		t.Fatalf("second apply %+v", again)
	}
	if head, _ := os.ReadFile(filepath.Join(gitDir, "HEAD")); string(head) != "ref: refs/heads/trunk-x\n" {
		t.Fatalf("HEAD changed to %q", head)
	}
}
