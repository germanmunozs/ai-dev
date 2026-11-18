//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func basicNewRepo() StrategyRequest {
	return newRepo("basic", map[string]string{"initial-branch": "main"})
}

func mustPlan(t *testing.T, f *planFixture, req StrategyRequest) PlanResult {
	t.Helper()
	result, rejection := f.plan(req)
	if rejection != nil {
		t.Fatalf("plan refused: %+v", rejection)
	}
	return result
}

func coveredBytes(t *testing.T, plan GitInitializationPlan) []byte {
	t.Helper()
	data, err := CanonicalCoveredPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// A new basic repository plans initialization, the configuration
// directory, its barrier and the configuration write, in that order.
func TestPlanEnumeratesEveryMutation(t *testing.T) {
	f := newPlanFixture(t)
	plan := mustPlan(t, f, basicNewRepo()).Plan
	var kinds []OperationKind
	for _, operation := range plan.DerivedOperations() {
		kinds = append(kinds, operation.Kind)
		if operation.Kind != OperationKindVerification && (len(operation.FailureOutcomes()) == 0 || len(operation.ExpectedPostconditions()) == 0 ||
			operation.RecoveryInstruction != RecoverySupervisedReconciliation) {
			t.Fatalf("operation %+v is incomplete", operation)
		}
	}
	want := []OperationKind{OperationKindRepositoryInit, OperationKindCreateProjectConfigDirectory, OperationKindVerification, OperationKindProjectConfigWrite}
	if !reflect.DeepEqual(kinds, want) {
		t.Fatalf("operations %v", kinds)
	}
	if !reflect.DeepEqual(plan.ProhibitedOperations(), ProhibitedOperations()) || len(plan.PartialEffectPoints()) != 3 {
		t.Fatalf("prohibited %v, partial points %v", plan.ProhibitedOperations(), plan.PartialEffectPoints())
	}
	write := plan.DerivedOperations()[3]
	if !reflect.DeepEqual(write.TemporaryResourceRoles(), []ResourceRole{ResourceRoleConfigTemporary}) {
		t.Fatalf("config write roles %v", write.TemporaryResourceRoles())
	}
	if plan.ImmutableIntent.ProjectID.Validate() != nil || plan.ImmutableIntent.ObjectFormat != ObjectFormatSHA1 ||
		plan.ImmutableIntent.RefFormat != RefFormatFiles {
		t.Fatalf("intent %+v", plan.ImmutableIntent)
	}
}

// AC-002: regeneration is byte-identical, excluded metadata never changes
// the digest, and any intent change does.
func TestPlanRegenerationIsCanonical(t *testing.T) {
	f := newPlanFixture(t)
	first := mustPlan(t, f, basicNewRepo()).Plan
	inspection := f.inspect()
	again, rejection := Regenerate(context.Background(), f.deps, first.ImmutableIntent, inspection, validEnvironmentProfile(),
		PlanMetadata{PlanID: "plan-other", GeneratedAt: time.Unix(999, 0).UTC()})
	if rejection != nil {
		t.Fatalf("regeneration refused: %+v", rejection)
	}
	if again.PlanDigest != first.PlanDigest || !bytes.Equal(coveredBytes(t, again), coveredBytes(t, first)) {
		t.Fatal("regeneration differs although only excluded metadata changed")
	}
	// Same project and inspection, different frozen intent.
	other := mustPlan(t, f, newRepo("basic", map[string]string{"initial-branch": "trunk"})).Plan
	if other.PlanDigest == first.PlanDigest {
		t.Fatal("a different intent produced the same digest")
	}
}

func TestPlanRejectsUnplannableRepositories(t *testing.T) {
	for name, test := range map[string]struct {
		setup  func(f *planFixture)
		reason Reason
	}{
		"interrupted merge": {func(f *planFixture) {
			f.existingRepository()
			mustWrite(t, filepath.Join(f.root, ".git", "MERGE_HEAD"), testCommitOID+"\n")
		}, ReasonRepositoryStateIncompatible},
		"interrupted rebase": {func(f *planFixture) {
			f.existingRepository()
			mustMkdir(t, filepath.Join(f.root, ".git", "rebase-merge"))
		}, ReasonRepositoryStateIncompatible},
		"config lock": {func(f *planFixture) {
			f.existingRepository()
			mustWrite(t, filepath.Join(f.root, ".git", "config.lock"), "")
		}, ReasonLockConflict},
		"ancestor": {func(f *planFixture) {
			fakeGitDir(t, filepath.Join(filepath.Dir(f.root), ".git"))
		}, ReasonAncestorRepository},
		"conflicting project config": {func(f *planFixture) {
			mustWrite(t, filepath.Join(f.root, ".ai-dev", "config.json"), `{"other":true}`)
		}, ReasonConfigurationConflict},
		".ai-dev is a file": {func(f *planFixture) {
			mustWrite(t, filepath.Join(f.root, ".ai-dev"), "x")
		}, ReasonConfigurationConflict},
	} {
		t.Run(name, func(t *testing.T) {
			f := newPlanFixture(t)
			test.setup(f)
			req := StrategyRequest{Strategy: "trunk-based", Branches: map[string]string{"trunk-branch": "main"}}
			if f.inspect().Observation.RepositoryKind == RepositoryKindAbsent {
				req.InitializeRepository = true
			}
			_, rejection := f.plan(req)
			expectRejection(t, rejection, test.reason, 1)
		})
	}
}

// An identical existing configuration is a verified no-op, and an existing
// .ai-dev directory binds the write to its identity.
func TestPlanProjectConfiguration(t *testing.T) {
	f := newPlanFixture(t)
	first := mustPlan(t, f, basicNewRepo()).Plan
	config, err := renderProjectConfig(first.ImmutableIntent.ProjectID, StrategyBasic, "", map[string]string{BranchInitial: "main"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(f.root, ".ai-dev", "config.json"), string(config))
	result := mustPlan(t, f, basicNewRepo())
	if result.Plan.ImmutableIntent.ProjectID != first.ImmutableIntent.ProjectID {
		t.Fatal("existing project ID not preserved")
	}
	operations := result.Plan.DerivedOperations()
	if operations[len(operations)-1].Kind != OperationKindVerification {
		t.Fatalf("identical configuration not a no-op: %v", operations)
	}
	var decoded map[string]any
	if err := json.Unmarshal(config, &decoded); err != nil || decoded["strategy"] != "basic" || strings.Contains(string(config), "token") {
		t.Fatalf("config %s", config)
	}

	g := newPlanFixture(t)
	mustMkdir(t, filepath.Join(g.root, ".ai-dev"))
	operations = mustPlan(t, g, basicNewRepo()).Plan.DerivedOperations()
	last := operations[len(operations)-1]
	if last.Kind != OperationKindProjectConfigWrite || last.Preconditions()[1].Kind != ConditionPathIdentity {
		t.Fatalf("write against an existing directory %+v", last)
	}
}

// c-01: the same condition is classified by the phase that first saw it.
func TestRepositoryReasonPrecedence(t *testing.T) {
	for phase, want := range map[RepositoryPhase]Reason{
		PhasePlanning: ReasonRepositoryStateIncompatible, PhaseApplyDrift: ReasonPlanStale, PhaseRevalidated: ReasonConcurrentChange,
	} {
		if got := ClassifyRepositoryReason(phase, ReasonRepositoryStateIncompatible); got != want {
			t.Errorf("phase %d: %s, want %s", phase, got, want)
		}
	}
	f := newPlanFixture(t)
	plan := mustPlan(t, f, basicNewRepo()).Plan
	meta := PlanMetadata{PlanID: "plan-2", GeneratedAt: time.Unix(300, 0).UTC()}
	if _, rejection := RegenerateAuthorized(context.Background(), f.deps, plan.ImmutableIntent, plan.PlanDigest, f.inspect(),
		validEnvironmentProfile(), meta); rejection != nil {
		t.Fatalf("unchanged regeneration refused: %+v", rejection)
	}
	// Drift after authorization is plan_stale, never the initial reason.
	mustWrite(t, filepath.Join(f.root, ".ai-dev", "config.json"), "{}")
	_, rejection := RegenerateAuthorized(context.Background(), f.deps, plan.ImmutableIntent, plan.PlanDigest, f.inspect(),
		validEnvironmentProfile(), meta)
	expectRejection(t, rejection, ReasonPlanStale, 1)
	g := newPlanFixture(t)
	other := mustPlan(t, g, basicNewRepo()).Plan
	_, rejection = RegenerateAuthorized(context.Background(), g.deps, other.ImmutableIntent, plan.PlanDigest, g.inspect(),
		validEnvironmentProfile(), meta)
	expectRejection(t, rejection, ReasonPlanStale, 1)
}

// An untrusted plan file yields only its intent, and only when it is the
// exact canonical file the planner wrote.
func TestDecodePlanFile(t *testing.T) {
	f := newPlanFixture(t)
	plan := mustPlan(t, f, gitFlowRequest("avh")).Plan
	data, err := EncodePlanFile(plan)
	if err != nil {
		t.Fatal(err)
	}
	file, err := DecodePlanFile(data)
	if err != nil {
		t.Fatal(err)
	}
	if file.PlanDigest != plan.PlanDigest || file.PlanID != plan.PlanID {
		t.Fatalf("decoded %+v", file)
	}
	if again, rejection := Regenerate(context.Background(), f.deps, file.Intent, f.inspect(), validEnvironmentProfile(),
		PlanMetadata{PlanID: "x", GeneratedAt: time.Unix(1, 0).UTC()}); rejection != nil || again.PlanDigest != plan.PlanDigest {
		t.Fatalf("decoded intent does not regenerate the plan: %+v", rejection)
	}
	text := string(data)
	for name, bad := range map[string]string{
		"non-canonical":      " " + text,
		"unknown member":     strings.Replace(text, `{"derived_operations"`, `{"admin":true,"derived_operations"`, 1),
		"tampered digest":    strings.Replace(text, string(plan.PlanDigest), testSHA256, 1),
		"tampered intent":    strings.Replace(text, `"production_branch":"main"`, `"production_branch":"prod"`, 1),
		"tampered operation": strings.Replace(text, `"gitflow.branch.master"`, `"gitflow.branch.mastr"`, 1),
		"wrong schema":       strings.Replace(text, GitInitializationPlanSchema, "ai-dev.git-init-plan/v2", 1),
		"truncated":          text[:len(text)/2],
	} {
		if bad == text {
			t.Fatalf("%s: mutation did not apply", name)
		}
		if _, err := DecodePlanFile([]byte(bad)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	oversized := bytes.Repeat([]byte(" "), int(LimitPlanBytes.Max())+1)
	var limit *LimitError
	if _, err := DecodePlanFile(oversized); !errors.As(err, &limit) || limit.Reason != ReasonPlanInvalid {
		t.Fatalf("oversized plan: %v", err)
	}
}
