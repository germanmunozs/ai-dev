package gitmanager

import (
	"bytes"
	"errors"
	"io/fs"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	testSHA256              = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	testOtherSHA256         = "sha256:fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
	testSHA1OID             = "0123456789abcdef0123456789abcdef01234567"
	testOtherSHA1OID        = "89abcdef0123456789abcdef0123456789abcdef"
	testSHA256OID           = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	testInspectionSchema    = "ai-dev.git-inspection/v1"
	testAuthorizationSchema = "ai-dev.git-authorization/v1"
	testResultSchema        = "ai-dev.git-apply-result/v1"
	testStateRoot           = "/home/u/.local/state/ai-dev/git"

	testApplyID         ApplyID         = "123e4567-e89b-42d3-a456-426614174001"
	testOtherApplyID    ApplyID         = "123e4567-e89b-42d3-a456-426614174009"
	testAuthorizationID AuthorizationID = "123e4567-e89b-42d3-a456-426614174000"
	testProjectID       ProjectID       = "123e4567-e89b-42d3-a456-426614174002"
)

func TestClosedEnums(t *testing.T) {
	tests := []struct {
		name    string
		valid   []string
		isValid func(string) bool
	}{
		{"invocation mode", []string{"launcher", "direct"}, func(v string) bool { return InvocationMode(v).Valid() }},
		{"repository kind", []string{"absent", "normal", "bare", "linked_worktree", "submodule", "nested", "ancestor", "inaccessible", "ambiguous"}, func(v string) bool { return RepositoryKind(v).Valid() }},
		{"head state", []string{"unborn", "attached", "detached", "invalid"}, func(v string) bool { return HeadState(v).Valid() }},
		{"strategy", []string{"basic", "git-flow", "github-flow", "trunk-based"}, func(v string) bool { return Strategy(v).Valid() }},
		{"git flow dialect", []string{"avh", "git-flow-next"}, func(v string) bool { return GitFlowDialect(v).Valid() }},
		{"authorization state", []string{"issued", "claimed", "mutation_started", "applied", "failed", "partially_applied", "state_indeterminate"}, func(v string) bool { return AuthorizationState(v).Valid() }},
		{"apply status", []string{"applied", "already_applied", "refused", "failed", "partially_applied", "state_indeterminate"}, func(v string) bool { return ApplyStatus(v).Valid() }},
		{"resource role", []string{"apply_lock", "index_lock", "config_temporary", "prepared_index"}, func(v string) bool { return ResourceRole(v).Valid() }},
		{"approval kind", []string{"normal", "ignored"}, func(v string) bool { return ApprovalKind(v).Valid() }},
		{"operation kind", []string{"audit_write", "authorization_transition", "apply_lock", "repository_init", "git_config_write", "create_project_config_directory", "project_config_write", "initial_commit", "initial_ref_cas", "development_ref_cas", "prepared_index", "verification", "resource_release"}, func(v string) bool { return OperationKind(v).Valid() }},
		{"object format", []string{"sha1", "sha256"}, func(v string) bool { return ObjectFormat(v).Valid() }},
		{"ref format", []string{"files", "reftable"}, func(v string) bool { return RefFormat(v).Valid() }},
		{"observation state", []string{"not_applicable", "not_observed", "observed_empty", "observed", "absent", "error"}, func(v string) bool { return ObservationState(v).Valid() }},
		{"source read state", []string{"absent", "read", "unreadable", "ambiguous", "unsupported"}, func(v string) bool { return SourceReadState(v).Valid() }},
		{"redaction state", []string{"not_needed", "redacted", "opaque"}, func(v string) bool { return RedactionState(v).Valid() }},
		{"git flow classification", []string{"absent", "complete_avh", "complete_git_flow_next", "partial", "mixed", "contradictory", "unrecognized", "unobservable"}, func(v string) bool { return GitFlowClassification(v).Valid() }},
		{"attribute source kind", []string{"system_attributes", "global_attributes_explicit", "global_attributes_default", "repository_gitattributes", "info_attributes", "post_init_intent"}, func(v string) bool { return AttributeSourceKind(v).Valid() }},
		{"ignore source kind", []string{"core_excludes_file", "info_exclude", "repository_gitignore", "post_init_intent"}, func(v string) bool { return IgnoreSourceKind(v).Valid() }},
		{"conversion risk", []string{"none", "neutral", "may_transform", "external_code", "unknown", "incompatible"}, func(v string) bool { return ConversionRisk(v).Valid() }},
		{"worktree availability", []string{"available", "missing", "locked", "inaccessible", "ambiguous"}, func(v string) bool { return WorktreeAvailability(v).Valid() }},
		{"submodule initialization", []string{"declared_uninitialized", "initialized_matching", "initialized_mismatched", "missing", "inaccessible", "ambiguous"}, func(v string) bool { return SubmoduleInitialization(v).Valid() }},
		{"audit event kind", []string{"attempt_created", "authorization_issued", "authorization_transition", "mutation_intent", "operation_started", "operation_completed", "operation_failed", "operation_indeterminate", "resource_release_started", "resource_release_result", "prepared_terminal_result", "terminal_authorization_cas"}, func(v string) bool { return AuditEventKind(v).Valid() }},
		{"filesystem role", []string{"project", "state_root", "plan_output_parent", "prepared_index"}, func(v string) bool { return FilesystemRole(v).Valid() }},
		{"sequencer state", []string{"none", "merge", "rebase", "cherry_pick", "revert", "bisect", "am", "unknown"}, func(v string) bool { return SequencerState(v).Valid() }},
		{"index presence", []string{"absent", "present", "unreadable"}, func(v string) bool { return IndexPresence(v).Valid() }},
		{"lock file state", []string{"absent", "present", "unobservable"}, func(v string) bool { return LockFileState(v).Valid() }},
		{"index change kind", []string{"staged", "conflicted", "type_changed", "mode_changed"}, func(v string) bool { return IndexChangeKind(v).Valid() }},
		{"worktree change kind", []string{"unstaged", "untracked", "ignored", "type_changed", "mode_changed"}, func(v string) bool { return WorktreeChangeKind(v).Valid() }},
		{"config scope", []string{"system", "xdg", "global", "local", "worktree", "command"}, func(v string) bool { return ConfigScope(v).Valid() }},
		{"project config state", []string{"absent", "valid", "invalid", "unreadable"}, func(v string) bool { return ProjectConfigState(v).Valid() }},
		{"condition kind", []string{"repository_absent", "repository_kind", "head_state", "head_symbolic_ref", "ref_absent", "ref_target", "reflog_absent", "object_type", "path_absent", "path_identity", "path_content", "path_mode", "index_absent", "index_empty", "index_tree", "index_lock_absent", "config_absent", "config_value", "path_status_clean"}, func(v string) bool { return ConditionKind(v).Valid() }},
		{"partial effect classification", []string{"none", "atomic", "partial_possible"}, func(v string) bool { return PartialEffectClassification(v).Valid() }},
		{"recovery instruction", []string{"not_applicable", "supervised_reconciliation"}, func(v string) bool { return RecoveryInstruction(v).Valid() }},
		{"prohibited operation", []string{"push", "fetch", "pull", "merge", "rebase", "tag_mutation", "remote_mutation", "branch_delete", "branch_rename", "reset", "checkout", "switch", "stash", "gc", "amend", "automatic_restore", "submodule_mutation", "linked_worktree_mutation", "git_flow_hooks", "git_flow_feature_finish"}, func(v string) bool { return ProhibitedOperation(v).Valid() }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, value := range test.valid {
				if !test.isValid(value) {
					t.Errorf("valid value %q rejected", value)
				}
				if test.isValid(strings.ToUpper(value)) {
					t.Errorf("uppercase value %q accepted", value)
				}
			}
			// "unknown" itself is canonical in some catalogues, so it cannot
			// serve as the out-of-catalogue probe.
			for _, value := range []string{"", "bogus"} {
				if test.isValid(value) {
					t.Errorf("invalid value %q accepted", value)
				}
			}
		})
	}
	if len(conditionShapes) != 19 || len(ProhibitedOperations()) != 20 {
		t.Fatalf("catalogue sizes: conditions=%d prohibited=%d", len(conditionShapes), len(ProhibitedOperations()))
	}
}

func TestClosedReasonCatalogue(t *testing.T) {
	valid := []Reason{
		"interactive_terminal_required", "invalid_strategy", "missing_required_branch", "repository_absent", "ancestor_repository",
		"repository_kind_unsupported", "repository_state_incompatible", "worktree_config_unsupported", "project_identity_unavailable",
		"plan_invalid", "plan_stale", "concurrent_change", "authorization_input_incomplete", "authorization_id_exists",
		"authorization_missing", "authorization_mismatch", "authorization_expired", "authorization_consumed", "audit_unavailable",
		"audit_failed_after_mutation", "resource_release_failed", "lock_conflict", "git_capability_missing",
		"development_branch_pending_initial_history", "output_exists", "unsafe_output_path", "output_failed", "operation_failed",
		"attributes_incompatible", "path_unrepresentable", "path_normalization_incompatible", "path_collision",
		"sensitive_content_suspected", "commit_message_invalid", "identity_invalid", "platform_unsupported", "filesystem_unsupported",
		"dubious_ownership", "strategy_conflict", "configuration_conflict",
	}
	if len(valid) != 40 || len(validReasons) != 40 {
		t.Fatalf("reason catalogue size=%d/%d, want 40", len(valid), len(validReasons))
	}
	seen := map[Reason]bool{}
	for _, reason := range valid {
		if !reason.Valid() || seen[reason] {
			t.Errorf("invalid or duplicate reason %q", reason)
		}
		seen[reason] = true
	}
	for _, reason := range []Reason{"", "unknown", "PLAN_INVALID"} {
		if reason.Valid() {
			t.Errorf("invalid reason %q accepted", reason)
		}
	}
}

func TestNormativeCatalogLiteralValues(t *testing.T) {
	checks := map[string][2]string{
		"invocation launcher": {string(InvocationModeLauncher), "launcher"}, "repository linked": {string(RepositoryKindLinkedWorktree), "linked_worktree"},
		"head detached": {string(HeadStateDetached), "detached"}, "strategy git flow": {string(StrategyGitFlow), "git-flow"},
		"dialect next": {string(GitFlowDialectNext), "git-flow-next"}, "authorization indeterminate": {string(AuthorizationStateIndeterminate), "state_indeterminate"},
		"apply partial": {string(ApplyStatusPartiallyApplied), "partially_applied"}, "resource prepared": {string(ResourceRolePreparedIndex), "prepared_index"},
		"approval ignored": {string(ApprovalKindIgnored), "ignored"}, "operation release": {string(OperationKindResourceRelease), "resource_release"},
		"reason stale": {string(ReasonPlanStale), "plan_stale"}, "reason audit": {string(ReasonAuditFailedAfterMutation), "audit_failed_after_mutation"},
		"observation empty": {string(ObservationObservedEmpty), "observed_empty"}, "git flow next": {string(GitFlowCompleteGitFlowNext), "complete_git_flow_next"},
		"conversion transform": {string(ConversionRiskMayTransform), "may_transform"}, "audit terminal": {string(AuditEventTerminalAuthorizationCAS), "terminal_authorization_cas"},
		"plan schema": {GitInitializationPlanSchema, "ai-dev.git-init-plan/v1"}, "project config": {ProjectConfigPath, ".ai-dev/config.json"},
		"recovery policy": {string(RecoveryPolicySupervisedReconciliation), "supervised_reconciliation"},
		"home leaf":       {EnvironmentHomeLeaf, "environment/nonexistent-home-v1"}, "xdg leaf": {EnvironmentXDGLeaf, "environment/nonexistent-xdg-v1"},
	}
	for name, pair := range checks {
		if pair[0] != pair[1] {
			t.Errorf("%s=%q want %q", name, pair[0], pair[1])
		}
	}
}

func TestNominalIdentifierNegativeCases(t *testing.T) {
	for name, validate := range map[string]func() error{
		"project wrong version":  func() error { return ProjectID("123e4567-e89b-12d3-a456-426614174000").Validate() },
		"project wrong variant":  func() error { return ProjectID("123e4567-e89b-42d3-7456-426614174000").Validate() },
		"apply uppercase":        func() error { return ApplyID("123E4567-E89B-42D3-A456-426614174001").Validate() },
		"operation leading zero": func() error { return OperationID("00:" + testSHA256).Validate() },
		"blob uppercase":         func() error { return BlobOID("0123456789ABCDEF0123456789abcdef01234567").Validate() },
		"object uppercase":       func() error { return ObjectOID("0123456789ABCDEF0123456789abcdef01234567").Validate() },
		"object wrong length":    func() error { return ObjectOID("0123").Validate() },
		"object sha1 as sha256":  func() error { return ObjectOID(testSHA1OID).ValidateFor(ObjectFormatSHA256) },
		"blob sha256 as sha1":    func() error { return BlobOID(testSHA256OID).ValidateFor(ObjectFormatSHA1) },
		"object unknown format":  func() error { return ObjectOID(testSHA1OID).ValidateFor("md5") },
	} {
		if validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestNominalIdentifiersAndDigests(t *testing.T) {
	for name, validate := range map[string]func() error{
		"plan ID":                 func() error { return PlanID("plan-1").Validate() },
		"inspection ID":           func() error { return InspectionID("inspection-1").Validate() },
		"plan digest":             func() error { return PlanDigest(testSHA256).Validate() },
		"project identity digest": func() error { return ProjectIdentityDigest(testSHA256).Validate() },
		"content digest":          func() error { return ContentDigest(testSHA256).Validate() },
		"authorization UUID":      func() error { return testAuthorizationID.Validate() },
		"apply UUID":              func() error { return testApplyID.Validate() },
		"operation ID":            func() error { return operationID(0).Validate() },
		"sha1 blob":               func() error { return BlobOID(testSHA1OID).ValidateFor(ObjectFormatSHA1) },
		"sha256 blob":             func() error { return BlobOID(testSHA256OID).ValidateFor(ObjectFormatSHA256) },
		"sha1 object":             func() error { return ObjectOID(testSHA1OID).ValidateFor(ObjectFormatSHA1) },
		"sha256 object":           func() error { return ObjectOID(testSHA256OID).ValidateFor(ObjectFormatSHA256) },
	} {
		if err := validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	invalidDigests := []string{"", "sha1:0123", "sha256:abc", "sha256:0123456789ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef", "sha256:zz23456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	for _, value := range invalidDigests {
		if PlanDigest(value).Validate() == nil {
			t.Errorf("invalid digest %q accepted", value)
		}
	}
	for _, value := range []string{"", "not-a-uuid", "123E4567-E89B-42D3-A456-426614174000", "123e4567-e89b-12d3-a456-426614174000"} {
		if AuthorizationID(value).Validate() == nil {
			t.Errorf("invalid authorization ID %q accepted", value)
		}
	}
}

func TestDecimalIdentityComponents(t *testing.T) {
	for _, value := range []string{"0", "1", "9007199254740993", "18446744073709551615", "1" + strings.Repeat("0", 255)} {
		if err := ValidateDecimalIdentity(value); err != nil {
			t.Errorf("valid decimal %q: %v", value, err)
		}
	}
	for _, value := range []string{"", "-1", "+1", "01", "1.0", "1a"} {
		if ValidateDecimalIdentity(value) == nil {
			t.Errorf("invalid decimal %q accepted", value)
		}
	}
}

func TestReversiblePathRoundTrip(t *testing.T) {
	paths := [][]byte{
		{},
		[]byte("simple/file.txt"),
		[]byte("caf\xc3\xa9/\xe6\x96\x87\xe4\xbb\xb6"),
		{0xff, 0xfe, '/', '\'', ' ', 0x00},
	}
	for _, path := range paths {
		encoded := EncodeReversiblePath(path)
		if bytes.ContainsRune([]byte(encoded), '=') {
			t.Fatalf("encoding %q contains padding", encoded)
		}
		decoded, err := DecodeReversiblePath(encoded)
		if err != nil || !bytes.Equal(decoded, path) {
			t.Fatalf("round trip %v -> %q -> %v, %v", path, encoded, decoded, err)
		}
	}
	for _, encoded := range []string{"=", "YQ==", "+/", "a b", "*", "YQ\n", "Y\nQ", "YQ\r", "YR"} {
		if _, err := DecodeReversiblePath(encoded); err == nil {
			t.Errorf("invalid base64url %q accepted", encoded)
		}
	}
}

func TestPlanIDRejectsControlCharacters(t *testing.T) {
	for _, value := range []string{"a\nb", "a\rb", "a\tb", "a\x00b", "a\x7fb", " a", "a ", "a\xffb"} {
		if PlanID(value).Validate() == nil {
			t.Errorf("PlanID accepted %q", value)
		}
	}
}

func TestPhysicalProjectIdentityValidationAndDefensiveCopy(t *testing.T) {
	path := []byte("/tmp/project")
	identity, err := NewPhysicalProjectIdentity("/tmp/project", path, "9007199254740993", "42")
	if err != nil {
		t.Fatal(err)
	}
	if want := ComputeProjectIdentityDigest(path, "9007199254740993", "42"); identity.Digest() != want {
		t.Fatalf("digest=%q want=%q", identity.Digest(), want)
	}
	path[0] = 'X'
	got := identity.ReversiblePathBytes()
	if string(got) != "/tmp/project" {
		t.Fatalf("constructor retained caller bytes: %q", got)
	}
	got[0] = 'Y'
	if string(identity.ReversiblePathBytes()) != "/tmp/project" {
		t.Fatal("accessor exposed mutable internal bytes")
	}
	bad := identity
	bad.digest = ProjectIdentityDigest(testSHA256)
	if bad.Validate() == nil {
		t.Fatal("inconsistent identity digest accepted")
	}
	if identity.IsZero() || !(PhysicalProjectIdentity{}).IsZero() {
		t.Fatal("IsZero does not distinguish the unset identity")
	}
	if _, err := NewPhysicalProjectIdentity("/other", []byte("/tmp/project"), "1", "2"); err == nil {
		t.Fatal("inconsistent display path accepted")
	}
	if _, err := NewPhysicalProjectIdentity("", []byte{0xff}, "1", "2"); err != nil {
		t.Fatalf("non-UTF-8 reversible path with no display rejected: %v", err)
	}
}

func TestObservedPathAndRef(t *testing.T) {
	utf := NewObservedPath([]byte("café/file"))
	if utf.DisplayPathUTF8 != "café/file" || utf.Validate() != nil || string(utf.Bytes()) != "café/file" {
		t.Fatalf("UTF-8 path = %+v, %v", utf, utf.Validate())
	}
	raw := NewObservedPath([]byte{0xff, '/', 'a'})
	if raw.DisplayPathUTF8 != "" || raw.Validate() != nil {
		t.Fatalf("non-UTF-8 path = %+v, %v", raw, raw.Validate())
	}
	for name, bad := range map[string]ObservedPath{
		"empty":                {},
		"empty bytes":          NewObservedPath(nil),
		"display mismatch":     {ReversiblePathBytesBase64URL: utf.ReversiblePathBytesBase64URL, DisplayPathUTF8: "cafe/file"},
		"display missing":      {ReversiblePathBytesBase64URL: utf.ReversiblePathBytesBase64URL},
		"display on non-UTF-8": {ReversiblePathBytesBase64URL: raw.ReversiblePathBytesBase64URL, DisplayPathUTF8: "x"},
	} {
		if bad.Validate() == nil {
			t.Errorf("path %s accepted", name)
		}
	}

	for _, name := range [][]byte{[]byte("refs/heads/main"), []byte("refs/tags/v1"), {'r', 'e', 'f', 's', '/', 0xff}} {
		if err := NewObservedRef(name).Validate(); err != nil {
			t.Errorf("ref %q rejected: %v", name, err)
		}
	}
	for _, name := range []string{"HEAD", "heads/main", "refs/", "refs/heads/a\x00b", ""} {
		if NewObservedRef([]byte(name)).Validate() == nil {
			t.Errorf("ref %q accepted", name)
		}
	}
}

func TestAuthorizedFileStrictValidation(t *testing.T) {
	valid := validAuthorizedFile("café/file.txt")
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../../etc/passwd", "/absolute/path", "a/../b", "a/./b", "a//b", "a\x00b", "a\nb", "a|b", "*.go", "dir/"} {
		if validAuthorizedFile(path).Validate() == nil {
			t.Errorf("unsafe path %q accepted", path)
		}
	}
	for name, mutate := range map[string]func(*AuthorizedFile){
		"unauthorized mode":         func(f *AuthorizedFile) { f.Mode = "100600" },
		"inconsistent reversible":   func(f *AuthorizedFile) { f.ReversiblePathBytesBase64URL = EncodeReversiblePath([]byte("other")) },
		"not individually approved": func(f *AuthorizedFile) { f.IndividuallyApproved = false },
		"ignored without approval":  func(f *AuthorizedFile) { f.Ignored = true },
		"missing observed identity": func(f *AuthorizedFile) { f.ObservedIdentity = FileIdentity{} },
		"observed directory":        func(f *AuthorizedFile) { f.ObservedIdentity.Mode = fs.ModeDir | 0o755 },
		"invalid observed inode":    func(f *AuthorizedFile) { f.ObservedIdentity.InodeDecimal = "01" },
		"negative observed size":    func(f *AuthorizedFile) { f.ObservedIdentity.Size = -1 },
	} {
		bad := valid
		mutate(&bad)
		if bad.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
	ignored := valid
	ignored.Ignored, ignored.ApprovalKind = true, ApprovalKindIgnored
	if err := ignored.Validate(); err != nil {
		t.Fatalf("ignored approval form rejected: %v", err)
	}
}

func TestRedactedRemotes(t *testing.T) {
	plain := RedactedRemoteURL{Ordinal: 0, DisplayRedacted: "https://example.test/repo.git", RedactionState: RedactionNotNeeded}
	redacted := RedactedRemoteURL{Ordinal: 1, DisplayRedacted: "https://***@example.test/repo.git", CredentialMaterialDetected: true, RedactionState: RedactionRedacted}
	opaque := RedactedRemoteURL{Ordinal: 0, DisplayRedacted: OpaqueMarker, CredentialMaterialDetected: true, RedactionState: RedactionOpaque}
	for name, url := range map[string]RedactedRemoteURL{"plain": plain, "redacted": redacted, "opaque": opaque} {
		if err := url.Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, bad := range map[string]RedactedRemoteURL{
		"opaque state without marker":   {DisplayRedacted: "https://example.test", RedactionState: RedactionOpaque},
		"marker without opaque state":   {DisplayRedacted: OpaqueMarker, RedactionState: RedactionRedacted},
		"credentials marked not needed": {DisplayRedacted: "https://example.test", CredentialMaterialDetected: true, RedactionState: RedactionNotNeeded},
		"empty display":                 {RedactionState: RedactionNotNeeded},
		"invalid original digest":       {DisplayRedacted: "https://example.test", RedactionState: RedactionNotNeeded, OriginalDigest: "sha256:abc"},
	} {
		if bad.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}

	base := func() RemoteRedactedInput {
		return RemoteRedactedInput{Name: "origin", FetchURLs: []RedactedRemoteURL{plain, redacted}, PushURLs: []RedactedRemoteURL{opaque}, SourceScope: "local", SourceDigest: testSHA256, ObservationState: ObservationObserved}
	}
	in := base()
	remote, err := NewRemoteRedacted(in)
	if err != nil {
		t.Fatal(err)
	}
	in.FetchURLs[0].DisplayRedacted = "changed"
	remote.FetchURLs()[1].DisplayRedacted = "changed"
	if got := remote.FetchURLs(); got[0] != plain || got[1] != redacted {
		t.Fatal("remote exposed or retained caller-owned URLs")
	}
	for name, mutate := range map[string]func(*RemoteRedactedInput){
		"control in name":       func(in *RemoteRedactedInput) { in.Name = "ori\ngin" },
		"unordered ordinals":    func(in *RemoteRedactedInput) { in.FetchURLs = []RedactedRemoteURL{redacted, plain} },
		"observed without URLs": func(in *RemoteRedactedInput) { in.FetchURLs, in.PushURLs = nil, nil },
		"empty state with URLs": func(in *RemoteRedactedInput) { in.ObservationState = ObservationObservedEmpty },
		"error state with URLs": func(in *RemoteRedactedInput) { in.ObservationState = ObservationError },
		"not applicable entry":  func(in *RemoteRedactedInput) { in.ObservationState = ObservationNotApplicable },
		"missing source digest": func(in *RemoteRedactedInput) { in.SourceDigest = "" },
		"missing source scope":  func(in *RemoteRedactedInput) { in.SourceScope = "" },
		"invalid nested URL":    func(in *RemoteRedactedInput) { in.PushURLs = []RedactedRemoteURL{{RedactionState: RedactionOpaque}} },
	} {
		bad := base()
		mutate(&bad)
		if _, err := NewRemoteRedacted(bad); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestGitFlowEvidenceClassification(t *testing.T) {
	entry := func(key string) GitFlowEntry {
		return GitFlowEntry{Key: key, Value: "value", Origin: "local", SourceDigest: testSHA256}
	}
	entries := []GitFlowEntry{entry("gitflow.branch.develop"), entry("gitflow.branch.master")}
	prefixes := []GitFlowEntry{entry("gitflow.prefix.feature")}
	valid := map[string]GitFlowEvidenceInput{
		"absent":        {ObservationState: ObservationObservedEmpty, Classification: GitFlowAbsent},
		"absent state":  {ObservationState: ObservationAbsent, Classification: GitFlowAbsent},
		"complete avh":  {ObservationState: ObservationObserved, Classification: GitFlowCompleteAVH, Dialect: GitFlowDialectAVH, Entries: entries, Prefixes: prefixes, ProductionBranch: "master", DevelopmentBranch: "develop", SourceDigests: []ContentDigest{testSHA256}},
		"complete next": {ObservationState: ObservationObserved, Classification: GitFlowCompleteGitFlowNext, Dialect: GitFlowDialectNext, Entries: entries, Prefixes: prefixes, ProductionBranch: "main", DevelopmentBranch: "develop", SourceDigests: []ContentDigest{testSHA256}},
		"partial":       {ObservationState: ObservationObserved, Classification: GitFlowPartial, Dialect: GitFlowDialectAVH, Entries: entries[:1], SourceDigests: []ContentDigest{testSHA256}},
		"mixed":         {ObservationState: ObservationObserved, Classification: GitFlowMixed, Entries: entries, SourceDigests: []ContentDigest{testSHA256}, Issues: []string{"keys from both profiles"}},
		"contradictory": {ObservationState: ObservationObserved, Classification: GitFlowContradictory, Entries: entries, SourceDigests: []ContentDigest{testSHA256}},
		"unrecognized":  {ObservationState: ObservationObserved, Classification: GitFlowUnrecognized, InstanceBases: []GitFlowEntry{entry("gitflow.branch.feature/x.base")}, SourceDigests: []ContentDigest{testSHA256}},
		"unobservable":  {ObservationState: ObservationError, Classification: GitFlowUnobservable, Issues: []string{"config unreadable"}},
	}
	for name, in := range valid {
		if _, err := NewGitFlowEvidence(in); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	invalidCases := map[string]GitFlowEvidenceInput{
		"absent with entries":        {ObservationState: ObservationObservedEmpty, Classification: GitFlowAbsent, Entries: entries},
		"absent not observed":        {ObservationState: ObservationNotObserved, Classification: GitFlowAbsent},
		"complete wrong dialect":     {ObservationState: ObservationObserved, Classification: GitFlowCompleteAVH, Dialect: GitFlowDialectNext, Entries: entries, Prefixes: prefixes, ProductionBranch: "master", DevelopmentBranch: "develop"},
		"complete without branches":  {ObservationState: ObservationObserved, Classification: GitFlowCompleteAVH, Dialect: GitFlowDialectAVH, Entries: entries, Prefixes: prefixes},
		"complete without prefixes":  {ObservationState: ObservationObserved, Classification: GitFlowCompleteAVH, Dialect: GitFlowDialectAVH, Entries: entries, ProductionBranch: "master", DevelopmentBranch: "develop"},
		"partial without entries":    {ObservationState: ObservationObserved, Classification: GitFlowPartial, SourceDigests: []ContentDigest{testSHA256}},
		"partial without digests":    {ObservationState: ObservationObserved, Classification: GitFlowPartial, Entries: entries},
		"mixed with dialect":         {ObservationState: ObservationObserved, Classification: GitFlowMixed, Dialect: GitFlowDialectAVH, Entries: entries, SourceDigests: []ContentDigest{testSHA256}},
		"unobservable without issue": {ObservationState: ObservationError, Classification: GitFlowUnobservable},
		"unobservable but observed":  {ObservationState: ObservationObserved, Classification: GitFlowUnobservable, Issues: []string{"x"}},
		"unsorted entries":           {ObservationState: ObservationObserved, Classification: GitFlowMixed, Entries: []GitFlowEntry{entries[1], entries[0]}, SourceDigests: []ContentDigest{testSHA256}},
		"foreign key":                {ObservationState: ObservationObserved, Classification: GitFlowUnrecognized, Entries: []GitFlowEntry{entry("core.bare")}, SourceDigests: []ContentDigest{testSHA256}},
		"unknown classification":     {ObservationState: ObservationObserved, Classification: "other"},
	}
	for name, in := range invalidCases {
		if _, err := NewGitFlowEvidence(in); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestAttributeAndIgnoreSources(t *testing.T) {
	read := InspectedSourceIdentity{Scope: "local", Path: NewObservedPath([]byte(".gitattributes")), PhysicalIdentityDigest: testSHA256, ContentDigest: testOtherSHA256, ReadState: SourceReadRead, ObservedSizeDecimal: "12", Precedence: 2}
	absent := InspectedSourceIdentity{Scope: "global", Path: NewObservedPath([]byte("/home/u/.config/git/attributes")), ReadState: SourceReadAbsent}
	for name, identity := range map[string]InspectedSourceIdentity{"read": read, "absent": absent, "unreadable": {Scope: "system", ReadState: SourceReadUnreadable}} {
		if err := identity.Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, mutate := range map[string]func(*InspectedSourceIdentity){
		"read without digest":   func(v *InspectedSourceIdentity) { v.ContentDigest = "" },
		"read without identity": func(v *InspectedSourceIdentity) { v.PhysicalIdentityDigest = "" },
		"read without path":     func(v *InspectedSourceIdentity) { v.Path = ObservedPath{} },
		"missing scope":         func(v *InspectedSourceIdentity) { v.Scope = "" },
		"invalid size":          func(v *InspectedSourceIdentity) { v.ObservedSizeDecimal = "012" },
		"unknown read state":    func(v *InspectedSourceIdentity) { v.ReadState = "other" },
	} {
		bad := read
		mutate(&bad)
		if bad.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
	fabricated := absent
	fabricated.ContentDigest = testSHA256
	if fabricated.Validate() == nil {
		t.Error("absent source with fabricated digest accepted")
	}

	if err := (AttributeSource{Identity: read, Kind: AttributeSourceRepository}).Validate(); err != nil {
		t.Fatal(err)
	}
	if (AttributeSource{Identity: InspectedSourceIdentity{Scope: "local", ReadState: SourceReadUnreadable}, Kind: AttributeSourceRepository}).Validate() == nil {
		t.Error("repository attribute source without path accepted")
	}
	if (AttributeSource{Identity: read, Kind: AttributeSourcePostInitIntent}).Validate() == nil {
		t.Error("post-init attribute intent with observed physical identity accepted")
	}
	if (AttributeSource{Identity: read, Kind: "repository_gitignore"}).Validate() == nil {
		t.Error("ignore kind accepted as attribute source kind")
	}

	ignore := IgnoreSource{Identity: read, Kind: IgnoreSourceRepository, BaseDirectory: NewObservedPath([]byte(".")), HasDirectoryDepth: true}
	if err := ignore.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*IgnoreSource){
		"repository without base":  func(v *IgnoreSource) { v.BaseDirectory = ObservedPath{} },
		"repository without depth": func(v *IgnoreSource) { v.HasDirectoryDepth = false },
		"depth without flag": func(v *IgnoreSource) {
			v.Kind, v.HasDirectoryDepth, v.DirectoryDepth = IgnoreSourceInfoExclude, false, 3
		},
		"attribute kind":          func(v *IgnoreSource) { v.Kind = "repository_gitattributes" },
		"post-init with identity": func(v *IgnoreSource) { v.Kind = IgnoreSourcePostInitIntent },
	} {
		bad := ignore
		mutate(&bad)
		if bad.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestIgnoreEvaluation(t *testing.T) {
	base := func() IgnoreEvaluationInput {
		return IgnoreEvaluationInput{Path: NewObservedPath([]byte("build/out.bin")), OrderedSourceDigests: []ContentDigest{testSHA256, testOtherSHA256}, MatchedRules: []string{"build/"}, Ignored: true, WinningRuleSource: testOtherSHA256, EquivalenceProven: true}
	}
	in := base()
	evaluation, err := NewIgnoreEvaluation(in)
	if err != nil {
		t.Fatal(err)
	}
	in.OrderedSourceDigests[0] = "changed"
	if evaluation.OrderedSourceDigests()[0] != testSHA256 {
		t.Fatal("evaluation retained caller-owned digests")
	}
	for name, mutate := range map[string]func(*IgnoreEvaluationInput){
		"ignored without winner":  func(in *IgnoreEvaluationInput) { in.WinningRuleSource = "" },
		"winner outside sources":  func(in *IgnoreEvaluationInput) { in.OrderedSourceDigests = []ContentDigest{testSHA256} },
		"proven yet incompatible": func(in *IgnoreEvaluationInput) { in.IncompatibilityReason = ReasonAttributesIncompatible },
		"foreign incompatibility": func(in *IgnoreEvaluationInput) {
			in.EquivalenceProven, in.IncompatibilityReason = false, ReasonPlanStale
		},
		"missing path":       func(in *IgnoreEvaluationInput) { in.Path = ObservedPath{} },
		"empty matched rule": func(in *IgnoreEvaluationInput) { in.MatchedRules = []string{""} },
	} {
		bad := base()
		mutate(&bad)
		if _, err := NewIgnoreEvaluation(bad); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := NewIgnoreEvaluation(IgnoreEvaluationInput{Path: NewObservedPath([]byte("a")), IncompatibilityReason: ReasonAttributesIncompatible}); err != nil {
		t.Fatalf("unproven incompatible evaluation rejected: %v", err)
	}
}

func TestConversionSettingsRiskAndProof(t *testing.T) {
	setting := EffectiveConversionSetting{Key: "core.autocrlf", ConfiguredValueRedacted: "false", EffectiveValueRedacted: "false", SourceScope: "global", SourceDigest: testSHA256, InterpretationState: "interpreted"}
	settings := []EffectiveConversionSetting{setting}
	valid := map[string]ConversionSettingsInput{
		"none":          {ObservationState: ObservationObservedEmpty, Risk: ConversionRiskNone, ByteIdentityProven: true},
		"neutral":       {ObservationState: ObservationObserved, Risk: ConversionRiskNeutral, Settings: settings, ByteIdentityProven: true, SourceDigests: []ContentDigest{testSHA256}},
		"may transform": {ObservationState: ObservationObserved, Risk: ConversionRiskMayTransform, Settings: settings},
		"external code": {ObservationState: ObservationObserved, Risk: ConversionRiskExternalCode, FilterDrivers: []string{"lfs"}, ExternalCodeRequired: true},
		"unknown":       {ObservationState: ObservationError, Risk: ConversionRiskUnknown},
		"incompatible":  {ObservationState: ObservationObserved, Risk: ConversionRiskIncompatible, Settings: settings, IncompatibilityReason: ReasonAttributesIncompatible},
	}
	for name, in := range valid {
		if _, err := NewConversionSettings(in); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	invalidCases := map[string]ConversionSettingsInput{
		"none with settings":            {ObservationState: ObservationObserved, Risk: ConversionRiskNone, Settings: settings},
		"none unobserved":               {ObservationState: ObservationNotObserved, Risk: ConversionRiskNone},
		"neutral without settings":      {ObservationState: ObservationObserved, Risk: ConversionRiskNeutral},
		"proof with external code":      {ObservationState: ObservationObserved, Risk: ConversionRiskNeutral, Settings: settings, ExternalCodeRequired: true, ByteIdentityProven: true},
		"proof with may transform":      {ObservationState: ObservationObserved, Risk: ConversionRiskMayTransform, Settings: settings, ByteIdentityProven: true},
		"proof with unknown":            {ObservationState: ObservationObserved, Risk: ConversionRiskUnknown, ByteIdentityProven: true},
		"proof with incompatible":       {ObservationState: ObservationObserved, Risk: ConversionRiskIncompatible, Settings: settings, ByteIdentityProven: true, IncompatibilityReason: ReasonAttributesIncompatible},
		"external risk without flag":    {ObservationState: ObservationObserved, Risk: ConversionRiskExternalCode, FilterDrivers: []string{"lfs"}},
		"incompatible without reason":   {ObservationState: ObservationObserved, Risk: ConversionRiskIncompatible, Settings: settings},
		"reason without incompatible":   {ObservationState: ObservationObserved, Risk: ConversionRiskMayTransform, Settings: settings, IncompatibilityReason: ReasonAttributesIncompatible},
		"foreign reason":                {ObservationState: ObservationObserved, Risk: ConversionRiskIncompatible, Settings: settings, IncompatibilityReason: ReasonPlanStale},
		"error state claiming risk":     {ObservationState: ObservationError, Risk: ConversionRiskMayTransform, Settings: settings},
		"setting without source digest": {ObservationState: ObservationObserved, Risk: ConversionRiskMayTransform, Settings: []EffectiveConversionSetting{{Key: "core.eol", SourceScope: "local", InterpretationState: "interpreted"}}},
	}
	for name, in := range invalidCases {
		if _, err := NewConversionSettings(in); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestHeadObservation(t *testing.T) {
	main := NewObservedRef([]byte("refs/heads/main"))
	valid := map[string]HeadObservation{
		"attached":              {State: HeadStateAttached, SymbolicRef: main, CommitOID: testSHA1OID, HistoryPresent: true},
		"detached":              {State: HeadStateDetached, CommitOID: testSHA1OID, HistoryPresent: true},
		"unborn":                {State: HeadStateUnborn},
		"unborn named branch":   {State: HeadStateUnborn, SymbolicRef: main},
		"unborn with history":   {State: HeadStateUnborn, SymbolicRef: main, HistoryPresent: true},
		"invalid":               {State: HeadStateInvalid},
		"invalid symbolic name": {State: HeadStateInvalid, SymbolicRef: main},
	}
	for name, head := range valid {
		if err := head.Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	invalidCases := map[string]HeadObservation{
		"attached without ref":     {State: HeadStateAttached, CommitOID: testSHA1OID, HistoryPresent: true},
		"attached without commit":  {State: HeadStateAttached, SymbolicRef: main, HistoryPresent: true},
		"attached without history": {State: HeadStateAttached, SymbolicRef: main, CommitOID: testSHA1OID},
		"detached with ref":        {State: HeadStateDetached, SymbolicRef: main, CommitOID: testSHA1OID, HistoryPresent: true},
		"detached without commit":  {State: HeadStateDetached, HistoryPresent: true},
		"unborn with commit":       {State: HeadStateUnborn, CommitOID: testSHA1OID},
		"invalid with commit":      {State: HeadStateInvalid, CommitOID: testSHA1OID},
		"symbolic ref not a ref":   {State: HeadStateUnborn, SymbolicRef: NewObservedRef([]byte("HEAD"))},
		"unknown state":            {State: "other"},
	}
	for name, head := range invalidCases {
		if head.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestIndexObservation(t *testing.T) {
	change := func(path string, kind IndexChangeKind) IndexChange {
		return IndexChange{Path: NewObservedPath([]byte(path)), Kind: kind}
	}
	valid := map[string]IndexObservationInput{
		"absent":        {Presence: IndexAbsent, LockState: LockFileAbsent},
		"unreadable":    {Presence: IndexUnreadable, LockState: LockFileUnobservable},
		"empty present": {Presence: IndexPresent, Version: 2, EntryCountDecimal: "0", ContentDigest: testSHA256, LockState: LockFileAbsent},
		"populated":     {Presence: IndexPresent, Version: 4, EntryCountDecimal: "3", ContentDigest: testSHA256, LockState: LockFilePresent, Extensions: []string{"TREE", "link"}, Changes: []IndexChange{change("a", IndexChangeConflicted), change("a", IndexChangeStaged), change("b", IndexChangeModeChanged)}},
	}
	for name, in := range valid {
		if _, err := NewIndexObservation(in); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	empty, _ := NewIndexObservation(valid["empty present"])
	absent, _ := NewIndexObservation(valid["absent"])
	populated, _ := NewIndexObservation(valid["populated"])
	if !empty.Empty() || !absent.Empty() || populated.Empty() {
		t.Fatal("Empty does not follow presence and entry count")
	}
	invalidCases := map[string]IndexObservationInput{
		"absent with digest":       {Presence: IndexAbsent, LockState: LockFileAbsent, ContentDigest: testSHA256},
		"unreadable with version":  {Presence: IndexUnreadable, LockState: LockFileAbsent, Version: 2},
		"present without version":  {Presence: IndexPresent, EntryCountDecimal: "1", ContentDigest: testSHA256, LockState: LockFileAbsent},
		"version five":             {Presence: IndexPresent, Version: 5, EntryCountDecimal: "1", ContentDigest: testSHA256, LockState: LockFileAbsent},
		"present without count":    {Presence: IndexPresent, Version: 2, ContentDigest: testSHA256, LockState: LockFileAbsent},
		"present without digest":   {Presence: IndexPresent, Version: 2, EntryCountDecimal: "1", LockState: LockFileAbsent},
		"empty index with changes": {Presence: IndexPresent, Version: 2, EntryCountDecimal: "0", ContentDigest: testSHA256, LockState: LockFileAbsent, Changes: []IndexChange{change("a", IndexChangeStaged)}},
		"unsorted changes":         {Presence: IndexPresent, Version: 2, EntryCountDecimal: "2", ContentDigest: testSHA256, LockState: LockFileAbsent, Changes: []IndexChange{change("b", IndexChangeStaged), change("a", IndexChangeStaged)}},
		"duplicate change":         {Presence: IndexPresent, Version: 2, EntryCountDecimal: "2", ContentDigest: testSHA256, LockState: LockFileAbsent, Changes: []IndexChange{change("a", IndexChangeStaged), change("a", IndexChangeStaged)}},
		"bad extension signature":  {Presence: IndexPresent, Version: 2, EntryCountDecimal: "1", ContentDigest: testSHA256, LockState: LockFileAbsent, Extensions: []string{"TOOLONG"}},
		"unknown lock state":       {Presence: IndexAbsent, LockState: "other"},
		"unknown change kind":      {Presence: IndexPresent, Version: 2, EntryCountDecimal: "1", ContentDigest: testSHA256, LockState: LockFileAbsent, Changes: []IndexChange{change("a", "other")}},
	}
	for name, in := range invalidCases {
		if _, err := NewIndexObservation(in); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	in := valid["populated"]
	index, _ := NewIndexObservation(in)
	in.Changes[0].Kind = IndexChangeStaged
	index.Extensions()[0] = "XXXX"
	if index.Changes()[0].Kind != IndexChangeConflicted || index.Extensions()[0] != "TREE" {
		t.Fatal("index retained or exposed caller-owned collections")
	}
}

func TestConfigEntry(t *testing.T) {
	valid := map[string]ConfigEntry{
		"local":            {Key: "core.autocrlf", ValuePresent: true, ValueRedacted: "false", ValueRedaction: RedactionNotNeeded, Scope: ConfigScopeLocal, Origin: NewObservedPath([]byte("/p/.git/config")), SourceDigest: testSHA256},
		"boolean key":      {Key: "core.bare", ValueRedaction: RedactionNotNeeded, Scope: ConfigScopeSystem, Origin: NewObservedPath([]byte("/etc/gitconfig")), SourceDigest: testSHA256},
		"empty value":      {Key: "user.name", ValuePresent: true, ValueRedaction: RedactionNotNeeded, Scope: ConfigScopeGlobal, Origin: NewObservedPath([]byte("/h/.gitconfig")), SourceDigest: testSHA256},
		"subsection dots":  {Key: "filter.My.Driver.clean", ValuePresent: true, ValueRedacted: "x", ValueRedaction: RedactionNotNeeded, Scope: ConfigScopeXDG, Origin: NewObservedPath([]byte("/h/.config/git/config")), SourceDigest: testSHA256},
		"redacted value":   {Key: "http.extraheader", ValuePresent: true, ValueRedacted: "Authorization: ***", ValueRedaction: RedactionRedacted, Scope: ConfigScopeWorktree, Origin: NewObservedPath([]byte("/p/.git/config.worktree")), SourceDigest: testSHA256},
		"opaque value":     {Key: "remote.origin.url", ValuePresent: true, ValueRedacted: OpaqueMarker, ValueRedaction: RedactionOpaque, Scope: ConfigScopeLocal, Origin: NewObservedPath([]byte("/p/.git/config")), SourceDigest: testSHA256},
		"command override": {Key: "core.fsmonitor", ValuePresent: true, ValueRedacted: "false", ValueRedaction: RedactionNotNeeded, Scope: ConfigScopeCommand},
	}
	for name, entry := range valid {
		if err := entry.Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	base := valid["local"]
	for name, mutate := range map[string]func(*ConfigEntry){
		"uppercase section":      func(v *ConfigEntry) { v.Key = "Core.autocrlf" },
		"uppercase name":         func(v *ConfigEntry) { v.Key = "core.autoCRLF" },
		"no section":             func(v *ConfigEntry) { v.Key = "autocrlf" },
		"empty name":             func(v *ConfigEntry) { v.Key = "core." },
		"name starts with digit": func(v *ConfigEntry) { v.Key = "core.1x" },
		"newline in subsection":  func(v *ConfigEntry) { v.Key = "filter.a\nb.clean" },
		"value without presence": func(v *ConfigEntry) { v.ValuePresent = false },
		"opaque without marker":  func(v *ConfigEntry) { v.ValueRedaction = RedactionOpaque },
		"marker without opaque":  func(v *ConfigEntry) { v.ValueRedacted = OpaqueMarker },
		"NUL in value":           func(v *ConfigEntry) { v.ValueRedacted = "a\x00b" },
		"file scope no origin":   func(v *ConfigEntry) { v.Origin = ObservedPath{} },
		"file scope no digest":   func(v *ConfigEntry) { v.SourceDigest = "" },
		"command with origin":    func(v *ConfigEntry) { v.Scope = ConfigScopeCommand },
		"unknown scope":          func(v *ConfigEntry) { v.Scope = "other" },
	} {
		bad := base
		mutate(&bad)
		if bad.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestProjectConfigObservation(t *testing.T) {
	path := NewObservedPath([]byte("/tmp/project/.ai-dev/config.json"))
	identity := regularFileIdentity("7")
	valid := map[string]ProjectConfigObservation{
		"absent":     {State: ProjectConfigAbsent, Path: path},
		"unreadable": {State: ProjectConfigUnreadable, Path: path},
		"valid":      {State: ProjectConfigValid, Path: path, Identity: identity, ContentDigest: testSHA256, ProjectID: testProjectID},
		"invalid":    {State: ProjectConfigInvalid, Path: path, Identity: identity, ContentDigest: testSHA256},
	}
	for name, observation := range valid {
		if err := observation.Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	invalidCases := map[string]ProjectConfigObservation{
		"absent with digest":     {State: ProjectConfigAbsent, Path: path, ContentDigest: testSHA256},
		"unreadable with ID":     {State: ProjectConfigUnreadable, Path: path, ProjectID: testProjectID},
		"valid without ID":       {State: ProjectConfigValid, Path: path, Identity: identity, ContentDigest: testSHA256},
		"valid without identity": {State: ProjectConfigValid, Path: path, ContentDigest: testSHA256, ProjectID: testProjectID},
		"invalid with ID":        {State: ProjectConfigInvalid, Path: path, Identity: identity, ContentDigest: testSHA256, ProjectID: testProjectID},
		"missing path":           {State: ProjectConfigAbsent},
		"unknown state":          {State: "other", Path: path},
	}
	for name, observation := range invalidCases {
		if observation.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestRepositoryObservation(t *testing.T) {
	if _, err := NewRepositoryObservation(validObservationInput(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepositoryObservation(absentObservationInput(t)); err != nil {
		t.Fatalf("absent repository observation rejected: %v", err)
	}

	populated := validObservationInput(t)
	populated.RefsObservationState = ObservationObserved
	populated.Refs = []RefObservation{
		{Name: NewObservedRef([]byte("refs/heads/main")), TargetOID: testSHA1OID},
		{Name: NewObservedRef([]byte("refs/tags/v1")), TargetOID: testOtherSHA1OID, PeeledOID: testSHA1OID},
	}
	populated.WorktreeObservationState = ObservationObserved
	populated.WorktreeChanges = []WorktreeChange{
		{Path: NewObservedPath([]byte("notes.txt")), Kind: WorktreeChangeUntracked},
		{Path: NewObservedPath([]byte("src/a.go")), Kind: WorktreeChangeUnstaged},
	}
	populated.WorktreePathIdentities = []WorktreePathIdentity{
		{Path: NewObservedPath([]byte("notes.txt")), Identity: regularFileIdentity("10"), ContentDigest: testSHA256},
		{Path: NewObservedPath([]byte("src/a.go")), Identity: regularFileIdentity("11"), ContentDigest: testOtherSHA256},
	}
	populated.ConfigurationObservationState = ObservationObserved
	populated.EffectiveConfiguration = []ConfigEntry{
		{Ordinal: 0, Key: "core.bare", ValuePresent: true, ValueRedacted: "false", ValueRedaction: RedactionNotNeeded, Scope: ConfigScopeLocal, Origin: NewObservedPath([]byte("/tmp/project/.git/config")), SourceDigest: testSHA256},
	}
	observation, err := NewRepositoryObservation(populated)
	if err != nil {
		t.Fatalf("populated observation rejected: %v", err)
	}
	populated.Refs[0].TargetOID = testOtherSHA1OID
	observation.WorktreeChanges()[0].Kind = WorktreeChangeIgnored
	if observation.Refs()[0].TargetOID != testSHA1OID || observation.WorktreeChanges()[0].Kind != WorktreeChangeUntracked {
		t.Fatal("observation retained or exposed caller-owned collections")
	}

	for name, mutate := range map[string]func(*RepositoryObservationInput){
		"unsorted refs":                func(in *RepositoryObservationInput) { in.Refs[0], in.Refs[1] = in.Refs[1], in.Refs[0] },
		"duplicate ref":                func(in *RepositoryObservationInput) { in.Refs[1] = in.Refs[0] },
		"ref of another object format": func(in *RepositoryObservationInput) { in.Refs[0].TargetOID = testSHA256OID },
		"refs claimed but empty":       func(in *RepositoryObservationInput) { in.Refs = nil },
		"unsorted worktree changes": func(in *RepositoryObservationInput) {
			in.WorktreeChanges[0], in.WorktreeChanges[1] = in.WorktreeChanges[1], in.WorktreeChanges[0]
		},
		"change without path identity": func(in *RepositoryObservationInput) { in.WorktreePathIdentities = in.WorktreePathIdentities[:1] },
		"unsorted path identities": func(in *RepositoryObservationInput) {
			in.WorktreePathIdentities[0], in.WorktreePathIdentities[1] = in.WorktreePathIdentities[1], in.WorktreePathIdentities[0]
		},
		"regular file without digest":  func(in *RepositoryObservationInput) { in.WorktreePathIdentities[0].ContentDigest = "" },
		"configuration ordinal gap":    func(in *RepositoryObservationInput) { in.EffectiveConfiguration[0].Ordinal = 1 },
		"normal without Git dir":       func(in *RepositoryObservationInput) { in.GitDir = ObservedPath{} },
		"normal without object format": func(in *RepositoryObservationInput) { in.ObjectFormat = "" },
		"unknown ref format":           func(in *RepositoryObservationInput) { in.RefFormat = "other" },
		"unknown sequencer state":      func(in *RepositoryObservationInput) { in.SequencerState = "other" },
		"missing config lock state":    func(in *RepositoryObservationInput) { in.ConfigLockState = "" },
		"missing group state":          func(in *RepositoryObservationInput) { in.SubmodulesObservationState = "" },
		"duplicate filesystem role": func(in *RepositoryObservationInput) {
			in.FilesystemCapabilities = append(in.FilesystemCapabilities, in.FilesystemCapabilities[0])
		},
		"zero Git Flow evidence":   func(in *RepositoryObservationInput) { in.GitFlowEvidence = GitFlowEvidence{} },
		"zero conversion settings": func(in *RepositoryObservationInput) { in.ConversionSettings = ConversionSettings{} },
		"zero project config":      func(in *RepositoryObservationInput) { in.ProjectConfig = ProjectConfigObservation{} },
		"zero index":               func(in *RepositoryObservationInput) { in.Index = IndexObservation{} },
	} {
		bad := populated
		bad.Refs = cloneSlice(populated.Refs)
		bad.Refs[0].TargetOID = testSHA1OID
		bad.WorktreeChanges = cloneSlice(populated.WorktreeChanges)
		bad.WorktreePathIdentities = cloneSlice(populated.WorktreePathIdentities)
		bad.EffectiveConfiguration = cloneSlice(populated.EffectiveConfiguration)
		bad.FilesystemCapabilities = cloneSlice(populated.FilesystemCapabilities)
		mutate(&bad)
		if _, err := NewRepositoryObservation(bad); err == nil {
			t.Errorf("%s accepted", name)
		}
	}

	for name, mutate := range map[string]func(*RepositoryObservationInput){
		"absent with Git dir":       func(in *RepositoryObservationInput) { in.GitDir = NewObservedPath([]byte("/p/.git")) },
		"absent with history":       func(in *RepositoryObservationInput) { in.Head.HistoryPresent = true },
		"absent with sequencer":     func(in *RepositoryObservationInput) { in.SequencerState = SequencerMerge },
		"absent with refs observed": func(in *RepositoryObservationInput) { in.RefsObservationState = ObservationObservedEmpty },
		"absent with config lock":   func(in *RepositoryObservationInput) { in.ConfigLockState = LockFilePresent },
		"absent with index": func(in *RepositoryObservationInput) {
			in.Index = mustIndex(t, IndexObservationInput{Presence: IndexUnreadable, LockState: LockFileAbsent})
		},
	} {
		bad := absentObservationInput(t)
		mutate(&bad)
		if _, err := NewRepositoryObservation(bad); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestLinkedWorktreeObservation(t *testing.T) {
	valid := LinkedWorktree{ObservationState: ObservationObserved, WorktreePath: NewObservedPath([]byte("/tmp/wt")), GitDirPath: NewObservedPath([]byte("/tmp/project/.git/worktrees/wt")), CommonDirIdentity: testSHA256, Head: HeadObservation{State: HeadStateAttached, SymbolicRef: NewObservedRef([]byte("refs/heads/topic")), CommitOID: testSHA1OID, HistoryPresent: true}, Availability: WorktreeAvailable}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	locked := valid
	locked.Availability, locked.Locked, locked.LockReasonRedacted = WorktreeLocked, true, "maintenance"
	locked.PrunableObserved, locked.Prunable = true, false
	if err := locked.Validate(); err != nil {
		t.Fatalf("locked worktree rejected: %v", err)
	}
	for name, mutate := range map[string]func(*LinkedWorktree){
		"available without common dir": func(v *LinkedWorktree) { v.CommonDirIdentity = "" },
		"available with error state":   func(v *LinkedWorktree) { v.ObservationState = ObservationError },
		"inconsistent HEAD":            func(v *LinkedWorktree) { v.Head.SymbolicRef = ObservedRef{} },
		"missing path":                 func(v *LinkedWorktree) { v.WorktreePath = ObservedPath{} },
		"locked availability unlocked": func(v *LinkedWorktree) { v.Availability = WorktreeLocked },
		"lock reason while unlocked":   func(v *LinkedWorktree) { v.LockReasonRedacted = "reason" },
		"prunable never observed":      func(v *LinkedWorktree) { v.Prunable = true },
		"empty collection state":       func(v *LinkedWorktree) { v.ObservationState = ObservationObservedEmpty },
		"unknown availability":         func(v *LinkedWorktree) { v.Availability = "other" },
	} {
		bad := valid
		mutate(&bad)
		if bad.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestSubmoduleObservation(t *testing.T) {
	declared := Submodule{ObservationState: ObservationObserved, Name: "lib", Path: NewObservedPath([]byte("vendor/lib")), RecordedCommitOID: testSHA1OID, DeclarationDigest: testSHA256, InitializationState: SubmoduleDeclaredUninitialized,
		URLRedacted: RedactedRemoteURL{DisplayRedacted: "https://example.test/lib.git", RedactionState: RedactionNotNeeded}}
	matching := declared
	matching.InitializationState, matching.ObservedPathIdentity, matching.ObservedHeadOID, matching.ObservedRepositoryKind = SubmoduleInitializedMatching, testSHA256, testSHA1OID, RepositoryKindSubmodule
	mismatched := matching
	mismatched.InitializationState, mismatched.ObservedHeadOID = SubmoduleInitializedMismatched, testOtherSHA1OID
	missing := declared
	missing.InitializationState, missing.URLRedacted = SubmoduleMissing, RedactedRemoteURL{}
	for name, value := range map[string]Submodule{"declared": declared, "matching": matching, "mismatched": mismatched, "missing": missing} {
		if err := value.Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	cases := map[string]Submodule{}
	add := func(name string, base Submodule, mutate func(*Submodule)) {
		mutate(&base)
		cases[name] = base
	}
	add("uninitialized with HEAD", declared, func(v *Submodule) { v.ObservedHeadOID = testSHA1OID })
	add("uninitialized with identity", declared, func(v *Submodule) { v.ObservedPathIdentity = testSHA256 })
	add("matching with different HEAD", matching, func(v *Submodule) { v.ObservedHeadOID = testOtherSHA1OID })
	add("matching without identity", matching, func(v *Submodule) { v.ObservedPathIdentity = "" })
	add("mismatched with equal HEAD", mismatched, func(v *Submodule) { v.ObservedHeadOID = testSHA1OID })
	add("mismatched without HEAD", mismatched, func(v *Submodule) { v.ObservedHeadOID = "" })
	add("missing name", declared, func(v *Submodule) { v.Name = "" })
	add("missing gitlink", declared, func(v *Submodule) { v.RecordedCommitOID = "" })
	add("missing declaration digest", declared, func(v *Submodule) { v.DeclarationDigest = "" })
	add("unredacted URL state", declared, func(v *Submodule) { v.URLRedacted.CredentialMaterialDetected = true })
	add("unknown repository kind", matching, func(v *Submodule) { v.ObservedRepositoryKind = "other" })
	add("unknown initialization", declared, func(v *Submodule) { v.InitializationState = "other" })
	for name, bad := range cases {
		if bad.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestFilesystemCapabilityProfile(t *testing.T) {
	in := validFilesystemProfileInput(FilesystemRoleProject)
	profile, err := NewFilesystemCapabilityProfile(in)
	if err != nil {
		t.Fatal(err)
	}
	in.MountFlags[0] = "changed"
	profile.MountFlags()[0] = "changed"
	if profile.MountFlags()[0] != "local" {
		t.Fatal("profile exposed or retained caller-owned mount flags")
	}
	unsupported := validFilesystemProfileInput(FilesystemRoleStateRoot)
	unsupported.FilesystemType, unsupported.Local, unsupported.ProductionSupported, unsupported.DirectorySync = "nfs", false, false, false
	if _, err := NewFilesystemCapabilityProfile(unsupported); err != nil {
		t.Fatalf("honest unsupported profile rejected: %v", err)
	}
	for name, mutate := range map[string]func(*FilesystemCapabilityProfileInput){
		"supported but remote":            func(in *FilesystemCapabilityProfileInput) { in.Local = false },
		"supported but read-only":         func(in *FilesystemCapabilityProfileInput) { in.ReadOnly = true },
		"supported without dir sync":      func(in *FilesystemCapabilityProfileInput) { in.DirectorySync = false },
		"supported without advisory lock": func(in *FilesystemCapabilityProfileInput) { in.AdvisoryLock = false },
		"unknown role":                    func(in *FilesystemCapabilityProfileInput) { in.Role = "other" },
		"missing filesystem type":         func(in *FilesystemCapabilityProfileInput) { in.FilesystemType = "" },
		"invalid device":                  func(in *FilesystemCapabilityProfileInput) { in.DeviceDecimal = "01" },
		"missing profile version":         func(in *FilesystemCapabilityProfileInput) { in.ProfileVersion = "" },
		"invalid probe digest":            func(in *FilesystemCapabilityProfileInput) { in.ProbeEvidenceDigest = "sha256:abc" },
	} {
		bad := validFilesystemProfileInput(FilesystemRoleProject)
		mutate(&bad)
		if _, err := NewFilesystemCapabilityProfile(bad); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestGitInspection(t *testing.T) {
	if _, err := NewGitInspection(validInspectionInput(t)); err != nil {
		t.Fatal(err)
	}
	remote, err := NewRemoteRedacted(RemoteRedactedInput{Name: "origin", FetchURLs: []RedactedRemoteURL{{DisplayRedacted: "https://example.test/r.git", RedactionState: RedactionNotNeeded}}, SourceScope: "local", SourceDigest: testSHA256, ObservationState: ObservationObserved})
	if err != nil {
		t.Fatal(err)
	}
	withRemote := validInspectionInput(t)
	withRemote.RemotesObservationState, withRemote.RemotesRedacted = ObservationObserved, []RemoteRedacted{remote}
	if _, err := NewGitInspection(withRemote); err != nil {
		t.Fatalf("inspection with remote rejected: %v", err)
	}

	unreliable := validObservationInput(t)
	unreliable.LinkedWorktreesObservationState = ObservationError
	unreliableObservation := mustObservation(t, unreliable)
	unprofiled := validObservationInput(t)
	unprofiled.FilesystemCapabilitiesObservationState, unprofiled.FilesystemCapabilities = ObservationError, nil
	unprofiledObservation := mustObservation(t, unprofiled)

	for name, mutate := range map[string]func(*GitInspectionInput){
		"empty list claims observed":   func(in *GitInspectionInput) { in.RemotesObservationState = ObservationObserved },
		"entries under observed_empty": func(in *GitInspectionInput) { in.RemotesRedacted = []RemoteRedacted{remote} },
		"duplicate remote": func(in *GitInspectionInput) {
			in.RemotesObservationState, in.RemotesRedacted = ObservationObserved, []RemoteRedacted{remote, remote}
		},
		"unexplained observation error":  func(in *GitInspectionInput) { in.Observation = unreliableObservation },
		"unexplained remote error":       func(in *GitInspectionInput) { in.RemotesObservationState = ObservationError },
		"unexplained missing filesystem": func(in *GitInspectionInput) { in.Observation, in.Warnings = unprofiledObservation, []string{"w"} },
		"unknown incompatibility reason": func(in *GitInspectionInput) { in.Incompatibilities = []Incompatibility{{Reason: "other"}} },
		"zero observation":               func(in *GitInspectionInput) { in.Observation = RepositoryObservation{} },
		"missing schema":                 func(in *GitInspectionInput) { in.SchemaVersion = "" },
		"missing inspection time":        func(in *GitInspectionInput) { in.InspectedAt = time.Time{} },
		"zero tool identity":             func(in *GitInspectionInput) { in.ToolIdentity = GitToolIdentity{} },
	} {
		bad := validInspectionInput(t)
		mutate(&bad)
		if _, err := NewGitInspection(bad); err == nil {
			t.Errorf("%s accepted", name)
		}
	}

	explained := validInspectionInput(t)
	explained.Observation, explained.Warnings = unreliableObservation, []string{"worktree listing failed"}
	if _, err := NewGitInspection(explained); err != nil {
		t.Fatalf("explained observation error rejected: %v", err)
	}
	incompatible := validInspectionInput(t)
	incompatible.Observation = unprofiledObservation
	incompatible.Incompatibilities = []Incompatibility{{Reason: ReasonFilesystemUnsupported, Subject: "project"}}
	inspection, err := NewGitInspection(incompatible)
	if err != nil {
		t.Fatalf("explicit filesystem incompatibility rejected: %v", err)
	}
	incompatible.Incompatibilities[0].Subject = "changed"
	inspection.Incompatibilities()[0].Reason = ReasonPlanStale
	if got := inspection.Incompatibilities()[0]; got.Subject != "project" || got.Reason != ReasonFilesystemUnsupported {
		t.Fatal("inspection retained or exposed caller-owned incompatibilities")
	}
}

func TestEnvironmentProfile(t *testing.T) {
	if err := validEnvironmentProfile().Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*EnvironmentProfile){
		"relative state root": func(v *EnvironmentProfile) { *v = environmentProfileFor("state/git") },
		"trailing slash root": func(v *EnvironmentProfile) { *v = environmentProfileFor(testStateRoot + "/") },
		"home outside root":   func(v *EnvironmentProfile) { v.Home = NewObservedPath([]byte("/home/u")) },
		"doubled git segment": func(v *EnvironmentProfile) {
			v.XDGConfigHome = NewObservedPath([]byte(testStateRoot + "/git/" + EnvironmentXDGLeaf))
		},
		"swapped leaves":  func(v *EnvironmentProfile) { v.Home, v.XDGConfigHome = v.XDGConfigHome, v.Home },
		"missing version": func(v *EnvironmentProfile) { v.Version = "" },
	} {
		bad := validEnvironmentProfile()
		mutate(&bad)
		if bad.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestDevelopmentBase(t *testing.T) {
	base := validDevelopmentBase()
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*DevelopmentBase){
		"base differs from tip":  func(v *DevelopmentBase) { v.BaseOID = testOtherSHA1OID },
		"OID of another format":  func(v *DevelopmentBase) { v.ObjectFormat = ObjectFormatSHA256 },
		"production ref not ref": func(v *DevelopmentBase) { v.ProductionRef = NewObservedRef([]byte("main")) },
		"missing production ref": func(v *DevelopmentBase) { v.ProductionRef = ObservedRef{} },
	} {
		bad := base
		mutate(&bad)
		if bad.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestImmutableIntentRules(t *testing.T) {
	if _, err := NewImmutableIntent(validIntentInput(t)); err != nil {
		t.Fatal(err)
	}

	// The initial commit is optional: without it there are no approvals,
	// identities, timestamps, or message bytes at all.
	noCommit := noCommitIntentInput(t)
	if _, err := NewImmutableIntent(noCommit); err != nil {
		t.Fatalf("intent without initial commit rejected: %v", err)
	}

	gitFlow := validIntentInput(t)
	gitFlow.Strategy, gitFlow.GitFlowDialect = StrategyGitFlow, GitFlowDialectAVH
	if _, err := NewImmutableIntent(gitFlow); err != nil {
		t.Fatalf("new Git Flow intent rejected: %v", err)
	}

	development := developmentIntentInput(t)
	if _, err := NewImmutableIntent(development); err != nil {
		t.Fatalf("existing-history development intent rejected: %v", err)
	}

	for name, mutate := range map[string]func(*ImmutableIntentInput){
		"unknown strategy":              func(in *ImmutableIntentInput) { in.Strategy = "trunk" },
		"dialect on non-Git-Flow":       func(in *ImmutableIntentInput) { in.GitFlowDialect = GitFlowDialectAVH },
		"new Git Flow without dialect":  func(in *ImmutableIntentInput) { in.Strategy = StrategyGitFlow },
		"new repository with sha256":    func(in *ImmutableIntentInput) { in.ObjectFormat = ObjectFormatSHA256 },
		"new repository with reftable":  func(in *ImmutableIntentInput) { in.RefFormat = RefFormatReftable },
		"new repository without branch": func(in *ImmutableIntentInput) { in.BranchParameters = nil },
		"missing object format":         func(in *ImmutableIntentInput) { in.ObjectFormat = "" },
		"invalid project ID":            func(in *ImmutableIntentInput) { in.ProjectID = "not-a-uuid" },
		"missing capabilities":          func(in *ImmutableIntentInput) { in.RequiredCapabilities = nil },
		"commit without files":          func(in *ImmutableIntentInput) { in.AuthorizedFiles = nil },
		"commit without project config": func(in *ImmutableIntentInput) {
			in.AuthorizedFiles = []AuthorizedFile{validAuthorizedFile("README.md")}
		},
		"project config executable":  func(in *ImmutableIntentInput) { in.AuthorizedFiles[0].Mode = "100755" },
		"duplicate authorized path":  func(in *ImmutableIntentInput) { in.AuthorizedFiles = append(in.AuthorizedFiles, in.AuthorizedFiles[0]) },
		"blob OID of another format": func(in *ImmutableIntentInput) { in.AuthorizedFiles[0].BlobOID = testSHA256OID },
		"commit without message":     func(in *ImmutableIntentInput) { in.CommitMessageBytes = nil },
		"commit without author":      func(in *ImmutableIntentInput) { in.Author = GitIdentity{} },
		"commit without timestamp":   func(in *ImmutableIntentInput) { in.CommitterTimestamp = time.Time{} },
		"files without commit":       func(in *ImmutableIntentInput) { in.InitialCommit = false },
		"development base on new repo": func(in *ImmutableIntentInput) {
			in.Strategy, in.GitFlowDialect, in.DevelopmentBase = StrategyGitFlow, GitFlowDialectAVH, validDevelopmentBase()
		},
	} {
		bad := validIntentInput(t)
		mutate(&bad)
		if _, err := NewImmutableIntent(bad); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	for name, mutate := range map[string]func(*ImmutableIntentInput){
		"development base on basic":    func(in *ImmutableIntentInput) { in.Strategy = StrategyBasic },
		"development base unequal tip": func(in *ImmutableIntentInput) { in.DevelopmentBase.BaseOID = testOtherSHA1OID },
		"development base other format": func(in *ImmutableIntentInput) {
			in.DevelopmentBase.ObjectFormat, in.DevelopmentBase.BaseOID, in.DevelopmentBase.ProductionTipOID = ObjectFormatSHA256, testSHA256OID, testSHA256OID
		},
	} {
		bad := developmentIntentInput(t)
		mutate(&bad)
		if _, err := NewImmutableIntent(bad); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestImmutableIntentDefensiveCopies(t *testing.T) {
	in := validIntentInput(t)
	intent, err := NewImmutableIntent(in)
	if err != nil {
		t.Fatal(err)
	}
	in.CommitMessageBytes[0] = 'X'
	in.BranchParameters["initial"] = "changed"
	in.AuthorizedFiles[0].Mode = "100755"
	copyOf := intent
	copyOf.BranchParameters()["initial"] = "changed"
	copyOf.CommitMessageBytes()[0] = 'X'
	copyOf.AuthorizedFiles()[0].Mode = "100755"
	if string(intent.CommitMessageBytes()) != "chore: initial commit\n" || intent.BranchParameters()["initial"] != "main" ||
		intent.AuthorizedFiles()[0].Mode != "100644" {
		t.Fatal("intent retained or exposed caller-owned collections")
	}
	if err := intent.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestPreconditionSnapshot(t *testing.T) {
	if err := validSnapshot(t).Validate(); err != nil {
		t.Fatal(err)
	}
	withBase := validSnapshot(t)
	withBase.DevelopmentBase = validDevelopmentBase()
	if err := withBase.Validate(); err != nil {
		t.Fatalf("snapshot with development base rejected: %v", err)
	}

	// A snapshot accepts only evidence planning can rely on, even when the
	// inspection that produced it explained the gap.
	for name, mutate := range map[string]func(*RepositoryObservationInput){
		"refs not observed": func(in *RepositoryObservationInput) { in.RefsObservationState = ObservationNotObserved },
		"worktree error":    func(in *RepositoryObservationInput) { in.WorktreeObservationState = ObservationError },
		"unreadable index": func(in *RepositoryObservationInput) {
			in.Index = mustIndex(t, IndexObservationInput{Presence: IndexUnreadable, LockState: LockFileAbsent})
		},
		"unobservable config lock": func(in *RepositoryObservationInput) { in.ConfigLockState = LockFileUnobservable },
		"unobservable index lock": func(in *RepositoryObservationInput) {
			in.Index = mustIndex(t, IndexObservationInput{Presence: IndexAbsent, LockState: LockFileUnobservable})
		},
		"unreadable project config": func(in *RepositoryObservationInput) {
			in.ProjectConfig = ProjectConfigObservation{State: ProjectConfigUnreadable, Path: in.ProjectConfig.Path}
		},
		"missing filesystem profiles": func(in *RepositoryObservationInput) {
			in.FilesystemCapabilitiesObservationState, in.FilesystemCapabilities = ObservationObservedEmpty, nil
		},
	} {
		observationInput := validObservationInput(t)
		mutate(&observationInput)
		bad := validSnapshot(t)
		bad.Observation = mustObservation(t, observationInput)
		if bad.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
	for name, mutate := range map[string]func(*PreconditionSnapshot){
		"zero environment profile": func(v *PreconditionSnapshot) { v.EnvironmentProfile = EnvironmentProfile{} },
		"zero project identity":    func(v *PreconditionSnapshot) { v.ProjectIdentity = PhysicalProjectIdentity{} },
		"zero tool identity":       func(v *PreconditionSnapshot) { v.ToolIdentity = GitToolIdentity{} },
		"invalid development base": func(v *PreconditionSnapshot) { v.DevelopmentBase = DevelopmentBase{BaseOID: testSHA1OID} },
		"development base of other format": func(v *PreconditionSnapshot) {
			v.DevelopmentBase = DevelopmentBase{BaseOID: testSHA256OID, ObjectFormat: ObjectFormatSHA256, ProductionRef: NewObservedRef([]byte("refs/heads/main")), ProductionTipOID: testSHA256OID}
		},
	} {
		bad := validSnapshot(t)
		mutate(&bad)
		if bad.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestConditionCatalogue(t *testing.T) {
	path := EncodeReversiblePath([]byte(".ai-dev/config.json"))
	ref := EncodeReversiblePath([]byte("refs/heads/main"))
	valid := []Condition{
		{Kind: ConditionRepositoryAbsent},
		{Kind: ConditionRepositoryKind, Expected: "normal"},
		{Kind: ConditionHeadState, Expected: "unborn"},
		{Kind: ConditionHeadSymbolicRef, Subject: ref},
		{Kind: ConditionRefAbsent, Subject: ref},
		{Kind: ConditionRefTarget, Subject: ref, Expected: testSHA1OID},
		{Kind: ConditionReflogAbsent, Subject: ref},
		{Kind: ConditionObjectType, Subject: testSHA1OID, Expected: "commit"},
		{Kind: ConditionPathAbsent, Subject: path},
		{Kind: ConditionPathIdentity, Subject: path, Expected: "1:9007199254740993"},
		{Kind: ConditionPathContent, Subject: path, Expected: testSHA256},
		{Kind: ConditionPathMode, Subject: path, Expected: "100644"},
		{Kind: ConditionIndexAbsent},
		{Kind: ConditionIndexEmpty},
		{Kind: ConditionIndexTree, Expected: testSHA1OID},
		{Kind: ConditionLockFileAbsent},
		{Kind: ConditionConfigAbsent, Subject: "gitflow.branch.master"},
		{Kind: ConditionConfigValue, Subject: "gitflow.branch.master", Expected: "main"},
		{Kind: ConditionPathStatusClean, Subject: path},
	}
	covered := map[ConditionKind]bool{}
	for _, condition := range valid {
		if err := condition.Validate(); err != nil {
			t.Errorf("%s: %v", condition.Kind, err)
		}
		covered[condition.Kind] = true
	}
	if len(covered) != len(conditionShapes) {
		t.Fatalf("valid cases cover %d of %d condition kinds", len(covered), len(conditionShapes))
	}
	for name, bad := range map[string]Condition{
		"unknown kind":                {Kind: "other"},
		"subject where none allowed":  {Kind: ConditionIndexEmpty, Subject: path},
		"expected where none allowed": {Kind: ConditionRefAbsent, Subject: ref, Expected: testSHA1OID},
		"missing path subject":        {Kind: ConditionPathAbsent},
		"padded path subject":         {Kind: ConditionPathAbsent, Subject: path + "="},
		"ref subject outside refs":    {Kind: ConditionRefAbsent, Subject: EncodeReversiblePath([]byte("HEAD"))},
		"invalid OID subject":         {Kind: ConditionObjectType, Subject: "zz", Expected: "commit"},
		"unknown object type":         {Kind: ConditionObjectType, Subject: testSHA1OID, Expected: "tag"},
		"unknown repository kind":     {Kind: ConditionRepositoryKind, Expected: "other"},
		"unknown HEAD state":          {Kind: ConditionHeadState, Expected: "other"},
		"identity without separator":  {Kind: ConditionPathIdentity, Subject: path, Expected: "12"},
		"identity with leading zero":  {Kind: ConditionPathIdentity, Subject: path, Expected: "01:2"},
		"non-octal mode":              {Kind: ConditionPathMode, Subject: path, Expected: "100689"},
		"invalid digest":              {Kind: ConditionPathContent, Subject: path, Expected: "sha256:abc"},
		"invalid config key":          {Kind: ConditionConfigAbsent, Subject: "Core.Bare"},
		"empty config value":          {Kind: ConditionConfigValue, Subject: "core.bare"},
	} {
		if bad.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestDerivedOperationAndPlan(t *testing.T) {
	in := validPlanInput(t)
	plan, err := NewGitInitializationPlan(in)
	if err != nil {
		t.Fatal(err)
	}
	in.DerivedOperations[0] = DerivedOperation{}
	in.ProhibitedOperations[0] = "changed"
	operations := plan.DerivedOperations()
	operations[0].Kind = OperationKindVerification
	operations[0].ExactArgv()[0] = "changed"
	if got := plan.DerivedOperations()[0]; got.Kind != OperationKindRepositoryInit || got.ExactArgv()[0] != "init" || plan.ProhibitedOperations()[0] != "push" {
		t.Fatal("plan retained or exposed caller-owned collections")
	}

	verification := mustOperation(t, verificationOperationInput(1))
	for name, mutate := range map[string]func(*DerivedOperationInput){
		"ordinal mismatch":         func(in *DerivedOperationInput) { in.Ordinal = 1 },
		"unknown kind":             func(in *DerivedOperationInput) { in.Kind = "other" },
		"missing target":           func(in *DerivedOperationInput) { in.TargetIdentity = "" },
		"missing preconditions":    func(in *DerivedOperationInput) { in.Preconditions = nil },
		"mutation without effects": func(in *DerivedOperationInput) { in.ExpectedEffects = nil },
		"mutation without failure": func(in *DerivedOperationInput) { in.FailureOutcomes = nil },
		"mutation classified none": func(in *DerivedOperationInput) {
			in.PartialEffectClassification, in.RecoveryInstruction = PartialEffectNone, RecoveryNotApplicable
		},
		"effects without recovery": func(in *DerivedOperationInput) { in.RecoveryInstruction = RecoveryNotApplicable },
		"argv without profile":     func(in *DerivedOperationInput) { in.EnvironmentProfile = "" },
		"profile without argv":     func(in *DerivedOperationInput) { in.ExactArgv = nil },
		"NUL in argv":              func(in *DerivedOperationInput) { in.ExactArgv = []string{"init", "a\x00b"} },
		"unknown failure outcome":  func(in *DerivedOperationInput) { in.FailureOutcomes = []Reason{"other"} },
		"invalid precondition":     func(in *DerivedOperationInput) { in.Preconditions = []Condition{{Kind: "other"}} },
		"invalid postcondition": func(in *DerivedOperationInput) {
			in.ExpectedPostconditions = []Condition{{Kind: ConditionHeadState, Expected: "x"}}
		},
		"invalid affected ref":        func(in *DerivedOperationInput) { in.AffectedRefs = []ObservedRef{NewObservedRef([]byte("main"))} },
		"invalid affected config key": func(in *DerivedOperationInput) { in.AffectedConfigKeys = []string{"Core"} },
		"unknown temporary role":      func(in *DerivedOperationInput) { in.TemporaryResourceRoles = []ResourceRole{"other"} },
		"invalid affected path":       func(in *DerivedOperationInput) { in.AffectedPaths = []ObservedPath{{}} },
		"invalid stdin digest":        func(in *DerivedOperationInput) { in.StdinDigest = "sha256:abc" },
		"stdin without Git invocation": func(in *DerivedOperationInput) {
			in.StdinDigest, in.ExactArgv, in.EnvironmentProfile = testSHA256, nil, ""
		},
		"unknown partial classification": func(in *DerivedOperationInput) { in.PartialEffectClassification = "other" },
	} {
		bad := validOperationInput(0)
		mutate(&bad)
		if _, err := NewDerivedOperation(bad); err == nil {
			t.Errorf("operation: %s accepted", name)
		}
	}
	effectfulVerification := verificationOperationInput(1)
	effectfulVerification.ExpectedEffects = []Condition{{Kind: ConditionIndexEmpty}}
	if _, err := NewDerivedOperation(effectfulVerification); err == nil {
		t.Error("operation: verification with effects accepted")
	}

	withVerification := validPlanInput(t)
	withVerification.DerivedOperations = append(withVerification.DerivedOperations, verification)
	sealPlanInput(t, &withVerification)
	if _, err := NewGitInitializationPlan(withVerification); err != nil {
		t.Fatalf("plan with trailing verification rejected: %v", err)
	}

	second, err := NewDerivedOperation(validOperationInput(1))
	if err != nil {
		t.Fatal(err)
	}
	expires := in.GeneratedAt
	for name, mutate := range map[string]func(*GitInitializationPlanInput){
		"unknown schema": func(in *GitInitializationPlanInput) { in.SchemaVersion = "ai-dev.git-init-plan/v2" },
		"no operations":  func(in *GitInitializationPlanInput) { in.DerivedOperations, in.PartialEffectPoints = nil, nil },
		"operation out of order": func(in *GitInitializationPlanInput) {
			in.DerivedOperations, in.PartialEffectPoints = []DerivedOperation{second}, []OperationID{second.OperationID}
		},
		"no postconditions":     func(in *GitInitializationPlanInput) { in.ExpectedPostconditions = nil },
		"incomplete prohibited": func(in *GitInitializationPlanInput) { in.ProhibitedOperations = in.ProhibitedOperations[1:] },
		"reordered prohibited": func(in *GitInitializationPlanInput) {
			in.ProhibitedOperations[0], in.ProhibitedOperations[1] = in.ProhibitedOperations[1], in.ProhibitedOperations[0]
		},
		"missing partial-effect point": func(in *GitInitializationPlanInput) { in.PartialEffectPoints = nil },
		"extra partial-effect point": func(in *GitInitializationPlanInput) {
			in.PartialEffectPoints = append(in.PartialEffectPoints, operationID(1))
		},
		"unknown recovery policy": func(in *GitInitializationPlanInput) { in.RecoveryPolicy = "retry" },
		"missing generation time": func(in *GitInitializationPlanInput) { in.GeneratedAt = time.Time{} },
		"missing inspection time": func(in *GitInitializationPlanInput) { in.InspectedAt = time.Time{} },
		"non-later expiry":        func(in *GitInitializationPlanInput) { in.ExpiresAt = expires },
		"invalid digest":          func(in *GitInitializationPlanInput) { in.PlanDigest = "sha256:abc" },
		"zero intent":             func(in *GitInitializationPlanInput) { in.ImmutableIntent = ImmutableIntent{} },
		"zero snapshot":           func(in *GitInitializationPlanInput) { in.PreconditionSnapshot = PreconditionSnapshot{} },
		"development base only in snapshot": func(in *GitInitializationPlanInput) {
			in.PreconditionSnapshot.DevelopmentBase = validDevelopmentBase()
		},
	} {
		bad := validPlanInput(t)
		mutate(&bad)
		if _, err := NewGitInitializationPlan(bad); err == nil {
			t.Errorf("plan: %s accepted", name)
		}
	}
}

func TestAuthorizationLifecycle(t *testing.T) {
	claim := AuthorizationTransition{From: AuthorizationStateIssued, To: AuthorizationStateClaimed, StateVersion: 2, OwnerApplyID: testApplyID}
	start := AuthorizationTransition{From: AuthorizationStateClaimed, To: AuthorizationStateMutationStarted, StateVersion: 3, OwnerApplyID: testApplyID}
	terminal := func(state AuthorizationState) []AuthorizationTransition {
		return []AuthorizationTransition{claim, start, {From: AuthorizationStateMutationStarted, To: state, StateVersion: 4, OwnerApplyID: testApplyID}}
	}
	states := []struct {
		name    string
		state   AuthorizationState
		version uint64
		owner   ApplyID
		digest  ContentDigest
		history []AuthorizationTransition
	}{
		{"issued", AuthorizationStateIssued, 1, "", "", nil},
		{"claimed", AuthorizationStateClaimed, 2, testApplyID, "", []AuthorizationTransition{claim}},
		{"mutation started", AuthorizationStateMutationStarted, 3, testApplyID, "", []AuthorizationTransition{claim, start}},
		{"applied", AuthorizationStateApplied, 4, testApplyID, testSHA256, terminal(AuthorizationStateApplied)},
		{"failed", AuthorizationStateFailed, 4, testApplyID, testSHA256, terminal(AuthorizationStateFailed)},
		{"partially applied", AuthorizationStatePartiallyApplied, 4, testApplyID, testSHA256, terminal(AuthorizationStatePartiallyApplied)},
		{"indeterminate", AuthorizationStateIndeterminate, 4, testApplyID, testSHA256, terminal(AuthorizationStateIndeterminate)},
		// A clean pre-mutation release returns to issued and leaves no owner.
		{"released to issued", AuthorizationStateIssued, 3, "", "", []AuthorizationTransition{claim, {From: AuthorizationStateClaimed, To: AuthorizationStateIssued, StateVersion: 3, OwnerApplyID: testApplyID}}},
		{"reclaimed after release", AuthorizationStateClaimed, 4, testOtherApplyID, "", []AuthorizationTransition{claim, {From: AuthorizationStateClaimed, To: AuthorizationStateIssued, StateVersion: 3, OwnerApplyID: testApplyID}, {From: AuthorizationStateIssued, To: AuthorizationStateClaimed, StateVersion: 4, OwnerApplyID: testOtherApplyID}}},
		{"orphan reassignment", AuthorizationStateClaimed, 3, testOtherApplyID, "", []AuthorizationTransition{claim, {From: AuthorizationStateClaimed, To: AuthorizationStateClaimed, StateVersion: 3, OwnerApplyID: testOtherApplyID}}},
	}
	for _, tc := range states {
		in := validAuthorizationInput(t)
		in.State, in.StateVersion, in.OwnerApplyID, in.TerminalResultDigest, in.TransitionHistory = tc.state, tc.version, tc.owner, tc.digest, tc.history
		if _, err := NewGitAuthorization(in); err != nil {
			t.Errorf("%s: %v", tc.name, err)
		}
	}

	equal := validAuthorizationInput(t).IssuedAt
	for name, mutate := range map[string]func(*GitAuthorizationInput){
		"missing issuance":         func(in *GitAuthorizationInput) { in.IssuedAt = time.Time{} },
		"non-later expiry":         func(in *GitAuthorizationInput) { in.ExpiresAt = equal },
		"unknown strategy":         func(in *GitAuthorizationInput) { in.Strategy = "legacy" },
		"dialect on non-Git-Flow":  func(in *GitAuthorizationInput) { in.GitFlowDialect = GitFlowDialectAVH },
		"missing operator":         func(in *GitAuthorizationInput) { in.DeclaredOperator = "" },
		"no operations":            func(in *GitAuthorizationInput) { in.AuthorizedOperationIDs = nil },
		"invalid authorized ref":   func(in *GitAuthorizationInput) { in.AuthorizedRefs = []ObservedRef{NewObservedRef([]byte("main"))} },
		"invalid config key":       func(in *GitAuthorizationInput) { in.AuthorizedConfigKeys = []string{"GitFlow.Branch"} },
		"commit without approvals": func(in *GitAuthorizationInput) { in.InitialCommitAuthorized = true },
		"approvals without commit": func(in *GitAuthorizationInput) {
			in.ApprovedFiles = []AuthorizedFile{validAuthorizedFile(ProjectConfigPath)}
		},
		"issued with owner":  func(in *GitAuthorizationInput) { in.OwnerApplyID = testApplyID },
		"issued with digest": func(in *GitAuthorizationInput) { in.TerminalResultDigest = testSHA256 },
		"claimed without history": func(in *GitAuthorizationInput) {
			in.State, in.StateVersion, in.OwnerApplyID = AuthorizationStateClaimed, 2, testApplyID
		},
		"partial previous linkage": func(in *GitAuthorizationInput) { in.PreviousApplyID = testApplyID },
		"self linkage": func(in *GitAuthorizationInput) {
			in.PreviousAuthorizationID, in.PreviousApplyID, in.PreviousPlanDigest = in.AuthorizationID, testApplyID, testSHA256
		},
		"direct terminal transition": func(in *GitAuthorizationInput) {
			in.State, in.StateVersion, in.OwnerApplyID, in.TerminalResultDigest = AuthorizationStateApplied, 2, testApplyID, testSHA256
			in.TransitionHistory = []AuthorizationTransition{{From: AuthorizationStateIssued, To: AuthorizationStateApplied, StateVersion: 2, OwnerApplyID: testApplyID}}
		},
		"terminal without digest": func(in *GitAuthorizationInput) {
			in.State, in.StateVersion, in.OwnerApplyID, in.TransitionHistory = AuthorizationStateApplied, 4, testApplyID, terminal(AuthorizationStateApplied)
		},
		"owner changes without reassignment": func(in *GitAuthorizationInput) {
			in.State, in.StateVersion, in.OwnerApplyID = AuthorizationStateMutationStarted, 3, testOtherApplyID
			in.TransitionHistory = []AuthorizationTransition{claim, {From: AuthorizationStateClaimed, To: AuthorizationStateMutationStarted, StateVersion: 3, OwnerApplyID: testOtherApplyID}}
		},
		"reassignment to same owner": func(in *GitAuthorizationInput) {
			in.State, in.StateVersion, in.OwnerApplyID = AuthorizationStateClaimed, 3, testApplyID
			in.TransitionHistory = []AuthorizationTransition{claim, {From: AuthorizationStateClaimed, To: AuthorizationStateClaimed, StateVersion: 3, OwnerApplyID: testApplyID}}
		},
		"owner differs from history": func(in *GitAuthorizationInput) {
			in.State, in.StateVersion, in.OwnerApplyID, in.TransitionHistory = AuthorizationStateClaimed, 2, testOtherApplyID, []AuthorizationTransition{claim}
		},
		"version skips history": func(in *GitAuthorizationInput) {
			in.State, in.StateVersion, in.OwnerApplyID, in.TransitionHistory = AuthorizationStateClaimed, 5, testApplyID, []AuthorizationTransition{claim}
		},
	} {
		bad := validAuthorizationInput(t)
		mutate(&bad)
		if _, err := NewGitAuthorization(bad); err == nil {
			t.Errorf("%s accepted", name)
		}
	}

	// A plan without an initial commit or Git configuration authorizes no
	// ref and no configuration key.
	unscoped := validAuthorizationInput(t)
	unscoped.AuthorizedRefs, unscoped.AuthorizedConfigKeys = nil, nil
	if _, err := NewGitAuthorization(unscoped); err != nil {
		t.Fatalf("authorization without ref or config scope rejected: %v", err)
	}
	linked := validAuthorizationInput(t)
	linked.PreviousAuthorizationID, linked.PreviousApplyID, linked.PreviousPlanDigest = "123e4567-e89b-42d3-a456-426614174007", testApplyID, testOtherSHA256
	if _, err := NewGitAuthorization(linked); err != nil {
		t.Fatalf("reconciliation linkage rejected: %v", err)
	}
	withCommit := validAuthorizationInput(t)
	withCommit.InitialCommitAuthorized, withCommit.ApprovedFiles = true, []AuthorizedFile{validAuthorizedFile(ProjectConfigPath)}
	authorization, err := NewGitAuthorization(withCommit)
	if err != nil {
		t.Fatalf("authorization with approvals rejected: %v", err)
	}
	withCommit.AuthorizedRefs[0] = NewObservedRef([]byte("refs/heads/other"))
	authorization.AuthorizedConfigKeys()[0] = "changed"
	if authorization.AuthorizedRefs()[0].DisplayUTF8 != "refs/heads/main" || authorization.AuthorizedConfigKeys()[0] != "gitflow.branch.master" {
		t.Fatal("authorization retained or exposed caller-owned scope")
	}
}

func TestAuthorizationTransition(t *testing.T) {
	issued, err := NewGitAuthorization(validAuthorizationInput(t))
	if err != nil {
		t.Fatal(err)
	}
	request := func(from AuthorizationState, version uint64, expectedOwner ApplyID, to AuthorizationState, owner ApplyID) AuthorizationTransitionRequest {
		return AuthorizationTransitionRequest{ProjectIdentityDigest: issued.ProjectIdentity.Digest(), AuthorizationID: issued.AuthorizationID, ExpectedState: from, ExpectedVersion: version, ExpectedOwner: expectedOwner, To: to, Owner: owner}
	}
	claimed, err := issued.Transition(request(AuthorizationStateIssued, 1, "", AuthorizationStateClaimed, testApplyID))
	if err != nil || claimed.State != AuthorizationStateClaimed || claimed.StateVersion != 2 || claimed.OwnerApplyID != testApplyID {
		t.Fatalf("claim = %+v, %v", claimed, err)
	}
	if issued.State != AuthorizationStateIssued || len(issued.TransitionHistory()) != 0 {
		t.Fatal("Transition mutated its receiver")
	}

	released, err := claimed.Transition(request(AuthorizationStateClaimed, 2, testApplyID, AuthorizationStateIssued, testApplyID))
	if err != nil || released.State != AuthorizationStateIssued || released.OwnerApplyID != "" || released.StateVersion != 3 {
		t.Fatalf("clean release = %+v, %v", released, err)
	}
	if _, err := released.Transition(request(AuthorizationStateIssued, 3, "", AuthorizationStateClaimed, testOtherApplyID)); err != nil {
		t.Fatalf("claim after clean release: %v", err)
	}

	started, err := claimed.Transition(request(AuthorizationStateClaimed, 2, testApplyID, AuthorizationStateMutationStarted, testApplyID))
	if err != nil {
		t.Fatal(err)
	}
	closing := request(AuthorizationStateMutationStarted, 3, testApplyID, AuthorizationStatePartiallyApplied, testApplyID)
	closing.TerminalResultDigest = testSHA256
	closed, err := started.Transition(closing)
	if err != nil || closed.TerminalResultDigest != testSHA256 || len(closed.TransitionHistory()) != 3 {
		t.Fatalf("terminal close = %+v, %v", closed, err)
	}

	conflicts := map[string]AuthorizationTransitionRequest{
		"stale version":  request(AuthorizationStateClaimed, 1, testApplyID, AuthorizationStateMutationStarted, testApplyID),
		"stale state":    request(AuthorizationStateIssued, 2, "", AuthorizationStateClaimed, testOtherApplyID),
		"foreign owner":  request(AuthorizationStateClaimed, 2, testOtherApplyID, AuthorizationStateMutationStarted, testOtherApplyID),
		"other project":  {ProjectIdentityDigest: testSHA256, AuthorizationID: issued.AuthorizationID, ExpectedState: AuthorizationStateClaimed, ExpectedVersion: 2, ExpectedOwner: testApplyID, To: AuthorizationStateMutationStarted, Owner: testApplyID},
		"other identity": {ProjectIdentityDigest: issued.ProjectIdentity.Digest(), AuthorizationID: "123e4567-e89b-42d3-a456-426614174008", ExpectedState: AuthorizationStateClaimed, ExpectedVersion: 2, ExpectedOwner: testApplyID, To: AuthorizationStateMutationStarted, Owner: testApplyID},
	}
	for name, req := range conflicts {
		if _, err := claimed.Transition(req); !errors.Is(err, ErrAuthorizationConflict) {
			t.Errorf("%s: err=%v, want compare-and-swap conflict", name, err)
		}
	}
	terminalAgain := request(AuthorizationStatePartiallyApplied, 4, testApplyID, AuthorizationStateApplied, testApplyID)
	terminalAgain.TerminalResultDigest = testSHA256
	if _, err := closed.Transition(terminalAgain); err == nil || errors.Is(err, ErrAuthorizationConflict) {
		t.Errorf("terminal state transitioned or misreported: %v", err)
	}

	digestBeforeTerminal := request(AuthorizationStateIssued, 1, "", AuthorizationStateClaimed, testApplyID)
	digestBeforeTerminal.TerminalResultDigest = testSHA256
	invalidRequests := map[string]AuthorizationTransitionRequest{
		"issued to mutation_started":     request(AuthorizationStateIssued, 1, "", AuthorizationStateMutationStarted, testApplyID),
		"issued with expected owner":     request(AuthorizationStateIssued, 1, testApplyID, AuthorizationStateClaimed, testApplyID),
		"claimed without expected owner": request(AuthorizationStateClaimed, 2, "", AuthorizationStateMutationStarted, testApplyID),
		"owner transfer on start":        request(AuthorizationStateClaimed, 2, testApplyID, AuthorizationStateMutationStarted, testOtherApplyID),
		"reassignment to same owner":     request(AuthorizationStateClaimed, 2, testApplyID, AuthorizationStateClaimed, testApplyID),
		"terminal without digest":        request(AuthorizationStateMutationStarted, 3, testApplyID, AuthorizationStateApplied, testApplyID),
		"zero version":                   request(AuthorizationStateIssued, 0, "", AuthorizationStateClaimed, testApplyID),
		"missing owner":                  request(AuthorizationStateIssued, 1, "", AuthorizationStateClaimed, ""),
		"digest before terminal":         digestBeforeTerminal,
	}
	for name, req := range invalidRequests {
		if req.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if err := request(AuthorizationStateClaimed, 2, testApplyID, AuthorizationStateClaimed, testOtherApplyID).Validate(); err != nil {
		t.Errorf("orphan reassignment request rejected: %v", err)
	}
}

func TestAuditEventValidation(t *testing.T) {
	digest := mustProjectIdentity(t).Digest()
	valid := AuditEvent{Kind: AuditEventOperationStarted, ProjectIdentityDigest: digest, ApplyID: testApplyID, Sequence: 3, AuthorizationID: testAuthorizationID, OperationID: operationID(0), Record: []byte("{}")}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	attempt := AuditEvent{Kind: AuditEventAttemptCreated, ProjectIdentityDigest: digest, ApplyID: testApplyID, Record: []byte("{}")}
	if err := attempt.Validate(); err != nil {
		t.Fatalf("attempt event without authorization rejected: %v", err)
	}
	for name, mutate := range map[string]func(*AuditEvent){
		"unknown kind":                 func(v *AuditEvent) { v.Kind = "other" },
		"empty record":                 func(v *AuditEvent) { v.Record = nil },
		"operation event without ID":   func(v *AuditEvent) { v.OperationID = "" },
		"operation ID on attempt kind": func(v *AuditEvent) { v.Kind = AuditEventMutationIntent },
		"invalid apply ID":             func(v *AuditEvent) { v.ApplyID = "x" },
		"invalid project digest":       func(v *AuditEvent) { v.ProjectIdentityDigest = "x" },
		"invalid authorization ID":     func(v *AuditEvent) { v.AuthorizationID = "x" },
	} {
		bad := valid
		mutate(&bad)
		if bad.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestApplyAttemptValidation(t *testing.T) {
	valid := GitApplyAttempt{SchemaVersion: "ai-dev.git-apply-attempt/v1", ApplyID: testApplyID, StartedAt: time.Unix(2, 0).UTC(), InvocationMode: InvocationModeDirect}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	full := valid
	full.AuthorizationID, full.DeclaredPlanDigest, full.ProjectIdentity = testAuthorizationID, testSHA256, mustProjectIdentity(t)
	full.ResultStatus, full.ResultReason = ApplyStatusRefused, ReasonPlanStale
	if err := full.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*GitApplyAttempt){
		"reason without status":    func(v *GitApplyAttempt) { v.ResultStatus = "" },
		"unknown status":           func(v *GitApplyAttempt) { v.ResultStatus = "other" },
		"unknown reason":           func(v *GitApplyAttempt) { v.ResultReason = "other" },
		"invalid plan digest":      func(v *GitApplyAttempt) { v.DeclaredPlanDigest = "x" },
		"invalid authorization ID": func(v *GitApplyAttempt) { v.AuthorizationID = "x" },
		"missing start":            func(v *GitApplyAttempt) { v.StartedAt = time.Time{} },
		"unknown invocation mode":  func(v *GitApplyAttempt) { v.InvocationMode = "other" },
	} {
		bad := full
		mutate(&bad)
		if bad.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestApplyResultStatusReasonExitMapping(t *testing.T) {
	tests := []struct {
		name    string
		status  ApplyStatus
		reason  Reason
		durable AuthorizationState
		exit    int
		second  []SecondaryFailure
		valid   bool
	}{
		{"applied", ApplyStatusApplied, "", AuthorizationStateApplied, 0, nil, true},
		{"applied informational", ApplyStatusApplied, ReasonDevelopmentBranchPendingInitialHistory, AuthorizationStateApplied, 0, nil, true},
		{"already applied", ApplyStatusAlreadyApplied, "", AuthorizationStateApplied, 0, nil, true},
		{"refused domain", ApplyStatusRefused, ReasonRepositoryStateIncompatible, AuthorizationStateIssued, 1, nil, true},
		{"refused usage", ApplyStatusRefused, ReasonInvalidStrategy, AuthorizationStateIssued, 2, nil, true},
		{"refused consumed", ApplyStatusRefused, ReasonAuthorizationConsumed, AuthorizationStateMutationStarted, 1, nil, true},
		{"failed", ApplyStatusFailed, ReasonOperationFailed, AuthorizationStateFailed, 1, nil, true},
		{"partial", ApplyStatusPartiallyApplied, ReasonOperationFailed, AuthorizationStatePartiallyApplied, 1, nil, true},
		{"indeterminate", ApplyStatusIndeterminate, ReasonAuditFailedAfterMutation, AuthorizationStateIndeterminate, 1, nil, true},
		{"indeterminate last verified claimed", ApplyStatusIndeterminate, ReasonAuditFailedAfterMutation, AuthorizationStateClaimed, 1, nil, true},
		{"output failed", ApplyStatusApplied, ReasonOutputFailed, AuthorizationStateApplied, 2, nil, true},
		{"secondary forces one", ApplyStatusApplied, ReasonResourceReleaseFailed, AuthorizationStateApplied, 1, []SecondaryFailure{validSecondaryFailure()}, true},
		{"secondary keeps primary", ApplyStatusPartiallyApplied, ReasonOperationFailed, AuthorizationStatePartiallyApplied, 1, []SecondaryFailure{validSecondaryFailure()}, true},
		{"output failed cannot invent refusal", ApplyStatusRefused, ReasonOutputFailed, AuthorizationStateIssued, 2, nil, false},
		{"audit failure cannot be refusal", ApplyStatusRefused, ReasonAuditFailedAfterMutation, AuthorizationStateIssued, 1, nil, false},
		{"informational reason cannot fail", ApplyStatusFailed, ReasonDevelopmentBranchPendingInitialHistory, AuthorizationStateFailed, 1, nil, false},
		{"refusal reason cannot be indeterminate", ApplyStatusIndeterminate, ReasonPlanStale, AuthorizationStateIndeterminate, 1, nil, false},
		{"release reason without secondary", ApplyStatusApplied, ReasonResourceReleaseFailed, AuthorizationStateApplied, 1, nil, false},
		{"secondary wrong exit", ApplyStatusApplied, ReasonResourceReleaseFailed, AuthorizationStateApplied, 0, []SecondaryFailure{validSecondaryFailure()}, false},
		{"unknown status", ApplyStatus("other"), ReasonOperationFailed, AuthorizationStateFailed, 1, nil, false},
		{"success wrong exit", ApplyStatusApplied, "", AuthorizationStateApplied, 1, nil, false},
		{"failure without reason", ApplyStatusFailed, "", AuthorizationStateFailed, 1, nil, false},
		{"failed wrong durable", ApplyStatusFailed, ReasonOperationFailed, AuthorizationStateApplied, 1, nil, false},
		{"unknown reason", ApplyStatusRefused, Reason("other"), AuthorizationStateIssued, 1, nil, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			in := validResultInput()
			in.Status, in.Reason, in.DurableAuthorizationState, in.ExitCode, in.SecondaryFailures = test.status, test.reason, test.durable, test.exit, test.second
			if _, err := NewGitApplyResult(in); (err == nil) != test.valid {
				t.Fatalf("error=%v valid=%t", err, test.valid)
			}
		})
	}
}

func TestApplyResultCompleteness(t *testing.T) {
	// A refusal that never resolved an authorization carries no
	// authorization, plan, or durable state at all.
	unresolved := GitApplyResultInput{SchemaVersion: testResultSchema, ApplyID: testApplyID, Status: ApplyStatusRefused, Reason: ReasonAuthorizationMissing, ExitCode: 1, AuditStatus: "recorded"}
	if _, err := NewGitApplyResult(unresolved); err != nil {
		t.Fatalf("refusal without authorization rejected: %v", err)
	}

	full := validResultInput()
	full.Status, full.Reason, full.DurableAuthorizationState, full.ExitCode = ApplyStatusPartiallyApplied, ReasonOperationFailed, AuthorizationStatePartiallyApplied, 1
	full.PriorAuthorizationState, full.BeforeSnapshotDigest, full.AfterSnapshotDigest = AuthorizationStateIssued, testSHA256, testOtherSHA256
	full.CompletedOperationIDs, full.FailedOperationID, full.NotStartedOperationIDs = []OperationID{operationID(0)}, operationID(1), []OperationID{operationID(2)}
	full.VerifiedEffects = []Condition{{Kind: ConditionHeadState, Expected: "unborn"}}
	full.CommitOID, full.IndexTreeOID, full.RecoveryRequired = testSHA1OID, testOtherSHA1OID, true
	full.Findings = []SecurityFinding{{Path: NewObservedPath([]byte("secrets.env")), Category: "private_key"}}
	result, err := NewGitApplyResult(full)
	if err != nil {
		t.Fatal(err)
	}
	full.CompletedOperationIDs[0] = "changed"
	result.CompletedOperationIDs()[0] = "changed"
	result.Findings()[0].Category = "changed"
	result.VerifiedEffects()[0].Expected = "changed"
	if result.CompletedOperationIDs()[0] != operationID(0) || result.Findings()[0].Category != "private_key" || result.VerifiedEffects()[0].Expected != "unborn" {
		t.Fatal("result retained or exposed caller-owned collections")
	}

	for name, mutate := range map[string]func(*GitApplyResultInput){
		"missing schema":                func(in *GitApplyResultInput) { in.SchemaVersion = "" },
		"missing audit status":          func(in *GitApplyResultInput) { in.AuditStatus = "" },
		"invalid apply ID":              func(in *GitApplyResultInput) { in.ApplyID = "x" },
		"applied without authorization": func(in *GitApplyResultInput) { in.AuthorizationID = "" },
		"applied without plan digest":   func(in *GitApplyResultInput) { in.PlanDigest = "" },
		"authorization without state":   func(in *GitApplyResultInput) { in.DurableAuthorizationState = "" },
		"invalid prior state":           func(in *GitApplyResultInput) { in.PriorAuthorizationState = "other" },
		"invalid commit OID":            func(in *GitApplyResultInput) { in.CommitOID = "zz" },
		"invalid snapshot digest":       func(in *GitApplyResultInput) { in.BeforeSnapshotDigest = "sha256:abc" },
		"invalid verified effect":       func(in *GitApplyResultInput) { in.VerifiedEffects = []Condition{{Kind: "other"}} },
		"operation with two outcomes": func(in *GitApplyResultInput) {
			in.CompletedOperationIDs, in.NotStartedOperationIDs = []OperationID{operationID(0)}, []OperationID{operationID(0)}
		},
		"failed operation also completed": func(in *GitApplyResultInput) {
			in.CompletedOperationIDs, in.FailedOperationID = []OperationID{operationID(0)}, operationID(0)
		},
		"invalid operation ID":     func(in *GitApplyResultInput) { in.IndeterminateOperationIDs = []OperationID{"x"} },
		"finding without category": func(in *GitApplyResultInput) { in.Findings = []SecurityFinding{{Path: NewObservedPath([]byte("a"))}} },
		"unordered secondary failures": func(in *GitApplyResultInput) {
			later := validSecondaryFailure()
			later.ReleaseOrdinal, later.ResourceRole = 2, ResourceRoleIndexLock
			in.Reason, in.ExitCode, in.SecondaryFailures = ReasonResourceReleaseFailed, 1, []SecondaryFailure{later, validSecondaryFailure()}
		},
	} {
		bad := validResultInput()
		mutate(&bad)
		if _, err := NewGitApplyResult(bad); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	stateless := unresolved
	stateless.DurableAuthorizationState = AuthorizationStateIssued
	if _, err := NewGitApplyResult(stateless); err == nil {
		t.Error("durable state without authorization accepted")
	}
}

func TestSecondaryFailureValidationAndOrdering(t *testing.T) {
	for _, role := range []ResourceRole{ResourceRoleApplyLock, ResourceRoleIndexLock, ResourceRoleConfigTemporary, ResourceRolePreparedIndex} {
		failure := validSecondaryFailure()
		failure.ResourceRole = role
		if err := failure.Validate(); err != nil {
			t.Errorf("role %q: %v", role, err)
		}
	}
	for name, mutate := range map[string]func(*SecondaryFailure){
		"foreign code":         func(v *SecondaryFailure) { v.Code = ReasonOperationFailed },
		"foreign phase":        func(v *SecondaryFailure) { v.Phase = "other" },
		"unknown role":         func(v *SecondaryFailure) { v.ResourceRole = "other" },
		"missing class":        func(v *SecondaryFailure) { v.FailureClass = "" },
		"invalid operation ID": func(v *SecondaryFailure) { v.OperationID = "x" },
		"fabricated digest":    func(v *SecondaryFailure) { v.AuditRecordDigest = "sha256:abc" },
	} {
		bad := validSecondaryFailure()
		mutate(&bad)
		if bad.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
	ordered := []SecondaryFailure{
		{Code: ReasonResourceReleaseFailed, Phase: SecondaryFailurePhaseResourceRelease, ResourceRole: ResourceRoleIndexLock, FailureClass: "fsync", OwnershipVerified: true, ReleaseOrdinal: 2},
		{Code: ReasonResourceReleaseFailed, Phase: SecondaryFailurePhaseResourceRelease, ResourceRole: ResourceRolePreparedIndex, FailureClass: "unlink", OwnershipVerified: true, ReleaseOrdinal: 1},
		{Code: ReasonResourceReleaseFailed, Phase: SecondaryFailurePhaseResourceRelease, ResourceRole: ResourceRoleApplyLock, FailureClass: "unlock", OwnershipVerified: true, ReleaseOrdinal: 1},
	}
	SortSecondaryFailures(ordered)
	if ordered[0].ResourceRole != ResourceRoleApplyLock || ordered[1].ResourceRole != ResourceRolePreparedIndex || ordered[2].ReleaseOrdinal != 2 {
		t.Fatalf("secondary failures not deterministically ordered: %+v", ordered)
	}
}

// Validate must hold for every instance, however it was built: the zero
// value of each entity is rejected without going through a constructor.
func TestZeroModelsAreRejected(t *testing.T) {
	models := map[string]interface{ Validate() error }{
		"project identity":       PhysicalProjectIdentity{},
		"tool identity":          GitToolIdentity{},
		"observed path":          ObservedPath{},
		"observed ref":           ObservedRef{},
		"source identity":        InspectedSourceIdentity{},
		"redacted URL":           RedactedRemoteURL{},
		"remote":                 RemoteRedacted{},
		"Git Flow entry":         GitFlowEntry{},
		"Git Flow evidence":      GitFlowEvidence{},
		"attribute source":       AttributeSource{},
		"ignore source":          IgnoreSource{},
		"ignore evaluation":      IgnoreEvaluation{},
		"conversion setting":     EffectiveConversionSetting{},
		"conversion settings":    ConversionSettings{},
		"HEAD observation":       HeadObservation{},
		"linked worktree":        LinkedWorktree{},
		"submodule":              Submodule{},
		"filesystem profile":     FilesystemCapabilityProfile{},
		"ref observation":        RefObservation{},
		"index observation":      IndexObservation{},
		"index change":           IndexChange{},
		"worktree change":        WorktreeChange{},
		"worktree path identity": WorktreePathIdentity{},
		"config entry":           ConfigEntry{},
		"project config":         ProjectConfigObservation{},
		"incompatibility":        Incompatibility{},
		"repository observation": RepositoryObservation{},
		"inspection":             GitInspection{},
		"authorized file":        AuthorizedFile{},
		"development base":       DevelopmentBase{},
		"intent":                 ImmutableIntent{},
		"environment profile":    EnvironmentProfile{},
		"snapshot":               PreconditionSnapshot{},
		"condition":              Condition{},
		"operation":              DerivedOperation{},
		"plan":                   GitInitializationPlan{},
		"authorization":          GitAuthorization{},
		"attempt":                GitApplyAttempt{},
		"result":                 GitApplyResult{},
		"secondary failure":      SecondaryFailure{},
		"security finding":       SecurityFinding{},
		"audit event":            AuditEvent{},
		"transition request":     AuthorizationTransitionRequest{},
		"file identity":          FileIdentity{},
	}
	for name, model := range models {
		if model.Validate() == nil {
			t.Errorf("zero %s accepted", name)
		}
	}
}

func operationID(ordinal uint64) OperationID {
	return OperationID(strconv.FormatUint(ordinal, 10) + ":" + testSHA256)
}

func regularFileIdentity(inode string) FileIdentity {
	return FileIdentity{DeviceDecimal: "1", InodeDecimal: inode, Mode: 0o644, Size: 12}
}

func mustProjectIdentity(t testing.TB) PhysicalProjectIdentity {
	t.Helper()
	identity, err := NewPhysicalProjectIdentity("/tmp/project", []byte("/tmp/project"), "1", "2")
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func mustToolIdentity(t testing.TB) GitToolIdentity {
	t.Helper()
	tool, err := NewGitToolIdentity(GitToolIdentityInput{
		InvocationMode: InvocationModeDirect, InvocationPath: "/usr/bin/git", InvocationPathDigest: testSHA256,
		InvocationDevice: "1", InvocationInode: "2", EffectiveExecutablePath: "/usr/bin/git", EffectiveExecutableDigest: testSHA256,
		Version: "git version test", ExecPath: "/usr/libexec/git-core", Capabilities: []string{"init"},
		EnvironmentSelectors: map[string]string{"profile": "v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return tool
}

func validFilesystemProfileInput(role FilesystemRole) FilesystemCapabilityProfileInput {
	return FilesystemCapabilityProfileInput{
		Role: role, GOOS: "darwin", FilesystemType: "apfs", MountIdentity: "disk3s1", DeviceDecimal: "1", InodeDecimal: "2",
		MountFlags: []string{"local", "journaled"}, Local: true, ProfileVersion: "v1",
		ExclusiveCreate: true, AdvisoryLock: true, FileSync: true, DirectorySync: true,
		AtomicSameFilesystemRename: true, AtomicReplace: true, StableIdentity: true, ProductionSupported: true,
	}
}

func mustFilesystemProfile(t testing.TB, role FilesystemRole) FilesystemCapabilityProfile {
	t.Helper()
	profile, err := NewFilesystemCapabilityProfile(validFilesystemProfileInput(role))
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func mustIndex(t testing.TB, in IndexObservationInput) IndexObservation {
	t.Helper()
	index, err := NewIndexObservation(in)
	if err != nil {
		t.Fatal(err)
	}
	return index
}

// validObservationInput describes a normal repository with an attached HEAD
// and every collection observed empty.
func validObservationInput(t testing.TB) RepositoryObservationInput {
	t.Helper()
	evidence, err := NewGitFlowEvidence(GitFlowEvidenceInput{ObservationState: ObservationObservedEmpty, Classification: GitFlowAbsent})
	if err != nil {
		t.Fatal(err)
	}
	conversion, err := NewConversionSettings(ConversionSettingsInput{ObservationState: ObservationObservedEmpty, Risk: ConversionRiskNone, ByteIdentityProven: true})
	if err != nil {
		t.Fatal(err)
	}
	gitDir := NewObservedPath([]byte("/tmp/project/.git"))
	return RepositoryObservationInput{
		RepositoryKind: RepositoryKindNormal, GitDir: gitDir, CommonDir: gitDir, ObjectFormat: ObjectFormatSHA1, RefFormat: RefFormatFiles,
		SequencerState:       SequencerNone,
		Head:                 HeadObservation{State: HeadStateAttached, SymbolicRef: NewObservedRef([]byte("refs/heads/main")), CommitOID: testSHA1OID, HistoryPresent: true},
		RefsObservationState: ObservationObservedEmpty, ConfigLockState: LockFileAbsent,
		Index:                    mustIndex(t, IndexObservationInput{Presence: IndexPresent, Version: 2, EntryCountDecimal: "0", ContentDigest: testSHA256, LockState: LockFileAbsent}),
		WorktreeObservationState: ObservationObservedEmpty, ConfigurationObservationState: ObservationObservedEmpty,
		ProjectConfig:   ProjectConfigObservation{State: ProjectConfigAbsent, Path: NewObservedPath([]byte("/tmp/project/.ai-dev/config.json"))},
		GitFlowEvidence: evidence, ConversionSettings: conversion,
		AttributeSourcesObservationState: ObservationObservedEmpty, IgnoreSourcesObservationState: ObservationObservedEmpty,
		IgnoreEvaluationsObservationState: ObservationObservedEmpty, LinkedWorktreesObservationState: ObservationObservedEmpty,
		SubmodulesObservationState: ObservationObservedEmpty, FilesystemCapabilitiesObservationState: ObservationObserved,
		FilesystemCapabilities: []FilesystemCapabilityProfile{mustFilesystemProfile(t, FilesystemRoleProject), mustFilesystemProfile(t, FilesystemRoleStateRoot)},
	}
}

func absentObservationInput(t testing.TB) RepositoryObservationInput {
	t.Helper()
	in := validObservationInput(t)
	in.RepositoryKind, in.GitDir, in.CommonDir, in.ObjectFormat, in.RefFormat = RepositoryKindAbsent, ObservedPath{}, ObservedPath{}, "", ""
	in.Head = HeadObservation{State: HeadStateUnborn}
	in.RefsObservationState = ObservationNotApplicable
	in.Index = mustIndex(t, IndexObservationInput{Presence: IndexAbsent, LockState: LockFileAbsent})
	in.LinkedWorktreesObservationState, in.SubmodulesObservationState = ObservationNotApplicable, ObservationNotApplicable
	return in
}

func mustObservation(t testing.TB, in RepositoryObservationInput) RepositoryObservation {
	t.Helper()
	observation, err := NewRepositoryObservation(in)
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func validInspectionInput(t testing.TB) GitInspectionInput {
	t.Helper()
	return GitInspectionInput{
		SchemaVersion: testInspectionSchema, InspectionID: "inspection-1", InspectedAt: time.Unix(1, 0).UTC(),
		ProjectIdentity: mustProjectIdentity(t), Observation: mustObservation(t, validObservationInput(t)),
		RemotesObservationState: ObservationObservedEmpty, ToolIdentity: mustToolIdentity(t),
	}
}

func validAuthorizedFile(path string) AuthorizedFile {
	return AuthorizedFile{
		PathUTF8: path, ReversiblePathBytesBase64URL: EncodeReversiblePath([]byte(path)), Mode: "100644",
		ContentSHA256: testSHA256, BlobOID: testSHA1OID, ApprovalKind: ApprovalKindNormal, IndividuallyApproved: true,
		ObservedIdentity: regularFileIdentity("3"),
	}
}

func validIntentInput(t testing.TB) ImmutableIntentInput {
	t.Helper()
	identity := GitIdentity{Name: "A", Email: "a@example.test"}
	return ImmutableIntentInput{
		ProjectID: testProjectID, Strategy: StrategyBasic, BranchParameters: map[string]string{"initial": "main"},
		PrefixParameters: map[string]string{}, InitializeRepository: true, ObjectFormat: ObjectFormatSHA1, RefFormat: RefFormatFiles,
		ToolIdentity: mustToolIdentity(t), ProjectConfigBytesDigest: testSHA256, InitialCommit: true,
		Author: identity, Committer: identity, AuthorTimestamp: time.Unix(1, 0).UTC(), CommitterTimestamp: time.Unix(1, 0).UTC(),
		CommitMessageBytes: []byte("chore: initial commit\n"), AuthorizedFiles: []AuthorizedFile{validAuthorizedFile(ProjectConfigPath)},
		RequiredCapabilities: []string{"init"},
	}
}

func noCommitIntentInput(t testing.TB) ImmutableIntentInput {
	t.Helper()
	in := validIntentInput(t)
	in.InitialCommit, in.AuthorizedFiles, in.CommitMessageBytes = false, nil, nil
	in.Author, in.Committer, in.AuthorTimestamp, in.CommitterTimestamp = GitIdentity{}, GitIdentity{}, time.Time{}, time.Time{}
	return in
}

func developmentIntentInput(t testing.TB) ImmutableIntentInput {
	t.Helper()
	in := noCommitIntentInput(t)
	in.Strategy, in.InitializeRepository, in.DevelopmentBase = StrategyGitFlow, false, validDevelopmentBase()
	return in
}

func validDevelopmentBase() DevelopmentBase {
	return DevelopmentBase{BaseOID: testSHA1OID, ObjectFormat: ObjectFormatSHA1, ProductionRef: NewObservedRef([]byte("refs/heads/main")), ProductionTipOID: testSHA1OID}
}

func environmentProfileFor(root string) EnvironmentProfile {
	return EnvironmentProfile{
		Version:       "v1",
		StateRoot:     NewObservedPath([]byte(root)),
		Home:          NewObservedPath([]byte(root + "/" + EnvironmentHomeLeaf)),
		XDGConfigHome: NewObservedPath([]byte(root + "/" + EnvironmentXDGLeaf)),
	}
}

func validEnvironmentProfile() EnvironmentProfile { return environmentProfileFor(testStateRoot) }

func validSnapshot(t testing.TB) PreconditionSnapshot {
	t.Helper()
	return PreconditionSnapshot{
		ProjectIdentity: mustProjectIdentity(t), Observation: mustObservation(t, validObservationInput(t)),
		EnvironmentProfile: validEnvironmentProfile(), ToolIdentity: mustToolIdentity(t),
	}
}

func validOperationInput(ordinal uint64) DerivedOperationInput {
	configPath := EncodeReversiblePath([]byte(".git"))
	return DerivedOperationInput{
		OperationID: operationID(ordinal), Ordinal: ordinal, Kind: OperationKindRepositoryInit,
		ExactArgv: []string{"init", "--template="}, EnvironmentProfile: "apply-v1", TargetIdentity: "project",
		Preconditions:               []Condition{{Kind: ConditionRepositoryAbsent}},
		AffectedPaths:               []ObservedPath{NewObservedPath([]byte(".git"))},
		AffectedRefs:                []ObservedRef{NewObservedRef([]byte("refs/heads/main"))},
		AffectedConfigKeys:          []string{"core.bare"},
		ExpectedEffects:             []Condition{{Kind: ConditionRepositoryKind, Expected: "normal"}},
		ExpectedPostconditions:      []Condition{{Kind: ConditionHeadState, Expected: "unborn"}, {Kind: ConditionPathMode, Subject: configPath, Expected: "40755"}},
		FailureOutcomes:             []Reason{ReasonOperationFailed},
		PartialEffectClassification: PartialEffectPartialPossible, RecoveryInstruction: RecoverySupervisedReconciliation,
	}
}

func verificationOperationInput(ordinal uint64) DerivedOperationInput {
	return DerivedOperationInput{
		OperationID: operationID(ordinal), Ordinal: ordinal, Kind: OperationKindVerification, TargetIdentity: "project",
		Preconditions:               []Condition{{Kind: ConditionRepositoryKind, Expected: "normal"}},
		ExpectedPostconditions:      []Condition{{Kind: ConditionHeadState, Expected: "unborn"}},
		PartialEffectClassification: PartialEffectNone, RecoveryInstruction: RecoveryNotApplicable,
	}
}

func validPlanInput(t testing.TB) GitInitializationPlanInput {
	t.Helper()
	intent, err := NewImmutableIntent(validIntentInput(t))
	if err != nil {
		t.Fatal(err)
	}
	operation := mustOperation(t, validOperationInput(0))
	in := GitInitializationPlanInput{
		SchemaVersion: GitInitializationPlanSchema, PlanID: "plan-1", GeneratedAt: time.Unix(5, 0).UTC(),
		InspectionID: "inspection-1", InspectedAt: time.Unix(1, 0).UTC(), ImmutableIntent: intent, PreconditionSnapshot: validSnapshot(t),
		DerivedOperations: []DerivedOperation{operation}, ExpectedPostconditions: []Condition{{Kind: ConditionHeadState, Expected: "unborn"}},
		ProhibitedOperations: ProhibitedOperations(), PartialEffectPoints: []OperationID{operation.OperationID},
		RecoveryPolicy: RecoveryPolicySupervisedReconciliation,
	}
	sealPlanInput(t, &in)
	return in
}

// mustOperation builds an operation whose ID is derived the way the
// planner derives it, against the fixture environment profile.
func mustOperation(t testing.TB, in DerivedOperationInput) DerivedOperation {
	t.Helper()
	id, err := DeriveOperationID(in, validEnvironmentProfile())
	if err != nil {
		t.Fatal(err)
	}
	in.OperationID = id
	operation, err := NewDerivedOperation(in)
	if err != nil {
		t.Fatal(err)
	}
	return operation
}

// sealPlanInput recomputes the partial-effect points and the plan digest
// after a fixture changed covered members.
func sealPlanInput(t testing.TB, in *GitInitializationPlanInput) {
	t.Helper()
	in.PartialEffectPoints = nil
	for _, operation := range in.DerivedOperations {
		if operation.PartialEffectClassification != PartialEffectNone {
			in.PartialEffectPoints = append(in.PartialEffectPoints, operation.OperationID)
		}
	}
	digest, err := ComputePlanDigest(*in)
	if err != nil {
		t.Fatal(err)
	}
	in.PlanDigest = digest
}

func validAuthorizationInput(t testing.TB) GitAuthorizationInput {
	t.Helper()
	return GitAuthorizationInput{
		SchemaVersion: testAuthorizationSchema, AuthorizationID: testAuthorizationID, PlanDigest: testSHA256,
		ProjectIdentity: mustProjectIdentity(t), AuthorizedOperationIDs: []OperationID{operationID(0)}, Strategy: StrategyBasic,
		AuthorizedRefs: []ObservedRef{NewObservedRef([]byte("refs/heads/main"))}, AuthorizedConfigKeys: []string{"gitflow.branch.master"},
		DeclaredOperator: "operator", IssuedAt: time.Unix(10, 0).UTC(), State: AuthorizationStateIssued, StateVersion: 1,
	}
}

func validResultInput() GitApplyResultInput {
	return GitApplyResultInput{
		SchemaVersion: testResultSchema, ApplyID: testApplyID, AuthorizationID: testAuthorizationID, PlanID: "plan-1",
		PlanDigest: testSHA256, Status: ApplyStatusApplied, DurableAuthorizationState: AuthorizationStateApplied, AuditStatus: "recorded",
	}
}

func validSecondaryFailure() SecondaryFailure {
	return SecondaryFailure{Code: ReasonResourceReleaseFailed, Phase: SecondaryFailurePhaseResourceRelease, ResourceRole: ResourceRoleApplyLock, FailureClass: "unlock", OwnershipVerified: true, ReleaseOrdinal: 1}
}
