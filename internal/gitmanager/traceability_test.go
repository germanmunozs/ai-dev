package gitmanager

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// frTrace is the spec's Requirement-to-Acceptance table, including the
// P-06/P-07 precision rows.
var frTrace = []struct {
	from, to int
	acs      []int
}{
	{1, 10, []int{20, 22}}, {11, 27, []int{1, 19, 22, 29}}, {28, 34, []int{14}}, {35, 49, []int{11, 23}}, {50, 58, []int{2, 30}},
	{59, 71, []int{5, 6, 18, 26}}, {72, 79, []int{11}}, {80, 93, []int{3, 8, 28}}, {94, 111, []int{7, 16, 27}},
	{112, 129, []int{9, 10, 25, 28, 29, 30}}, {130, 137, []int{19, 20}}, {138, 161, []int{2, 3, 11, 20, 23, 24}},
	{162, 177, []int{1, 2, 9, 12, 14, 15, 22, 25, 29, 30}}, {178, 188, []int{4, 5, 6, 12, 13, 17, 21, 22, 23, 24, 25, 26}},
	{189, 192, []int{7, 8, 9, 10, 21, 28}},
	{46, 46, []int{31}}, {59, 69, []int{32}}, {126, 126, []int{32}}, {128, 128, []int{32}}, {166, 166, []int{32}},
	{168, 169, []int{32}}, {179, 180, []int{32}}, {184, 184, []int{32}}, {189, 189, []int{32}},
}

// acTrace maps every acceptance criterion to the implementation files and
// test functions that realize and verify it.
var acTrace = map[int]struct{ implementation, tests []string }{
	1:  {[]string{"internal/gitmanager/inspect.go"}, []string{"TestInspectDoesNotRunGitWhenUnsafe", "TestHostileGitSentinelsRecordRealGitInvocations", "TestInitInspectSubprocess"}},
	2:  {[]string{"internal/gitmanager/plan.go", "internal/gitmanager/canonical.go"}, []string{"TestPlanRegenerationIsCanonical", "TestPlanDigest", "FuzzCanonicalWriter"}},
	3:  {[]string{"internal/gitmanager/allowlist.go", "internal/gitmanager/authorization.go"}, []string{"TestInitialCommitAllowlist", "TestCheckApprovals"}},
	4:  {[]string{"internal/gitmanager/authorization.go", "internal/gitmanager/authorization_store.go"}, []string{"TestApplyIssuanceInputs", "TestAuthorizationStoreIntegration", "TestRealEngineNonInteractiveFlow"}},
	5:  {[]string{"internal/gitmanager/safety.go", "internal/gitstore/authorization.go"}, []string{"TestConcurrentConsumption", "TestConcurrentClaimsHaveOneWinner", "TestRaceApplyKernel", "TestConcurrentLoserWithRealGit"}},
	6:  {[]string{"internal/gitmanager/recovery.go", "internal/gitmanager/reconcile.go"}, []string{"TestCrashEvidenceConsumesClaim", "TestMutationStartCASFaults", "TestSupervisedReconciliation", "TestFaultMatrix"}},
	7:  {[]string{"internal/gitmanager/initrepo.go"}, []string{"TestDeterministicInitializationUnderHostileAmbientConfiguration", "TestInitArgvIsExact", "TestInitCapabilities"}},
	8:  {[]string{"internal/gitmanager/preflight.go", "internal/gitmanager/secrets.go"}, []string{"TestInitialCommitSecretInLastFile", "TestSecretInLastAllowlistedFile", "TestPreflightPhases", "TestCommitBarrierStopsBeforeTheFirstBlob"}},
	9:  {[]string{"internal/gitmanager/terminal.go", "internal/gitmanager/operations.go"}, []string{"TestClassifyEffects", "TestOperationFaultMatrix", "TestApplyEffectClassification"}},
	10: {[]string{"internal/gitmanager/commit.go", "internal/gitmanager/index.go"}, []string{"TestInitialCommitFlowWithRealGit", "TestCommitGroupPlan", "TestAutonomousIndexParser"}},
	11: {[]string{"internal/gitmanager/gitflow.go", "internal/gitmanager/projectconfig_posix.go"}, []string{"TestGitFlowAdoption", "TestGitFlowConflictsAndFreedom", "TestGitFlowNewProjectWithInitialCommit", "TestProjectConfigWrite"}},
	12: {[]string{"internal/gitmanager/allowlist.go", "internal/gitmanager/path.go"}, []string{"TestInitialCommitRefusesNonLiteralPaths", "TestInitialCommitPathEquivalence", "TestPathEquivalence", "TestPathWithPipeIsRefusedBeforeCanonicalization"}},
	13: {[]string{"internal/gitmanager/planoutput.go"}, []string{"TestWritePlanOutputRefusesUnsafeDestinations", "TestInitPlanSubprocess"}},
	14: {[]string{"internal/gitmanager/attributes.go"}, []string{"TestAttributeEvaluatorMatchesGit", "TestAttributeEvaluatorFailsClosed", "TestConversionRisk"}},
	15: {[]string{"internal/gitmanager/toolchain.go", "internal/gitprocess/identify.go"}, []string{"TestIdentifyFreezesToolchain", "TestFrozenEnvironmentEquality", "TestAutonomousIndexParser"}},
	16: {[]string{"internal/conventionalcommit/validate.go", "internal/app/gitcommit.go"}, []string{"TestValidateCorpus", "FuzzConventionalCommitValidator", "TestGitCommitValidateSubprocess", "TestInitialCommitMessage"}},
	17: {[]string{"internal/gitmanager/release.go", "internal/gitmanager/index_posix.go"}, []string{"TestIndexLockProtocol", "TestPreparedIndexStorage", "TestReleaseReplacedResource", "TestApplyLockReleaseDetectsReplacement"}},
	18: {[]string{"internal/gitmanager/journal.go"}, []string{"TestMutationIntentFaults", "TestTerminalClosureFaults", "TestFaultMatrix"}},
	19: {[]string{"internal/gitmanager/filesystem.go", "internal/gitstore/unsupported.go"}, []string{"TestPlatformSelection", "TestFeature008PlatformBuildSelection", "TestUnsupportedPlatformFailsClosed", "TestTmpfsIsTestOnly"}},
	20: {[]string{"internal/gitmanager/strategyreport.go", "internal/app/gitcompose.go"}, []string{"TestGitManagementRegression", "TestNoWorkflowGateOrFeature007Authority", "TestStrategyReport"}},
	21: {[]string{"internal/gitmanager/result.go", "internal/gitmanager/terminal.go"}, []string{"TestApplyResultStatusReasonExitMapping", "TestConsumedAfterConclusiveClosure", "TestSupervisedReconciliation"}},
	22: {[]string{"internal/app/gitapply.go", "internal/app/gitplan.go", "internal/app/gitreconcile.go"}, []string{"TestGitManagementPublicCLI", "TestInitApplySubprocess", "TestInitPlanSubprocess", "TestStrategyOptionMatrix"}},
	23: {[]string{"internal/gitmanager/gitflow.go"}, []string{"TestGitFlowProfilesArePlannedKeyByKey", "TestGitFlowNextProfileIsExact", "TestGitFlowNextAdoptionPreservesBaseKeys"}},
	24: {[]string{"internal/gitmanager/path_posix.go", "internal/gitmanager/projectconfig.go"}, []string{"TestResolveProjectRootIdentity", "TestProjectIdentityLargeInode", "TestProjectConfigRendering"}},
	25: {[]string{"internal/gitmanager/index_posix.go", "internal/gitmanager/index.go"}, []string{"TestIndexLockProtocol", "TestAutonomousIndexParser"}},
	26: {[]string{"internal/gitmanager/journal.go", "internal/gitmanager/records.go"}, []string{"TestAuditStoreIntegration", "TestAuditEventOrdering", "TestAttemptAndEventRecordsCarryNoContent"}},
	27: {[]string{"internal/gitmanager/allowlist.go", "internal/gitmanager/commit.go"}, []string{"TestInitialCommitIdentity", "TestInitialCommitMessage", "TestObjectEncoding"}},
	28: {[]string{"internal/gitmanager/operations.go", "internal/gitmanager/postconditions.go"}, []string{"TestInitialCommitFlowWithRealGit", "TestGitFlowNewProjectWithInitialCommit", "TestInitialCommitSecretInLastFile"}},
	29: {[]string{"internal/gitmanager/allowlist.go", "internal/gitmanager/snapshot.go"}, []string{"TestInitialCommitRepositoryState", "TestCommitInExistingUnbornRepository", "TestPlanRejectsUnplannableRepositories"}},
	30: {[]string{"internal/gitmanager/classify.go", "internal/gitmanager/apply.go"}, []string{"TestRepositoryReasonPrecedence", "TestApplyRevalidationPhases", "TestDevelopmentBaseDriftAndRace"}},
	31: {[]string{"internal/gitmanager/strategy.go", "internal/gitmanager/plan.go"}, []string{"TestDevelopmentBaseMatrix", "TestDevelopmentCASAuthorization", "TestDevelopmentBaseDriftAndRace"}},
	32: {[]string{"internal/gitmanager/release.go", "internal/gitmanager/terminal.go"}, []string{"TestTerminalClosureSecondaryFailures", "TestReleaseSoleFailure", "TestReleaseAuditFailures", "TestFaultMatrix"}},
}

// T108: every F008-FR-001–192 maps to at least one acceptance criterion,
// every AC-001–032 is reached and maps to existing implementation files
// and existing test functions. The mapping is data, so a renamed test or
// file, or an uncovered identifier, fails here.
func TestTraceability(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	tests := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "specs") {
			if err == nil {
				return filepath.SkipDir
			}
			return err
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for name := range file.Scope.Objects {
			tests[name] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	reached := map[int]bool{}
	for fr := 1; fr <= 192; fr++ {
		covered := false
		for _, row := range frTrace {
			if fr >= row.from && fr <= row.to {
				covered = true
				for _, ac := range row.acs {
					reached[ac] = true
				}
			}
		}
		if !covered {
			t.Errorf("F008-FR-%03d maps to no acceptance criterion", fr)
		}
	}
	for ac := 1; ac <= 32; ac++ {
		trace, ok := acTrace[ac]
		id := fmt.Sprintf("AC-%03d", ac)
		if !reached[ac] || !ok || len(trace.implementation) == 0 || len(trace.tests) == 0 {
			t.Errorf("%s is unreached or unmapped", id)
			continue
		}
		for _, file := range trace.implementation {
			if _, err := os.Stat(filepath.Join(root, file)); err != nil {
				t.Errorf("%s: implementation %s does not exist", id, file)
			}
		}
		for _, name := range trace.tests {
			if !tests[name] {
				t.Errorf("%s: test %s does not exist", id, name)
			}
		}
	}
	if len(acTrace) != 32 {
		t.Errorf("%d acceptance criteria mapped, want 32", len(acTrace))
	}
}
