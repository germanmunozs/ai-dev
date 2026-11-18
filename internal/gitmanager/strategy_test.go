//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// planGitReader answers check_ref_format like Git for the names the tests
// use (space and ".." are invalid) and delegates every other row.
type planGitReader struct {
	inner   *fakeGitReader
	checked []string
}

func (r *planGitReader) Read(ctx context.Context, request GitReadRequest) (GitRunResult, error) {
	if request.Row != GitReadCheckRefFormat {
		return r.inner.Read(ctx, request)
	}
	name := string(request.BranchName)
	r.checked = append(r.checked, name)
	if strings.ContainsAny(name, " ~^:?*[\\") || strings.Contains(name, "..") || strings.HasSuffix(name, ".lock") {
		return GitRunResult{ExitCode: 1}, nil
	}
	return GitRunResult{}, nil
}

// fakeCommandBuilder renders deterministic argv from the command fields.
type fakeCommandBuilder struct{}

func (fakeCommandBuilder) BuildCommand(c GitCommand) (GitInvocation, error) {
	if c.Kind == GitCommandInit {
		return GitInvocation{Args: append([]string{"-c", "baseline"}, initArgv(string(c.BranchName), c.PhysicalRoot)...)}, nil
	}
	args := []string{"-c", "baseline", string(c.Kind)}
	if c.ConfigKey != "" {
		args = append(args, "--", c.ConfigKey, c.ConfigValue)
	}
	if c.RefName != nil {
		args = append(args, string(c.RefName), string(c.ObjectOID), string(c.ExpectedOldOID))
	}
	return GitInvocation{Args: args}, nil
}

type planFixture struct {
	*inspectFixture
	git  *planGitReader
	deps PlannerDependencies
}

func capableTool(t testing.TB) GitToolIdentity {
	t.Helper()
	tool, err := NewGitToolIdentity(GitToolIdentityInput{
		InvocationMode: InvocationModeDirect, InvocationPath: "/usr/bin/git", InvocationPathDigest: testSHA256,
		InvocationDevice: "1", InvocationInode: "2", EffectiveExecutablePath: "/usr/bin/git", EffectiveExecutableDigest: testSHA256,
		Version: "2.54.0", ExecPath: "/usr/libexec/git-core",
		Capabilities:         []string{"config-end-of-options", "initial-branch", "object-format", "path-format", "ref-format"},
		EnvironmentSelectors: map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return tool
}

func newPlanFixture(t *testing.T) *planFixture {
	f := &planFixture{inspectFixture: newInspectFixture(t)}
	f.request.Tool = capableTool(t)
	f.git = &planGitReader{inner: f.inspectFixture.git}
	f.inspectFixture.deps.Git = f.git
	f.deps = PlannerDependencies{Git: f.git, Commands: fakeCommandBuilder{}, Random: newFakeRandomSource(bytes.Repeat([]byte{0x5a}, 64))}
	return f
}

// existingRepository makes the project a clean normal repository whose
// history has refs/heads/main at testCommitOID.
func (f *planFixture) existingRepository() {
	f.repository()
	f.git.inner.results[GitReadStatus] = GitRunResult{}
	f.git.inner.results[GitReadShowRefs] = GitRunResult{Stdout: []byte(testCommitOID + " refs/heads/main\n")}
}

func (f *planFixture) plan(req StrategyRequest) (PlanResult, *PlanRejection) {
	inspection := f.inspect()
	return Plan(context.Background(), f.deps, req, inspection, validEnvironmentProfile(),
		PlanMetadata{PlanID: "plan-1", GeneratedAt: time.Unix(200, 0).UTC()})
}

func newRepo(strategy string, branches map[string]string) StrategyRequest {
	return StrategyRequest{Strategy: strategy, InitializeRepository: true, Branches: branches}
}

var completePrefixes = map[string]string{
	"feature": "feature/", "bugfix": "bugfix/", "release": "release/", "hotfix": "hotfix/", "support": "support/", "version_tag": "v",
}

func gitFlowRequest(dialect string) StrategyRequest {
	return StrategyRequest{Strategy: "git-flow", InitializeRepository: true, GitFlowDialect: dialect, Prefixes: completePrefixes,
		Branches: map[string]string{"production-branch": "main", "development-branch": "develop"}}
}

func expectRejection(t *testing.T, rejection *PlanRejection, reason Reason, exit int) {
	t.Helper()
	if rejection == nil || rejection.Reason != reason || rejection.ExitCode() != exit {
		t.Fatalf("got %+v, want %s exit %d", rejection, reason, exit)
	}
}

func TestStrategyRequiresAnExplicitCanonicalName(t *testing.T) {
	for _, name := range []string{"", "trunk", "trunk_based", "Basic", "gitflow", "default"} {
		f := newPlanFixture(t)
		_, rejection := f.plan(newRepo(name, map[string]string{"initial-branch": "main"}))
		expectRejection(t, rejection, ReasonInvalidStrategy, 2)
	}
}

func TestStrategyOptionMatrix(t *testing.T) {
	for name, test := range map[string]struct {
		req    StrategyRequest
		reason Reason
		exit   int
	}{
		"basic without initial":       {newRepo("basic", nil), ReasonMissingRequiredBranch, 2},
		"github-flow without primary": {newRepo("github-flow", nil), ReasonMissingRequiredBranch, 2},
		"basic with production":       {newRepo("basic", map[string]string{"initial-branch": "main", "production-branch": "main"}), ReasonInvalidStrategy, 2},
		"basic with dialect":          {StrategyRequest{Strategy: "basic", InitializeRepository: true, GitFlowDialect: "avh", Branches: map[string]string{"initial-branch": "main"}}, ReasonInvalidStrategy, 2},
		"basic with prefix":           {StrategyRequest{Strategy: "basic", InitializeRepository: true, Prefixes: map[string]string{"feature": "f/"}, Branches: map[string]string{"initial-branch": "main"}}, ReasonInvalidStrategy, 2},
		"basic with development base": {StrategyRequest{Strategy: "basic", InitializeRepository: true, DevelopmentBaseOID: "sha1:" + testCommitOID, Branches: map[string]string{"initial-branch": "main"}}, ReasonInvalidStrategy, 2},
		"trunk without trunk branch":  {newRepo("trunk-based", nil), ReasonMissingRequiredBranch, 2},
		"trunk with separate initial": {newRepo("trunk-based", map[string]string{"trunk-branch": "trunk", "initial-branch": "main"}), ReasonInvalidStrategy, 2},
		"git-flow without production": {StrategyRequest{Strategy: "git-flow", InitializeRepository: true, GitFlowDialect: "avh", Prefixes: completePrefixes, Branches: map[string]string{"development-branch": "develop"}}, ReasonMissingRequiredBranch, 2},
		"git-flow with trunk":         {StrategyRequest{Strategy: "git-flow", InitializeRepository: true, Branches: map[string]string{"trunk-branch": "t"}}, ReasonInvalidStrategy, 2},
		"git-flow without dialect":    {gitFlowRequest(""), ReasonInvalidStrategy, 2},
		"git-flow unknown dialect":    {gitFlowRequest("nvie"), ReasonInvalidStrategy, 2},
		"git-flow missing prefix": {func() StrategyRequest {
			r := gitFlowRequest("avh")
			r.Prefixes = map[string]string{"feature": "f/"}
			return r
		}(), ReasonInvalidStrategy, 2},
		"git-flow initial differs": {func() StrategyRequest { r := gitFlowRequest("avh"); r.Branches["initial-branch"] = "develop"; return r }(), ReasonInvalidStrategy, 2},
		"git-flow equal branches": {func() StrategyRequest {
			r := gitFlowRequest("avh")
			r.Branches["development-branch"] = "main"
			return r
		}(), ReasonInvalidStrategy, 2},
		"git-flow-next reserved name": {func() StrategyRequest {
			r := gitFlowRequest("git-flow-next")
			r.Branches["development-branch"] = "release"
			return r
		}(), ReasonInvalidStrategy, 2},
		"absent without initialize": {StrategyRequest{Strategy: "trunk-based", Branches: map[string]string{"trunk-branch": "trunk"}}, ReasonRepositoryAbsent, 1},
		"new repo with development base": {func() StrategyRequest {
			r := gitFlowRequest("avh")
			r.DevelopmentBaseOID = "sha1:" + testCommitOID
			return r
		}(), ReasonInvalidStrategy, 2},
	} {
		t.Run(name, func(t *testing.T) {
			_, rejection := newPlanFixture(t).plan(test.req)
			expectRejection(t, rejection, test.reason, test.exit)
		})
	}
}

// Branch names are explicit inputs validated before Git and then by Git;
// shell metacharacters that Git accepts stay literal argv bytes.
func TestBranchNameValidation(t *testing.T) {
	for _, name := range []string{"HEAD", "-main", "ma\x01in", "ma\u0085in", "ma\nin", "@{-1}", "main@{1}", "a b", "a..b", "x.lock", "\xffbad"} {
		f := newPlanFixture(t)
		_, rejection := f.plan(newRepo("basic", map[string]string{"initial-branch": name}))
		expectRejection(t, rejection, ReasonInvalidStrategy, 2)
	}
	f := newPlanFixture(t)
	result, rejection := f.plan(newRepo("basic", map[string]string{"initial-branch": "feat;$(x)|y"}))
	if rejection != nil {
		t.Fatalf("literal metacharacters refused: %+v", rejection)
	}
	init := result.Plan.DerivedOperations()[0]
	if init.Kind != OperationKindRepositoryInit || !containsValue(init.ExactArgv(), "--initial-branch=feat;$(x)|y") {
		t.Fatalf("init argv %v", init.ExactArgv())
	}
	if !containsValue(f.git.checked, "feat;$(x)|y") {
		t.Fatalf("Git did not validate the branch: %v", f.git.checked)
	}
}

func TestExistingRepositoryRules(t *testing.T) {
	t.Run("never reinitialized", func(t *testing.T) {
		f := newPlanFixture(t)
		f.existingRepository()
		_, rejection := f.plan(newRepo("trunk-based", map[string]string{"trunk-branch": "main"}))
		expectRejection(t, rejection, ReasonRepositoryStateIncompatible, 1)
	})
	t.Run("initial branch only for new repositories", func(t *testing.T) {
		f := newPlanFixture(t)
		f.existingRepository()
		_, rejection := f.plan(StrategyRequest{Strategy: "basic", Branches: map[string]string{"initial-branch": "main"}})
		expectRejection(t, rejection, ReasonInvalidStrategy, 2)
	})
	t.Run("existing gitflow keys are informational for other strategies", func(t *testing.T) {
		f := newPlanFixture(t)
		f.existingRepository()
		mustWrite(t, filepath.Join(f.root, ".git", "config"), "[gitflow]\n\tversion = 1.0\n")
		result, rejection := f.plan(StrategyRequest{Strategy: "github-flow"})
		if rejection != nil {
			t.Fatalf("refused: %+v", rejection)
		}
		for _, operation := range result.Plan.DerivedOperations() {
			for _, key := range operation.AffectedConfigKeys() {
				if strings.HasPrefix(key, "gitflow.") {
					t.Fatalf("non-Git-Flow plan writes %s", key)
				}
			}
		}
		if !strings.Contains(strings.Join(result.Warnings, "\n"), "detected_git_flow_not_selected") {
			t.Fatalf("warnings %v", result.Warnings)
		}
	})
	t.Run("partial gitflow conflicts when Git Flow is selected", func(t *testing.T) {
		f := newPlanFixture(t)
		f.existingRepository()
		mustWrite(t, filepath.Join(f.root, ".git", "config"), "[gitflow]\n\tversion = 1.0\n")
		req := gitFlowRequest("git-flow-next")
		req.InitializeRepository = false
		_, rejection := f.plan(req)
		expectRejection(t, rejection, ReasonStrategyConflict, 1)
	})
}

// Development creation in existing history follows F008-FR-046 exactly.
func TestDevelopmentBaseMatrix(t *testing.T) {
	existing := func(base string) StrategyRequest {
		req := gitFlowRequest("avh")
		req.InitializeRepository, req.DevelopmentBaseOID = false, base
		return req
	}
	for name, test := range map[string]struct {
		base   string
		reason Reason
		exit   int
	}{
		"omitted":         {"", ReasonStrategyConflict, 1},
		"malformed":       {"sha1-" + testCommitOID, ReasonInvalidStrategy, 2},
		"bad hex":         {"sha1:xyz", ReasonInvalidStrategy, 2},
		"wrong algorithm": {"sha256:" + testSHA256OID, ReasonInvalidStrategy, 2},
		"not the tip":     {"sha1:" + testOtherSHA1OID, ReasonStrategyConflict, 1},
	} {
		t.Run(name, func(t *testing.T) {
			f := newPlanFixture(t)
			f.existingRepository()
			_, rejection := f.plan(existing(test.base))
			expectRejection(t, rejection, test.reason, test.exit)
		})
	}
	f := newPlanFixture(t)
	f.existingRepository()
	result, rejection := f.plan(existing("sha1:" + testCommitOID))
	if rejection != nil {
		t.Fatalf("valid base refused: %+v", rejection)
	}
	operations := result.Plan.DerivedOperations()
	last := operations[len(operations)-1]
	if last.Kind != OperationKindDevelopmentRefCAS || result.Plan.ImmutableIntent.DevelopmentBase.BaseOID != testCommitOID ||
		!containsValue(last.ExactArgv(), "refs/heads/develop") {
		t.Fatalf("development operation %+v", last)
	}
	// Without history the development branch is pending, never created.
	f = newPlanFixture(t)
	pending, rejection := f.plan(gitFlowRequest("avh"))
	if rejection != nil || len(pending.PendingDependencies) != 1 || pending.PendingDependencies[0] != ReasonDevelopmentBranchPendingInitialHistory {
		t.Fatalf("pending %+v %+v", pending.PendingDependencies, rejection)
	}
}

func TestGitFlowProfilesArePlannedKeyByKey(t *testing.T) {
	for _, test := range []struct {
		dialect string
		keys    int
	}{{"avh", 8}, {"git-flow-next", 2 + 5 + 6 + 4*9 + 2 + 9}} {
		f := newPlanFixture(t)
		result, rejection := f.plan(gitFlowRequest(test.dialect))
		if rejection != nil {
			t.Fatalf("%s: %+v", test.dialect, rejection)
		}
		writes := 0
		for _, operation := range result.Plan.DerivedOperations() {
			if operation.Kind == OperationKindGitConfigWrite {
				writes++
				if operation.PartialEffectClassification != PartialEffectAtomic || len(operation.AffectedConfigKeys()) != 1 {
					t.Fatalf("config operation %+v", operation)
				}
				key := operation.AffectedConfigKeys()[0]
				if key == "gitflow.path.hooks" || key == "core.hookspath" {
					t.Fatalf("%s writes hooks configuration", test.dialect)
				}
			}
		}
		if writes != test.keys {
			t.Fatalf("%s writes %d keys, want %d", test.dialect, writes, test.keys)
		}
	}
}

// A complete existing profile is adopted only when every requested value
// equals it; its keys become one verification and nothing is written.
func TestGitFlowAdoption(t *testing.T) {
	setup := func(t *testing.T) *planFixture {
		f := newPlanFixture(t)
		f.existingRepository()
		f.git.inner.results[GitReadShowRefs] = GitRunResult{Stdout: []byte(testCommitOID + " refs/heads/develop\n" + testCommitOID + " refs/heads/main\n")}
		mustWrite(t, filepath.Join(f.root, ".git", "config"), strings.Join([]string{
			"[gitflow \"branch\"]", "\tmaster = main", "\tdevelop = develop",
			"[gitflow \"prefix\"]", "\tfeature = feature/", "\tbugfix = bugfix/", "\trelease = release/", "\thotfix = hotfix/",
			"\tsupport = support/", "\tversiontag = v", "",
		}, "\n"))
		return f
	}
	request := func(production string) StrategyRequest {
		return StrategyRequest{Strategy: "git-flow", Branches: map[string]string{"production-branch": production, "development-branch": "develop"}}
	}
	result, rejection := setup(t).plan(request("main"))
	if rejection != nil {
		t.Fatalf("equal profile refused: %+v", rejection)
	}
	verifications, writes := 0, 0
	for _, operation := range result.Plan.DerivedOperations() {
		switch operation.Kind {
		case OperationKindGitConfigWrite:
			writes++
		case OperationKindVerification:
			if strings.HasPrefix(operation.TargetIdentity, "git_config:") {
				verifications++
				if len(operation.Preconditions()) != 8 {
					t.Fatalf("adopted keys %v", operation.Preconditions())
				}
			}
		}
	}
	if writes != 0 || verifications != 1 || result.Plan.ImmutableIntent.GitFlowDialect != GitFlowDialectAVH {
		t.Fatalf("writes %d verifications %d", writes, verifications)
	}
	_, rejection = setup(t).plan(request("trunk"))
	expectRejection(t, rejection, ReasonStrategyConflict, 1)
	differentPrefix := request("main")
	differentPrefix.Prefixes = map[string]string{"feature": "feat/"}
	_, rejection = setup(t).plan(differentPrefix)
	expectRejection(t, rejection, ReasonStrategyConflict, 1)
}
