package gitmanager

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"
)

// RFC 8785 section 3.2.3: properties sort by UTF-16 code units, so the
// emoji (U+1F600, surrogate D83D) precedes U+FB33.
func TestCanonicalPropertySortingVector(t *testing.T) {
	value := map[string]any{
		"\u20ac":     "Euro Sign",
		"\r":         "Carriage Return",
		"\ufb33":     "Hebrew Letter Dalet With Dagesh",
		"1":          "One",
		"\U0001F600": "Emoji: Grinning Face",
		"\u0080":     "Control",
		"\u00f6":     "Latin Small Letter O With Diaeresis",
	}
	want := "{\"\\r\":\"Carriage Return\",\"1\":\"One\",\"\u0080\":\"Control\",\"\u00f6\":\"Latin Small Letter O With Diaeresis\"," +
		"\"\u20ac\":\"Euro Sign\",\"\U0001F600\":\"Emoji: Grinning Face\",\"\ufb33\":\"Hebrew Letter Dalet With Dagesh\"}"
	got, err := MarshalCanonical(value)
	if err != nil || string(got) != want {
		t.Fatalf("got %s, %v\nwant %s", got, err, want)
	}
}

// RFC 8785 section 3.2.2.2: only '"', '\' and C0 controls are escaped, with
// short forms where JSON has them and lowercase \u00xx otherwise.
func TestCanonicalStringEscapingVector(t *testing.T) {
	value := map[string]any{"string": "\u20ac$\x0f\nA'B\"\\\\\"/", "literals": []any{nil, true, false}}
	want := `{"literals":[null,true,false],"string":"€$\u000f\nA'B\"\\\\\"/"}`
	got, err := MarshalCanonical(value)
	if err != nil || string(got) != want {
		t.Fatalf("got %s, %v\nwant %s", got, err, want)
	}
	// The RFC input spells the same value with escapes; it decodes, but it
	// is not canonical.
	input := `{"literals":[null,true,false],"string":"\u20ac$\u000F\u000aA'\u0042\u0022\u005c\\\"\/"}`
	if _, err := UnmarshalCanonical([]byte(input), PlanDecodeLimits); !errors.Is(err, errNonCanonical) {
		t.Fatalf("escaped RFC input: %v, want non-canonical", err)
	}
	decoded, err := UnmarshalCanonical([]byte(want), PlanDecodeLimits)
	if err != nil || decoded.(map[string]any)["string"] != value["string"] {
		t.Fatalf("canonical round trip: %#v, %v", decoded, err)
	}
}

func TestCanonicalIntegers(t *testing.T) {
	got, err := MarshalCanonical([]any{int64(0), int64(-1), int64(maxSafeInteger), int64(-maxSafeInteger)})
	if err != nil || string(got) != "[0,-1,9007199254740991,-9007199254740991]" {
		t.Fatalf("got %s, %v", got, err)
	}
	for _, unsafe := range []int64{maxSafeInteger + 1, -maxSafeInteger - 1} {
		if _, err := MarshalCanonical(unsafe); !errors.Is(err, errCanonicalSyntax) {
			t.Errorf("unsafe integer %d: %v", unsafe, err)
		}
	}
	// F008-FR-171: beyond the safe range a model integer becomes a decimal
	// string, never a JSON number.
	if canonicalInteger(maxSafeInteger) != int64(maxSafeInteger) || canonicalInteger(maxSafeInteger+1) != "9007199254740992" ||
		canonicalInteger(18446744073709551615) != "18446744073709551615" {
		t.Fatal("canonicalInteger does not switch to decimal strings above the safe range")
	}
	for _, unsupported := range []any{1.5, float32(1), int(1), uint64(1), []string{"x"}, map[string]string{}} {
		if _, err := MarshalCanonical(unsupported); !errors.Is(err, errCanonicalSyntax) {
			t.Errorf("unsupported %T accepted: %v", unsupported, err)
		}
	}
}

// Canonicalization never normalizes Unicode: NFC and NFD spellings of the
// same text keep distinct bytes and distinct digests.
func TestCanonicalPreservesUnicodeNormalization(t *testing.T) {
	nfc, nfd := "caf\u00e9", "cafe\u0301"
	a, err := MarshalCanonical(map[string]any{nfc: nfc})
	if err != nil {
		t.Fatal(err)
	}
	b, err := MarshalCanonical(map[string]any{nfd: nfd})
	if err != nil {
		t.Fatal(err)
	}
	if string(a) == string(b) {
		t.Fatal("NFC and NFD encoded identically")
	}
	digestA, _ := canonicalSHA256(map[string]any{nfc: nfc})
	digestB, _ := canonicalSHA256(map[string]any{nfd: nfd})
	if digestA == digestB {
		t.Fatal("NFC and NFD share a digest")
	}
	for _, encoded := range [][]byte{a, b} {
		decoded, err := UnmarshalCanonical(encoded, PlanDecodeLimits)
		if err != nil {
			t.Fatal(err)
		}
		again, _ := MarshalCanonical(decoded)
		if string(again) != string(encoded) {
			t.Fatalf("round trip changed bytes: %q -> %q", encoded, again)
		}
	}
	if _, err := MarshalCanonical("bad\xffutf8"); !errors.Is(err, errCanonicalSyntax) {
		t.Fatalf("invalid UTF-8 accepted: %v", err)
	}
}

func TestUnmarshalCanonicalRejections(t *testing.T) {
	syntax := map[string]string{
		"duplicate member":    `{"a":1,"a":2}`,
		"fraction":            `{"a":1.0}`,
		"exponent":            `{"a":1e3}`,
		"leading zero":        `{"a":01}`,
		"negative zero":       `{"a":-0}`,
		"unsafe integer":      `{"a":9007199254740992}`,
		"lone high surrogate": `{"a":"\ud800"}`,
		"lone low surrogate":  `{"a":"\udc00"}`,
		"raw control":         "{\"a\":\"\x01\"}",
		"invalid escape":      `{"a":"\x"}`,
		"trailing data":       `{}{}`,
		"trailing comma":      `{"a":1,}`,
		"unterminated":        `{"a":"x`,
		"bare word":           `{"a":nope}`,
		"empty input":         ``,
		"invalid UTF-8":       "{\"a\":\"\xff\"}",
	}
	for name, input := range syntax {
		if _, err := UnmarshalCanonical([]byte(input), PlanDecodeLimits); !errors.Is(err, errCanonicalSyntax) {
			t.Errorf("%s: %v, want syntax error", name, err)
		}
	}
	nonCanonical := map[string]string{
		"whitespace":       `{ "a":1}`,
		"unsorted keys":    `{"b":1,"a":2}`,
		"escaped letter":   `{"a":"\u0041"}`,
		"escaped solidus":  `{"a":"\/"}`,
		"uppercase escape": `{"a":"\u001F"}`,
		"trailing newline": "{}\n",
		"code point order": "{\"\ufb33\":1,\"\U0001F600\":2}",
		"escaped pair":     `{"a":"\ud83d\ude00"}`,
	}
	for name, input := range nonCanonical {
		if _, err := UnmarshalCanonical([]byte(input), PlanDecodeLimits); !errors.Is(err, errNonCanonical) {
			t.Errorf("%s: %v, want non-canonical", name, err)
		}
	}
	// A surrogate-pair escape decodes to the single code point it spells.
	p := canonicalParser{data: []byte(`"\ud83d\ude00"`), limits: PlanDecodeLimits}
	if decoded, err := p.string(); err != nil || decoded != "\U0001F600" {
		t.Fatalf("surrogate pair decoded to %q, %v", decoded, err)
	}
}

func TestUnmarshalCanonicalLimits(t *testing.T) {
	limits := DecodeLimits{MaxBytes: 64, MaxDepth: 3, MaxContainerMembers: 2, MaxStringBytes: 4}
	accepted := []string{`[[[]]]`, `{"a":1,"b":2}`, `"abcd"`, `{"abcd":"abcd"}`}
	for _, input := range accepted {
		if _, err := UnmarshalCanonical([]byte(input), limits); err != nil {
			t.Errorf("exact boundary %s rejected: %v", input, err)
		}
	}
	rejected := map[string]string{
		"depth":        `[[[[]]]]`,
		"members":      `{"a":1,"b":2,"c":3}`,
		"items":        `[1,2,3]`,
		"string bytes": `"abcde"`,
		"key bytes":    `{"abcde":1}`,
		"total bytes":  `"` + strings.Repeat("a", 70) + `"`,
	}
	for name, input := range rejected {
		if _, err := UnmarshalCanonical([]byte(input), limits); !errors.Is(err, errCanonicalLimit) {
			t.Errorf("%s: %v, want limit", name, err)
		}
	}
	deep := strings.Repeat("[", maxCanonicalDepth+1) + strings.Repeat("]", maxCanonicalDepth+1)
	if _, err := UnmarshalCanonical([]byte(deep), PlanDecodeLimits); !errors.Is(err, errCanonicalLimit) {
		t.Fatalf("plan nesting above %d accepted: %v", maxCanonicalDepth, err)
	}
	var nested any = []any{}
	for i := 1; i < maxCanonicalDepth; i++ {
		nested = []any{nested}
	}
	if _, err := MarshalCanonical(nested); err != nil {
		t.Fatalf("writer rejected the nesting bound: %v", err)
	}
	if _, err := MarshalCanonical([]any{nested}); !errors.Is(err, errCanonicalLimit) {
		t.Fatalf("writer accepted nesting above the bound: %v", err)
	}
}

func TestCanonicalFieldsRejectsUnknownAndMissingMembers(t *testing.T) {
	value, err := UnmarshalCanonical([]byte(`{"expected":"unborn","kind":"head_state"}`), PlanDecodeLimits)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := canonicalFields(value, []string{"kind"}, []string{"subject", "expected"}); err != nil {
		t.Fatalf("closed member set rejected: %v", err)
	}
	if _, err := canonicalFields(value, []string{"kind"}, []string{"subject"}); !errors.Is(err, errCanonicalSyntax) {
		t.Fatalf("unknown member accepted: %v", err)
	}
	if _, err := canonicalFields(value, []string{"kind", "subject"}, nil); !errors.Is(err, errCanonicalSyntax) {
		t.Fatalf("missing required member accepted: %v", err)
	}
	if _, err := canonicalFields([]any{}, nil, nil); !errors.Is(err, errCanonicalSyntax) {
		t.Fatalf("non-object accepted: %v", err)
	}
}

// The member names are part of the plan schema; pin a representative set.
func TestCanonicalModelMemberNames(t *testing.T) {
	e := &canonicalEncoder{}
	for name, test := range map[string]struct {
		value any
		want  string
	}{
		"condition": {e.condition(Condition{Kind: ConditionHeadState, Expected: "unborn"}), `{"expected":"unborn","kind":"head_state"}`},
		"path":      {e.path(NewObservedPath([]byte("a"))), `{"display_path_utf8":"a","reversible_path_bytes_base64url":"YQ"}`},
		"raw path":  {e.path(NewObservedPath([]byte{0xff})), `{"reversible_path_bytes_base64url":"_w"}`},
		"identity": {e.fileIdentity(FileIdentity{DeviceDecimal: "1", InodeDecimal: "18446744073709551615", Mode: 0o644, Size: 12}),
			`{"device_decimal":"1","file_type":"regular","inode_decimal":"18446744073709551615","permissions_octal":"0644","size":12}`},
		"head": {e.head(HeadObservation{State: HeadStateUnborn}), `{"history_present":false,"state":"unborn"}`},
	} {
		got, err := MarshalCanonical(test.value)
		if err != nil || string(got) != test.want {
			t.Errorf("%s: got %s, %v want %s", name, got, err, test.want)
		}
	}
	if e.err != nil {
		t.Fatal(e.err)
	}
}

func TestDeriveOperationID(t *testing.T) {
	in := validOperationInput(3)
	id, err := DeriveOperationID(in, validEnvironmentProfile())
	if err != nil {
		t.Fatal(err)
	}
	again, err := DeriveOperationID(in, validEnvironmentProfile())
	if err != nil || again != id || !strings.HasPrefix(string(id), "3:sha256:") || id.Validate() != nil {
		t.Fatalf("operation ID %q / %q, %v", id, again, err)
	}

	same := map[string]func(*DerivedOperationInput){
		"stored ID":                func(v *DerivedOperationInput) { v.OperationID = "anything" },
		"affected paths":           func(v *DerivedOperationInput) { v.AffectedPaths = nil },
		"expected postconditions":  func(v *DerivedOperationInput) { v.ExpectedPostconditions = nil },
		"failure outcomes":         func(v *DerivedOperationInput) { v.FailureOutcomes = []Reason{ReasonConcurrentChange} },
		"recovery instruction":     func(v *DerivedOperationInput) { v.RecoveryInstruction = RecoveryNotApplicable },
		"temporary resource roles": func(v *DerivedOperationInput) { v.TemporaryResourceRoles = []ResourceRole{ResourceRoleIndexLock} },
	}
	for name, mutate := range same {
		changed := validOperationInput(3)
		mutate(&changed)
		if got, _ := DeriveOperationID(changed, validEnvironmentProfile()); got != id {
			t.Errorf("%s changed the operation ID", name)
		}
	}
	different := map[string]func(*DerivedOperationInput){
		"ordinal":          func(v *DerivedOperationInput) { v.Ordinal = 4 },
		"kind":             func(v *DerivedOperationInput) { v.Kind = OperationKindGitConfigWrite },
		"argv":             func(v *DerivedOperationInput) { v.ExactArgv = []string{"init", "--template=", "--bare"} },
		"argv order":       func(v *DerivedOperationInput) { v.ExactArgv = []string{"--template=", "init"} },
		"environment name": func(v *DerivedOperationInput) { v.EnvironmentProfile = "apply-v2" },
		"target":           func(v *DerivedOperationInput) { v.TargetIdentity = "other" },
		"precondition":     func(v *DerivedOperationInput) { v.Preconditions = []Condition{{Kind: ConditionIndexAbsent}} },
		"expected effect":  func(v *DerivedOperationInput) { v.ExpectedEffects = []Condition{{Kind: ConditionIndexEmpty}} },
		"partial effect":   func(v *DerivedOperationInput) { v.PartialEffectClassification = PartialEffectAtomic },
		"NFD target":       func(v *DerivedOperationInput) { v.TargetIdentity = "proje\u0301ct" },
	}
	for name, mutate := range different {
		changed := validOperationInput(3)
		mutate(&changed)
		if got, err := DeriveOperationID(changed, validEnvironmentProfile()); err != nil || got == id {
			t.Errorf("%s did not change the operation ID (%v)", name, err)
		}
	}
	// A Git invocation binds the exact frozen environment bytes (plan L-2);
	// an operation without argv does not depend on them.
	if got, _ := DeriveOperationID(in, environmentProfileFor("/other/state/git")); got == id {
		t.Error("environment path bytes did not change a Git operation ID")
	}
	verification := verificationOperationInput(1)
	a, _ := DeriveOperationID(verification, validEnvironmentProfile())
	b, _ := DeriveOperationID(verification, environmentProfileFor("/other/state/git"))
	if a != b {
		t.Error("environment changed the ID of an operation without argv")
	}
	bad := validOperationInput(3)
	bad.Preconditions = []Condition{{Kind: ConditionPathAbsent, Subject: EncodeReversiblePath([]byte("bad\xff"))}}
	if _, err := DeriveOperationID(bad, validEnvironmentProfile()); err != nil {
		t.Fatalf("subject bytes are base64url and always encodable: %v", err)
	}
}

func TestPlanDigest(t *testing.T) {
	in := validPlanInput(t)
	plan, err := NewGitInitializationPlan(in)
	if err != nil {
		t.Fatal(err)
	}
	covered, err := CanonicalCoveredPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UnmarshalCanonical(covered, PlanDecodeLimits); err != nil {
		t.Fatalf("covered plan is not canonical: %v", err)
	}
	for _, member := range []string{`"schema_version"`, `"immutable_intent"`, `"precondition_snapshot"`, `"derived_operations"`,
		`"expected_postconditions"`, `"prohibited_operations"`, `"partial_effect_points"`, `"recovery_policy"`} {
		if !strings.Contains(string(covered), member) {
			t.Errorf("covered plan lacks %s", member)
		}
	}
	for _, excluded := range []string{`"plan_id"`, `"generated_at"`, `"expires_at"`, `"inspection_id"`, `"inspected_at"`, `"plan_digest"`} {
		if strings.Contains(string(covered), excluded) {
			t.Errorf("covered plan includes excluded %s", excluded)
		}
	}

	// F008-FR-172: excluded metadata never changes the digest.
	metadata := in
	metadata.PlanID, metadata.InspectionID = "plan-2", "inspection-2"
	metadata.GeneratedAt, metadata.InspectedAt, metadata.ExpiresAt = time.Unix(50, 0).UTC(), time.Unix(40, 0).UTC(), time.Unix(60, 0).UTC()
	if digest, err := ComputePlanDigest(metadata); err != nil || digest != in.PlanDigest {
		t.Fatalf("metadata changed the digest: %q, %v", digest, err)
	}
	if _, err := NewGitInitializationPlan(metadata); err != nil {
		t.Fatalf("plan with other metadata rejected: %v", err)
	}

	covers := map[string]func(*GitInitializationPlanInput){
		"postcondition": func(v *GitInitializationPlanInput) {
			v.ExpectedPostconditions = []Condition{{Kind: ConditionHeadState, Expected: "attached"}}
		},
		"intent strategy": func(v *GitInitializationPlanInput) {
			intentInput := validIntentInput(t)
			intentInput.Strategy = StrategyGitHubFlow
			intent, err := NewImmutableIntent(intentInput)
			if err != nil {
				t.Fatal(err)
			}
			v.ImmutableIntent = intent
		},
		"snapshot environment": func(v *GitInitializationPlanInput) {
			v.PreconditionSnapshot.EnvironmentProfile = environmentProfileFor("/other/state/git")
		},
	}
	for name, mutate := range covers {
		changed := validPlanInput(t)
		mutate(&changed)
		if digest, err := ComputePlanDigest(changed); err != nil || digest == in.PlanDigest {
			t.Errorf("%s did not change the digest (%v)", name, err)
		}
		if _, err := NewGitInitializationPlan(changed); err == nil {
			t.Errorf("%s: plan with a stale digest accepted", name)
		}
	}

	tampered := validPlanInput(t)
	tampered.PlanDigest = testSHA256
	if _, err := NewGitInitializationPlan(tampered); err == nil {
		t.Fatal("plan with a forged digest accepted")
	}
	forgedOperation := validOperationInput(0)
	forgedOperation.OperationID = operationID(0)
	operation, err := NewDerivedOperation(forgedOperation)
	if err != nil {
		t.Fatal(err)
	}
	forged := validPlanInput(t)
	forged.DerivedOperations = []DerivedOperation{operation}
	sealPlanInput(t, &forged)
	if _, err := NewGitInitializationPlan(forged); err == nil {
		t.Fatal("plan with an underived operation ID accepted")
	}

	subSecond := validIntentInput(t)
	subSecond.AuthorTimestamp = time.Unix(1, 5).UTC()
	intent, err := NewImmutableIntent(subSecond)
	if err != nil {
		t.Fatal(err)
	}
	fractional := validPlanInput(t)
	fractional.ImmutableIntent = intent
	if _, err := ComputePlanDigest(fractional); !errors.Is(err, errCanonicalSyntax) {
		t.Fatalf("sub-second commit timestamp encoded: %v", err)
	}
}

// richObservationInput populates every observation group, so the digest
// test exercises the canonical encoding of each member type.
func richObservationInput(t testing.TB) RepositoryObservationInput {
	t.Helper()
	in := validObservationInput(t)
	path := func(p string) ObservedPath { return NewObservedPath([]byte(p)) }
	entry := func(key, value string) GitFlowEntry {
		return GitFlowEntry{Key: key, Value: value, Origin: "local", SourceDigest: testSHA256}
	}
	var err error
	in.RefsObservationState = ObservationObserved
	in.Refs = []RefObservation{
		{Name: NewObservedRef([]byte("refs/heads/main")), TargetOID: testSHA1OID},
		{Name: NewObservedRef([]byte("refs/tags/v1")), TargetOID: testOtherSHA1OID, PeeledOID: testSHA1OID},
	}
	in.WorktreeObservationState = ObservationObserved
	in.WorktreeChanges = []WorktreeChange{{Path: path("notes.txt"), Kind: WorktreeChangeUntracked}}
	in.WorktreePathIdentities = []WorktreePathIdentity{
		{Path: path("dir"), Identity: FileIdentity{DeviceDecimal: "1", InodeDecimal: "9", Mode: fs.ModeDir | 0o755}},
		{Path: path("link"), Identity: FileIdentity{DeviceDecimal: "1", InodeDecimal: "8", Mode: fs.ModeSymlink | 0o777}},
		{Path: path("notes.txt"), Identity: regularFileIdentity("10"), ContentDigest: testSHA256},
	}
	in.ConfigurationObservationState = ObservationObserved
	in.EffectiveConfiguration = []ConfigEntry{
		{Ordinal: 0, Key: "core.bare", ValuePresent: true, ValueRedacted: "false", ValueRedaction: RedactionNotNeeded, Scope: ConfigScopeLocal, Origin: path("/tmp/project/.git/config"), SourceDigest: testSHA256},
		{Ordinal: 1, Key: "core.fsmonitor", ValuePresent: true, ValueRedacted: "false", ValueRedaction: RedactionNotNeeded, Scope: ConfigScopeCommand},
	}
	in.ProjectConfig = ProjectConfigObservation{State: ProjectConfigValid, Path: path("/tmp/project/.ai-dev/config.json"), Identity: regularFileIdentity("11"), ContentDigest: testSHA256, ProjectID: testProjectID}
	if in.GitFlowEvidence, err = NewGitFlowEvidence(GitFlowEvidenceInput{
		ObservationState: ObservationObserved, Classification: GitFlowCompleteAVH, Dialect: GitFlowDialectAVH,
		Entries:  []GitFlowEntry{entry("gitflow.branch.develop", "develop"), entry("gitflow.branch.master", "main")},
		Prefixes: []GitFlowEntry{entry("gitflow.prefix.feature", "feature/")}, InstanceBases: []GitFlowEntry{entry("gitflow.branch.feature/x.base", "develop")},
		ProductionBranch: "main", DevelopmentBranch: "develop", SourceDigests: []ContentDigest{testSHA256}, Issues: []string{"note"},
	}); err != nil {
		t.Fatal(err)
	}
	read := InspectedSourceIdentity{Scope: "local", Path: path(".gitignore"), PhysicalIdentityDigest: testSHA256, ContentDigest: testOtherSHA256, ReadState: SourceReadRead, ObservedSizeDecimal: "12", Precedence: 2}
	in.AttributeSourcesObservationState = ObservationObserved
	in.AttributeSources = []AttributeSource{{Identity: read, Kind: AttributeSourceRepository}}
	in.IgnoreSourcesObservationState = ObservationObserved
	in.IgnoreSources = []IgnoreSource{{Identity: read, Kind: IgnoreSourceRepository, BaseDirectory: path("."), HasDirectoryDepth: true}}
	evaluation, err := NewIgnoreEvaluation(IgnoreEvaluationInput{Path: path("notes.txt"), OrderedSourceDigests: []ContentDigest{testOtherSHA256}, MatchedRules: []string{"*.txt"}, Ignored: true, WinningRuleSource: testOtherSHA256, EquivalenceProven: true})
	if err != nil {
		t.Fatal(err)
	}
	in.IgnoreEvaluationsObservationState, in.IgnoreEvaluations = ObservationObserved, []IgnoreEvaluation{evaluation}
	if in.ConversionSettings, err = NewConversionSettings(ConversionSettingsInput{
		ObservationState: ObservationObserved, Risk: ConversionRiskNeutral, ByteIdentityProven: true, SourceDigests: []ContentDigest{testSHA256},
		Settings: []EffectiveConversionSetting{{Key: "core.autocrlf", ConfiguredValueRedacted: "false", EffectiveValueRedacted: "false", SourceScope: "global", SourceDigest: testSHA256, InterpretationState: "interpreted"}},
	}); err != nil {
		t.Fatal(err)
	}
	in.LinkedWorktreesObservationState = ObservationObserved
	in.LinkedWorktrees = []LinkedWorktree{{
		ObservationState: ObservationObserved, WorktreePath: path("/tmp/wt"), GitDirPath: path("/tmp/project/.git/worktrees/wt"), CommonDirIdentity: testSHA256,
		Head: HeadObservation{State: HeadStateDetached, CommitOID: testSHA1OID, HistoryPresent: true}, Availability: WorktreeLocked,
		Locked: true, LockReasonRedacted: "maintenance", PrunableObserved: true,
	}}
	in.SubmodulesObservationState = ObservationObserved
	in.Submodules = []Submodule{{
		ObservationState: ObservationObserved, Name: "lib", Path: path("vendor/lib"), RecordedCommitOID: testSHA1OID, DeclarationDigest: testSHA256,
		InitializationState: SubmoduleInitializedMatching, ObservedPathIdentity: testSHA256, ObservedHeadOID: testSHA1OID, ObservedRepositoryKind: RepositoryKindSubmodule,
		URLRedacted: RedactedRemoteURL{DisplayRedacted: "https://***@example.test/lib.git", CredentialMaterialDetected: true, RedactionState: RedactionRedacted, OriginalDigest: testSHA256},
	}}
	return in
}

func TestPlanDigestCoversEveryObservationGroup(t *testing.T) {
	snapshot := validSnapshot(t)
	snapshot.Observation = mustObservation(t, richObservationInput(t))
	snapshot.DevelopmentBase = validDevelopmentBase()
	intent, err := NewImmutableIntent(developmentIntentInput(t))
	if err != nil {
		t.Fatal(err)
	}
	in := validPlanInput(t)
	in.ImmutableIntent, in.PreconditionSnapshot = intent, snapshot
	sealPlanInput(t, &in)
	plan, err := NewGitInitializationPlan(in)
	if err != nil {
		t.Fatalf("rich plan rejected: %v", err)
	}
	covered, err := CanonicalCoveredPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UnmarshalCanonical(covered, PlanDecodeLimits); err != nil {
		t.Fatalf("rich covered plan is not canonical: %v", err)
	}
	if in.PlanDigest == validPlanInput(t).PlanDigest {
		t.Fatal("observed evidence did not change the digest")
	}
	for _, member := range []string{
		`"peeled_oid"`, `"worktree_changes"`, `"file_type":"directory"`, `"file_type":"symlink"`, `"value_redaction"`,
		`"project_id"`, `"instance_bases"`, `"precedence":2`, `"directory_depth":0`, `"winning_rule_source"`,
		`"configured_value_redacted"`, `"lock_reason_redacted"`, `"prunable":false`, `"url_redacted"`,
		`"observed_repository_kind"`, `"development_base"`, `"config_lock_state"`, `"environment_profile"`,
	} {
		if !strings.Contains(string(covered), member) {
			t.Errorf("covered plan lacks %s", member)
		}
	}
	// Each evidence group is digest-covered: changing one observed value
	// changes the digest.
	changed := richObservationInput(t)
	changed.EffectiveConfiguration[0].ValueRedacted = "true"
	snapshot.Observation = mustObservation(t, changed)
	other := in
	other.PreconditionSnapshot = snapshot
	if digest, err := ComputePlanDigest(other); err != nil || digest == in.PlanDigest {
		t.Fatalf("configuration value did not change the digest (%v)", err)
	}
}
