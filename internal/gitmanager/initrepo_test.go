//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"testing"
)

// T073: the planned initialization is exactly the F008-FR-190 argv for the
// explicit initial branch and the physical root.
func TestInitArgvIsExact(t *testing.T) {
	f := newPlanFixture(t)
	plan := mustPlan(t, f, newRepo("trunk-based", map[string]string{"trunk-branch": "trunk-x"})).Plan
	root := plan.PreconditionSnapshot.ProjectIdentity.ReversiblePathBytes()
	init := plan.DerivedOperations()[0]
	if init.Kind != OperationKindRepositoryInit || init.Ordinal != 0 {
		t.Fatalf("first operation %s", init.Kind)
	}
	argv := init.ExactArgv()
	want := []string{"init", "--template=", "--object-format=sha1", "--ref-format=files", "--initial-branch=trunk-x", "--", string(root)}
	if !equalLists(argv[len(argv)-len(want):], want) {
		t.Fatalf("argv %q", argv)
	}
	intent := plan.ImmutableIntent
	if rejection := checkInitArgv(intent, root, argv); rejection != nil {
		t.Fatalf("exact argv refused: %s", rejection.Detail)
	}
	exact := func(tail ...string) []string { return append([]string{"-c", "core.pager=cat"}, tail...) }
	for name, argv := range map[string][]string{
		"no template override":  exact("init", "--object-format=sha1", "--ref-format=files", "--initial-branch=trunk-x", "--", string(root)),
		"ambient template":      exact("init", "--template=/tmp/t", "--object-format=sha1", "--ref-format=files", "--initial-branch=trunk-x", "--", string(root)),
		"sha256":                exact("init", "--template=", "--object-format=sha256", "--ref-format=files", "--initial-branch=trunk-x", "--", string(root)),
		"reftable":              exact("init", "--template=", "--object-format=sha1", "--ref-format=reftable", "--initial-branch=trunk-x", "--", string(root)),
		"ambient branch":        exact("init", "--template=", "--object-format=sha1", "--ref-format=files", "--", string(root)),
		"other branch":          exact("init", "--template=", "--object-format=sha1", "--ref-format=files", "--initial-branch=main", "--", string(root)),
		"other root":            exact("init", "--template=", "--object-format=sha1", "--ref-format=files", "--initial-branch=trunk-x", "--", "/elsewhere"),
		"no separator":          exact("init", "--template=", "--object-format=sha1", "--ref-format=files", "--initial-branch=trunk-x", string(root)),
		"shared repository":     exact("init", "--shared=group", "--template=", "--object-format=sha1", "--ref-format=files", "--initial-branch=trunk-x", "--", string(root)),
		"init default override": append([]string{"-c", "init.defaultBranch=x"}, want...),
		"shared override":       append([]string{"-c", "core.sharedRepository=group"}, want...),
		"trailing argument":     append(exact(want...), "extra"),
	} {
		if rejection := checkInitArgv(intent, root, argv); rejection == nil || rejection.Reason != ReasonPlanInvalid {
			t.Errorf("%s: %+v", name, rejection)
		}
	}
}

// Missing support for any mandatory option is git_capability_missing
// before mutation, both when planning and when dispatching.
func TestInitCapabilities(t *testing.T) {
	for _, missing := range initCapabilities {
		f := newPlanFixture(t)
		capabilities := []string{"config-end-of-options", "path-format"}
		for _, capability := range initCapabilities {
			if capability != missing {
				capabilities = append(capabilities, capability)
			}
		}
		lacking, err := NewGitToolIdentity(GitToolIdentityInput{
			InvocationMode: InvocationModeDirect, InvocationPath: "/usr/bin/git", InvocationPathDigest: testSHA256,
			InvocationDevice: "1", InvocationInode: "2", EffectiveExecutablePath: "/usr/bin/git", EffectiveExecutableDigest: testSHA256,
			Version: "2.54.0", ExecPath: "/usr/libexec/git-core", Capabilities: capabilities, EnvironmentSelectors: map[string]string{},
		})
		if err != nil {
			t.Fatal(err)
		}
		f.request.Tool = lacking
		if _, rejection := f.plan(basicNewRepo()); rejection == nil || rejection.Reason != ReasonGitCapabilityMissing {
			t.Errorf("planning without %s: %+v", missing, rejection)
		}
		plan := mustPlan(t, newPlanFixture(t), basicNewRepo()).Plan
		intent := plan.ImmutableIntent
		intent.ToolIdentity = lacking
		root := plan.PreconditionSnapshot.ProjectIdentity.ReversiblePathBytes()
		if rejection := checkInitArgv(intent, root, plan.DerivedOperations()[0].ExactArgv()); rejection == nil || rejection.Reason != ReasonGitCapabilityMissing {
			t.Errorf("dispatch without %s: %+v", missing, rejection)
		}
	}
}

// An existing repository is never reinitialized and keeps its object
// format: its plan has no initialization operation.
func TestExistingRepositoryIsNotReinitialized(t *testing.T) {
	f := newPlanFixture(t)
	f.existingRepository()
	plan := mustPlan(t, f, StrategyRequest{Strategy: "basic", Branches: map[string]string{}, Prefixes: map[string]string{}}).Plan
	for _, operation := range plan.DerivedOperations() {
		if operation.Kind == OperationKindRepositoryInit {
			t.Fatal("an existing repository is planned for initialization")
		}
	}
	if plan.ImmutableIntent.ObjectFormat != f.inspect().Observation.ObjectFormat || plan.ImmutableIntent.InitializeRepository {
		t.Fatalf("intent %+v", plan.ImmutableIntent)
	}
}
