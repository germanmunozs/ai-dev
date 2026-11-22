//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"path/filepath"
	"strings"
	"testing"
)

// writeLocalConfig writes the repository's local configuration from
// dotted keys.
func writeLocalConfig(t *testing.T, f *planFixture, entries []configKV) {
	t.Helper()
	var b strings.Builder
	for _, entry := range entries {
		section, rest, _ := strings.Cut(entry.key, ".")
		last := strings.LastIndexByte(rest, '.')
		if last < 0 {
			b.WriteString("[" + section + "]\n\t" + rest + " = " + entry.value + "\n")
			continue
		}
		b.WriteString("[" + section + " \"" + rest[:last] + "\"]\n\t" + rest[last+1:] + " = " + entry.value + "\n")
	}
	mustWrite(t, filepath.Join(f.root, ".git", "config"), b.String())
}

func gitFlowExisting(dialect string) StrategyRequest {
	req := gitFlowRequest(dialect)
	req.InitializeRepository, req.Prefixes, req.GitFlowDialect = false, nil, ""
	return req
}

func historyWithDevelop(f *planFixture) {
	f.existingRepository()
	f.git.inner.results[GitReadShowRefs] = GitRunResult{Stdout: []byte(testCommitOID + " refs/heads/develop\n" + testCommitOID + " refs/heads/main\n")}
}

// T091/AC-023: the git-flow-next profile is written exactly in the
// normative order, and neither profile names a hooks key (F008-FR-038,
// 040, 149).
func TestGitFlowNextProfileIsExact(t *testing.T) {
	entries := gitFlowNextEntries("main", "develop", completePrefixes, "v")
	if entries[0] != (configKV{"gitflow.version", "1.0"}) || entries[1] != (configKV{"gitflow.initialized", "true"}) {
		t.Fatalf("profile head %v", entries[:2])
	}
	profile := gitFlowNextProfile("main", "develop", completePrefixes, "v")
	for key, want := range map[string]string{
		"gitflow.branch.main.type": "base", "gitflow.branch.develop.parent": "main", "gitflow.branch.feature.prefix": "feature/",
		"gitflow.branch.release.tagprefix": "v", "gitflow.branch.hotfix.startpoint": "main", "gitflow.branch.release.startpoint": "develop",
	} {
		if profile[key] != want {
			t.Errorf("%s = %q, want %q", key, profile[key], want)
		}
	}
	for _, entry := range append(entries, gitFlowAVHEntries("main", "develop", completePrefixes)...) {
		if strings.Contains(strings.ToLower(entry.key), "hooks") {
			t.Fatalf("hooks key %s", entry.key)
		}
	}
}

// T091/AC-023: an equal git-flow-next profile is adopted without writes
// even when topic instances carry .base keys, which are preserved.
func TestGitFlowNextAdoptionPreservesBaseKeys(t *testing.T) {
	f := newPlanFixture(t)
	historyWithDevelop(f)
	writeLocalConfig(t, f, append(gitFlowNextEntries("main", "develop", completePrefixes, "v"),
		configKV{"gitflow.branch.feature/login.base", "develop"}))
	result := mustPlan(t, f, gitFlowExisting(""))
	if result.Plan.ImmutableIntent.GitFlowDialect != GitFlowDialectNext {
		t.Fatalf("dialect %s", result.Plan.ImmutableIntent.GitFlowDialect)
	}
	for _, operation := range result.Plan.DerivedOperations() {
		if operation.Kind == OperationKindGitConfigWrite {
			t.Fatalf("an adopted profile writes %v", operation.AffectedConfigKeys())
		}
	}
	if bases := f.inspect().Observation.GitFlowEvidence.InstanceBases(); len(bases) != 1 || bases[0].Key != "gitflow.branch.feature/login.base" {
		t.Fatalf("instance bases %v", bases)
	}
}

// T091/AC-011: partial, mixed or contradictory profiles are conflicts and
// are never normalized; git-flow-next reserves the topic names; a
// non-Git-Flow strategy leaves existing keys alone with a warning
// (F008-FR-041, 146, 170).
func TestGitFlowConflictsAndFreedom(t *testing.T) {
	avh := gitFlowAVHEntries("main", "develop", completePrefixes)
	for name, entries := range map[string][]configKV{
		"partial":       avh[:5],
		"mixed":         append(append([]configKV(nil), avh...), configKV{"gitflow.version", "1.0"}),
		"contradictory": append(append([]configKV(nil), avh[2:]...), configKV{"gitflow.branch.master", "main"}, configKV{"gitflow.branch.develop", "main"}),
		"only bases":    {{"gitflow.branch.feature/x.base", "develop"}},
	} {
		f := newPlanFixture(t)
		historyWithDevelop(f)
		writeLocalConfig(t, f, entries)
		_, rejection := f.plan(gitFlowExisting("avh"))
		if rejection == nil || rejection.Reason != ReasonStrategyConflict {
			t.Errorf("%s: %+v", name, rejection)
		}
	}
	for _, reserved := range []string{"feature", "release"} {
		f := newPlanFixture(t)
		req := gitFlowRequest("git-flow-next")
		req.Branches = map[string]string{"production-branch": reserved, "development-branch": "develop"}
		_, rejection := f.plan(req)
		expectRejection(t, rejection, ReasonInvalidStrategy, 2)
	}
	f := newPlanFixture(t)
	historyWithDevelop(f)
	writeLocalConfig(t, f, avh)
	result := mustPlan(t, f, StrategyRequest{Strategy: "basic", Branches: map[string]string{}, Prefixes: map[string]string{}})
	for _, operation := range result.Plan.DerivedOperations() {
		if operation.Kind == OperationKindGitConfigWrite {
			t.Fatal("a non-Git-Flow strategy writes gitflow keys")
		}
	}
	if len(result.Warnings) == 0 || !strings.HasPrefix(result.Warnings[0], "detected_git_flow_not_selected") {
		t.Fatalf("warnings %v", result.Warnings)
	}
}

// T091/AC-023: multi-key writes are not atomic: a failing key stops the
// apply, later keys never start, and the earlier verified writes are a
// partial result (F008-FR-039).
func TestGitFlowWriteStopsAtFirstFailure(t *testing.T) {
	f := newApplyFixture(t, gitFlowRequest("avh"))
	third := -1
	writes := 0
	for _, operation := range f.plan.DerivedOperations() {
		if operation.Kind == OperationKindGitConfigWrite {
			if writes == 2 {
				third = int(operation.Ordinal)
			}
			writes++
		}
	}
	f.executor.outcomes = map[int]OperationOutcome{third: OperationFailed}
	f.executor.evidence = ApplyEvidence{VerifiedEffects: []Condition{{Kind: ConditionConfigValue, Subject: "gitflow.branch.master", Expected: "main"}}}
	result := f.issue(operator())
	expectResult(t, result, ApplyStatusPartiallyApplied, ReasonOperationFailed, 1)
	if len(f.executor.executed) != third+1 {
		t.Fatalf("executed %v after the failing key %d", f.executor.executed, third)
	}
}
