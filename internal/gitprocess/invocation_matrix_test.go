package gitprocess

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"ai-dev/internal/gitmanager"
)

const (
	testDigest  = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	testOID     = gitmanager.ObjectOID("0123456789abcdef0123456789abcdef01234567")
	testOtherID = gitmanager.ObjectOID("89abcdef0123456789abcdef0123456789abcdef")
	testRoot    = "/home/u/.local/state/ai-dev/git"
	testProject = "/tmp/project"
	testHome    = testRoot + "/environment/nonexistent-home-v1"
	testXDG     = testRoot + "/environment/nonexistent-xdg-v1"
)

func testTool(t testing.TB, capabilities ...string) gitmanager.GitToolIdentity {
	t.Helper()
	if len(capabilities) == 0 {
		capabilities = []string{"config-end-of-options", "initial-branch", "object-format", "path-format", "ref-format"}
	}
	tool, err := gitmanager.NewGitToolIdentity(gitmanager.GitToolIdentityInput{
		InvocationMode: gitmanager.InvocationModeLauncher, InvocationPath: "/usr/bin/git", InvocationPathDigest: testDigest,
		InvocationDevice: "1", InvocationInode: "2", EffectiveExecutablePath: "/opt/git/libexec/git-core/git", EffectiveExecutableDigest: testDigest,
		Version: "2.54.0", ExecPath: "/opt/git/libexec/git-core", Capabilities: capabilities,
		EnvironmentSelectors: map[string]string{"DEVELOPER_DIR": "/Applications/Xcode.app/Contents/Developer"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return tool
}

func testEnvironment(t testing.TB) gitmanager.EnvironmentProfile {
	t.Helper()
	root, err := gitmanager.ResolveStateRoot("linux", func(key string) (string, bool) {
		if key == "HOME" {
			return "/home/u", true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := gitmanager.NewEnvironmentProfile(root)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func testContext(t testing.TB) Context {
	t.Helper()
	return Context{Tool: testTool(t), Environment: testEnvironment(t), Directory: []byte(testProject)}
}

// baseline is the versioned override vector, in its fixed order.
var baseline = []string{
	"-c", "core.fsmonitor=false",
	"-c", "core.hooksPath=" + testHome + "/hooks",
	"-c", "credential.helper=",
	"-c", "credential.interactive=never",
	"-c", "core.askPass=",
	"-c", "core.editor=true",
	"-c", "sequence.editor=true",
	"-c", "core.pager=cat",
	"-c", "diff.external=",
	"-c", "commit.gpgSign=false",
	"-c", "tag.gpgSign=false",
	"-c", "core.sshCommand=false",
	"-c", "protocol.allow=never",
}

func withBaseline(args ...string) []string {
	return append(append([]string(nil), baseline...), args...)
}

func TestOperationMatrixRows(t *testing.T) {
	author := gitmanager.GitIdentity{Name: "A U Thor", Email: "author@example.test"}
	committer := gitmanager.GitIdentity{Name: "C O Mitter", Email: "committer@example.test"}
	date := time.Date(2026, 11, 17, 10, 30, 0, 0, time.FixedZone("", 2*3600))
	rows := []struct {
		op       Operation
		params   Parameters
		args     []string
		readOnly bool
		extraEnv []string
	}{
		{OpDiscoverRepository, Parameters{}, withBaseline("rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir", "--show-toplevel"), true, nil},
		{OpHeadSymbolicRef, Parameters{}, withBaseline("symbolic-ref", "-q", "HEAD"), true, nil},
		{OpHeadCommit, Parameters{}, withBaseline("rev-parse", "--verify", "HEAD^{commit}"), true, nil},
		{OpShowRefs, Parameters{}, withBaseline("show-ref", "--head", "-d"), true, nil},
		{OpStatus, Parameters{}, withBaseline("status", "--porcelain=v2", "-z", "--untracked-files=all", "--ignored=matching", "--no-ahead-behind", "--ignore-submodules=all", "--"), true, nil},
		{OpWorktreeDiff, Parameters{}, withBaseline("diff", "--no-ext-diff", "--no-textconv", "--raw", "-z", "--no-renames", "--ignore-submodules=all", "--"), true, nil},
		{OpIndexDiff, Parameters{}, withBaseline("diff", "--cached", "--no-ext-diff", "--no-textconv", "--raw", "-z", "--no-renames", "--ignore-submodules=all", "--"), true, nil},
		{OpCheckRefFormat, Parameters{BranchName: []byte("feature/x")}, withBaseline("check-ref-format", "refs/heads/feature/x"), true, nil},
		{OpObjectExists, Parameters{ObjectOID: testOID}, withBaseline("cat-file", "-e", string(testOID)+"^{commit}"), true, nil},
		{OpObjectType, Parameters{ObjectOID: testOID}, withBaseline("cat-file", "-t", string(testOID)), true, nil},
		{OpObjectContent, Parameters{ObjectOID: testOID}, withBaseline("cat-file", "-p", string(testOID)), true, nil},
		{OpConfigReadLocal, Parameters{}, withBaseline("config", "--local", "--null", "--show-origin", "--show-scope", "--list"), true, nil},
		{OpInit, Parameters{BranchName: []byte("main"), PhysicalRoot: []byte(testProject)},
			withBaseline("init", "--template=", "--object-format=sha1", "--ref-format=files", "--initial-branch=main", "--", testProject), false, nil},
		{OpHashBlob, Parameters{Stdin: []byte("content\n")}, withBaseline("hash-object", "-w", "--stdin", "--no-filters"), false, nil},
		{OpHashTree, Parameters{Stdin: []byte("tree bytes")}, withBaseline("hash-object", "-t", "tree", "-w", "--stdin"), false, nil},
		{OpCommitTree, Parameters{TreeOID: testOID, Author: author, Committer: committer, AuthorDate: date, CommitterDate: date, Stdin: []byte("chore: init\n")},
			withBaseline("-c", "commit.gpgSign=false", "commit-tree", string(testOID)), false,
			[]string{"GIT_AUTHOR_NAME=A U Thor", "GIT_AUTHOR_EMAIL=author@example.test", "GIT_AUTHOR_DATE=@1794904200 +0200",
				"GIT_COMMITTER_NAME=C O Mitter", "GIT_COMMITTER_EMAIL=committer@example.test", "GIT_COMMITTER_DATE=@1794904200 +0200"}},
		{OpUpdateRef, Parameters{RefName: []byte("refs/heads/main"), ObjectOID: testOID},
			withBaseline("-c", "core.logAllRefUpdates=false", "update-ref", "--no-deref", "refs/heads/main", string(testOID), ""), false, nil},
		{OpUpdateRef, Parameters{RefName: []byte("refs/heads/main"), ObjectOID: testOtherID, ExpectedOldOID: testOID},
			withBaseline("-c", "core.logAllRefUpdates=false", "update-ref", "--no-deref", "refs/heads/main", string(testOtherID), string(testOID)), false, nil},
		{OpConfigWriteLocal, Parameters{ConfigKey: "gitflow.branch.master", ConfigValue: "main"},
			withBaseline("config", "--local", "--replace-all", "--", "gitflow.branch.master", "main"), false, nil},
		{OpPreparedIndex, Parameters{TreeOID: testOID, IndexFile: []byte("/state/prepared/index")},
			withBaseline("-c", "core.splitIndex=false", "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false",
				"-c", "index.version=2", "-c", "index.skipHash=false", "read-tree", string(testOID)), false,
			[]string{"GIT_INDEX_FILE=/state/prepared/index"}},
	}
	covered := map[Operation]bool{}
	for _, row := range rows {
		t.Run(string(row.op), func(t *testing.T) {
			invocation, err := Build(row.op, testContext(t), row.params)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(invocation.Args, row.args) {
				t.Fatalf("argv\n got %q\nwant %q", invocation.Args, row.args)
			}
			if invocation.Executable != "/usr/bin/git" || invocation.Directory != testProject {
				t.Fatalf("executable %q directory %q", invocation.Executable, invocation.Directory)
			}
			if string(invocation.Stdin) != string(row.params.Stdin) {
				t.Fatalf("stdin %q", invocation.Stdin)
			}
			if invocation.StdoutLimit != gitmanager.LimitCapturedStreamBytes.Max() || invocation.StderrLimit != gitmanager.LimitCapturedStreamBytes.Max() {
				t.Fatalf("limits %d %d", invocation.StdoutLimit, invocation.StderrLimit)
			}
			want := append(commonEnvironment(), row.extraEnv...)
			if row.readOnly {
				want = append(want, "GIT_OPTIONAL_LOCKS=0")
			} else if row.op != OpInit {
				// R-04: every write names its repository explicitly.
				want = append(want, "GIT_DIR="+testProject+"/.git")
			}
			assertEnvironment(t, invocation.Environment, want)
		})
		covered[row.op] = true
	}
	for _, op := range append(Operations(), OpIdentifyVersion, OpIdentifyExecPath) {
		if !covered[op] && op != OpIdentifyVersion && op != OpIdentifyExecPath {
			t.Errorf("operation %s has no matrix row test", op)
		}
	}
}

// commonEnvironment is the controlled profile every repository row gets:
// nothing inherited, no system/global configuration, no prompts, pagers,
// editors or askpass programs, literal pathspecs, the frozen private
// HOME/XDG leaves, and the frozen toolchain selectors.
func commonEnvironment() []string {
	return []string{
		"DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer",
		"GCM_INTERACTIVE=never",
		"GIT_ASKPASS=",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_EDITOR=true",
		"GIT_LITERAL_PATHSPECS=1",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_PAGER=cat",
		"GIT_SEQUENCE_EDITOR=true",
		"GIT_TERMINAL_PROMPT=0",
		"HOME=" + testHome,
		"PAGER=cat",
		"SSH_ASKPASS=",
		"XDG_CONFIG_HOME=" + testXDG,
	}
}

func assertEnvironment(t *testing.T, got, want []string) {
	t.Helper()
	gotSet := map[string]bool{}
	for _, entry := range got {
		if gotSet[entry] {
			t.Fatalf("duplicate environment entry %q", entry)
		}
		gotSet[entry] = true
	}
	for _, entry := range want {
		if !gotSet[entry] {
			t.Errorf("missing environment entry %q in %q", entry, got)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("environment has %d entries, want exactly %d: %q", len(got), len(want), got)
	}
}

func TestIdentifyRowsCarryNoOverrides(t *testing.T) {
	for op, args := range map[Operation][]string{OpIdentifyVersion: {"--version"}, OpIdentifyExecPath: {"--exec-path"}} {
		invocation, err := BuildIdentify(op, "/usr/bin/git", map[string]string{"DEVELOPER_DIR": "/dev/dir"}, testEnvironment(t), []byte(testProject))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(invocation.Args, args) {
			t.Fatalf("%s argv %q", op, invocation.Args)
		}
		if !contains(invocation.Environment, "DEVELOPER_DIR=/dev/dir") || !contains(invocation.Environment, "HOME="+testHome) {
			t.Fatalf("%s environment %q", op, invocation.Environment)
		}
	}
	if _, err := BuildIdentify(OpStatus, "/usr/bin/git", nil, testEnvironment(t), []byte(testProject)); err == nil {
		t.Fatal("repository row built as an identify row")
	}
	if _, err := BuildIdentify(OpIdentifyVersion, "git", nil, testEnvironment(t), []byte(testProject)); err == nil {
		t.Fatal("relative executable accepted")
	}
	if _, err := Build(OpIdentifyVersion, testContext(t), Parameters{}); err == nil {
		t.Fatal("identify row built through the repository builder")
	}
}

func TestOverridesAreSortedAndNeutralizeFilters(t *testing.T) {
	ctx := testContext(t)
	ctx.Overrides = Overrides{
		FilterDrivers: []string{"lfs", "crypt"},
		Replay: []ConfigReplay{
			{Key: "core.excludesfile", Value: "/home/u/.config/git/ignore"},
			{Key: "core.autocrlf", Value: "false"},
		},
	}
	invocation, err := Build(OpStatus, ctx, Parameters{})
	if err != nil {
		t.Fatal(err)
	}
	want := append(append([]string(nil), baseline...),
		"-c", "filter.crypt.clean=", "-c", "filter.crypt.process=", "-c", "filter.crypt.required=false", "-c", "filter.crypt.smudge=",
		"-c", "filter.lfs.clean=", "-c", "filter.lfs.process=", "-c", "filter.lfs.required=false", "-c", "filter.lfs.smudge=",
		"-c", "core.autocrlf=false", "-c", "core.excludesfile=/home/u/.config/git/ignore",
		"status", "--porcelain=v2", "-z", "--untracked-files=all", "--ignored=matching", "--no-ahead-behind", "--ignore-submodules=all", "--")
	if !reflect.DeepEqual(invocation.Args, want) {
		t.Fatalf("argv\n got %q\nwant %q", invocation.Args, want)
	}
	// The same overrides in another order produce the same argv.
	ctx.Overrides.FilterDrivers = []string{"crypt", "lfs"}
	ctx.Overrides.Replay[0], ctx.Overrides.Replay[1] = ctx.Overrides.Replay[1], ctx.Overrides.Replay[0]
	again, err := Build(OpStatus, ctx, Parameters{})
	if err != nil || !reflect.DeepEqual(again.Args, invocation.Args) {
		t.Fatalf("override order changed argv: %v", err)
	}
}

func TestUnreproducibleConfigurationIsRejected(t *testing.T) {
	for name, overrides := range map[string]Overrides{
		"pager":              {Replay: []ConfigReplay{{Key: "core.pager", Value: "less"}}},
		"alias":              {Replay: []ConfigReplay{{Key: "alias.st", Value: "!sh"}}},
		"credential helper":  {Replay: []ConfigReplay{{Key: "credential.helper", Value: "store"}}},
		"filter command":     {Replay: []ConfigReplay{{Key: "filter.lfs.clean", Value: "git-lfs clean"}}},
		"hooks path":         {Replay: []ConfigReplay{{Key: "core.hookspath", Value: "/tmp/hooks"}}},
		"uncanonical key":    {Replay: []ConfigReplay{{Key: "Core.AutoCRLF", Value: "false"}}},
		"duplicate replay":   {Replay: []ConfigReplay{{Key: "core.autocrlf", Value: "false"}, {Key: "core.autocrlf", Value: "true"}}},
		"newline in value":   {Replay: []ConfigReplay{{Key: "core.eol", Value: "lf\nx"}}},
		"driver with equals": {FilterDrivers: []string{"a=b"}},
		"driver with dot":    {FilterDrivers: []string{"a.b"}},
		"empty driver":       {FilterDrivers: []string{""}},
		"duplicate driver":   {FilterDrivers: []string{"lfs", "lfs"}},
	} {
		ctx := testContext(t)
		ctx.Overrides = overrides
		_, err := Build(OpStatus, ctx, Parameters{})
		var buildErr *BuildError
		if !errors.As(err, &buildErr) || buildErr.Reason != gitmanager.ReasonAttributesIncompatible {
			t.Errorf("%s: %v, want attributes_incompatible", name, err)
		}
	}
}

// git config accepts "--" before key and value only when the frozen
// capability probe confirmed it; otherwise a leading-dash value is refused.
func TestConfigWriteSeparatorCapability(t *testing.T) {
	params := Parameters{ConfigKey: "gitflow.prefix.feature", ConfigValue: "-weird"}
	without := testContext(t)
	without.Tool = testTool(t, "initial-branch", "object-format", "path-format", "ref-format")
	if _, err := Build(OpConfigWriteLocal, without, params); err == nil {
		t.Fatal("leading-dash value written without a confirmed separator")
	}
	ctx := testContext(t)
	invocation, err := Build(OpConfigWriteLocal, ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if want := withBaseline("config", "--local", "--replace-all", "--", "gitflow.prefix.feature", "-weird"); !reflect.DeepEqual(invocation.Args, want) {
		t.Fatalf("argv %q", invocation.Args)
	}
}

func TestParameterValidation(t *testing.T) {
	refusals := []struct {
		name   string
		op     Operation
		params Parameters
		reason gitmanager.Reason
	}{
		{"leading dash branch", OpCheckRefFormat, Parameters{BranchName: []byte("-delete")}, gitmanager.ReasonInvalidStrategy},
		{"previous checkout", OpCheckRefFormat, Parameters{BranchName: []byte("@{-1}")}, gitmanager.ReasonInvalidStrategy},
		{"embedded previous checkout", OpCheckRefFormat, Parameters{BranchName: []byte("a@{-2}b")}, gitmanager.ReasonInvalidStrategy},
		{"newline in branch", OpCheckRefFormat, Parameters{BranchName: []byte("a\nb")}, gitmanager.ReasonInvalidStrategy},
		{"C1 control in branch", OpCheckRefFormat, Parameters{BranchName: []byte("a\u0085b")}, gitmanager.ReasonInvalidStrategy},
		{"NUL in branch", OpCheckRefFormat, Parameters{BranchName: []byte("a\x00b")}, gitmanager.ReasonInvalidStrategy},
		{"non-UTF-8 branch", OpCheckRefFormat, Parameters{BranchName: []byte("a\xffb")}, gitmanager.ReasonInvalidStrategy},
		{"empty branch", OpCheckRefFormat, Parameters{}, gitmanager.ReasonMissingRequiredBranch},
		{"literal HEAD initial branch", OpInit, Parameters{BranchName: []byte("HEAD"), PhysicalRoot: []byte(testProject)}, gitmanager.ReasonInvalidStrategy},
		{"relative init root", OpInit, Parameters{BranchName: []byte("main"), PhysicalRoot: []byte("project")}, gitmanager.ReasonPlanInvalid},
		{"ref outside refs", OpUpdateRef, Parameters{RefName: []byte("HEAD"), ObjectOID: testOID}, gitmanager.ReasonPlanInvalid},
		{"ref with newline", OpUpdateRef, Parameters{RefName: []byte("refs/heads/a\nb"), ObjectOID: testOID}, gitmanager.ReasonPlanInvalid},
		{"missing new OID", OpUpdateRef, Parameters{RefName: []byte("refs/heads/main")}, gitmanager.ReasonPlanInvalid},
		{"malformed OID", OpObjectType, Parameters{ObjectOID: "HEAD"}, gitmanager.ReasonPlanInvalid},
		{"stdin on read row", OpStatus, Parameters{Stdin: []byte("x")}, gitmanager.ReasonPlanInvalid},
		{"parameter on parameterless row", OpShowRefs, Parameters{ObjectOID: testOID}, gitmanager.ReasonPlanInvalid},
		{"commit without identity", OpCommitTree, Parameters{TreeOID: testOID, Stdin: []byte("m\n")}, gitmanager.ReasonIdentityInvalid},
		{"identity with angle bracket", OpCommitTree, Parameters{TreeOID: testOID, Author: gitmanager.GitIdentity{Name: "a<b", Email: "a@x"},
			Committer: gitmanager.GitIdentity{Name: "c", Email: "c@x"}, AuthorDate: time.Unix(1, 0), CommitterDate: time.Unix(1, 0), Stdin: []byte("m\n")}, gitmanager.ReasonIdentityInvalid},
		{"commit without date", OpCommitTree, Parameters{TreeOID: testOID, Author: gitmanager.GitIdentity{Name: "a", Email: "a@x"},
			Committer: gitmanager.GitIdentity{Name: "c", Email: "c@x"}, Stdin: []byte("m\n")}, gitmanager.ReasonIdentityInvalid},
		{"relative index file", OpPreparedIndex, Parameters{TreeOID: testOID, IndexFile: []byte("index")}, gitmanager.ReasonPlanInvalid},
		{"invalid config key", OpConfigWriteLocal, Parameters{ConfigKey: "Core.Bare", ConfigValue: "x"}, gitmanager.ReasonPlanInvalid},
		{"newline config value", OpConfigWriteLocal, Parameters{ConfigKey: "core.bare", ConfigValue: "a\nb"}, gitmanager.ReasonPlanInvalid},
		{"unknown operation", Operation("push"), Parameters{}, gitmanager.ReasonPlanInvalid},
	}
	for _, refusal := range refusals {
		_, err := Build(refusal.op, testContext(t), refusal.params)
		var buildErr *BuildError
		if !errors.As(err, &buildErr) || buildErr.Reason != refusal.reason {
			t.Errorf("%s: %v, want %s", refusal.name, err, refusal.reason)
		}
	}

	missing := testTool(t, "config-end-of-options", "object-format", "path-format")
	ctx := testContext(t)
	ctx.Tool = missing
	_, err := Build(OpInit, ctx, Parameters{BranchName: []byte("main"), PhysicalRoot: []byte(testProject)})
	var buildErr *BuildError
	if !errors.As(err, &buildErr) || buildErr.Reason != gitmanager.ReasonGitCapabilityMissing {
		t.Fatalf("init without ref-format capability: %v", err)
	}
	ctx = testContext(t)
	ctx.Directory = []byte("relative")
	if _, err := Build(OpStatus, ctx, Parameters{}); err == nil {
		t.Fatal("relative working directory accepted")
	}
	ctx = testContext(t)
	ctx.Environment = gitmanager.EnvironmentProfile{}
	if _, err := Build(OpStatus, ctx, Parameters{}); err == nil {
		t.Fatal("missing environment profile accepted")
	}
}

// The bytes Git receives are exactly the frozen profile bytes, including
// non-UTF-8 bytes, so plan, digest, operation IDs and execution agree.
func TestFrozenEnvironmentEquality(t *testing.T) {
	root, err := gitmanager.ResolveStateRoot("linux", func(key string) (string, bool) {
		if key == "HOME" {
			return "/home/\xff", true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := gitmanager.NewEnvironmentProfile(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := testContext(t)
	ctx.Environment = profile
	invocation, err := Build(OpStatus, ctx, Parameters{})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range profile.GitEnvironmentEntries() {
		if !contains(invocation.Environment, entry) {
			t.Fatalf("environment lacks frozen entry %q", entry)
		}
	}
	if !contains(invocation.Args, "core.hooksPath="+string(profile.Home.Bytes())+"/hooks") {
		t.Fatalf("hooks path is not under the frozen absent HOME leaf: %q", invocation.Args)
	}
	for _, entry := range invocation.Environment {
		if strings.HasPrefix(entry, "GIT_DIR=") || strings.HasPrefix(entry, "GIT_CONFIG_PARAMETERS=") || strings.HasPrefix(entry, "PATH=") {
			t.Fatalf("uncontrolled entry %q", entry)
		}
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// Inert values that change ordinary status results can be replayed.
func TestReplayAcceptsStatusAffectingKeys(t *testing.T) {
	for _, key := range []string{"core.filemode", "core.symlinks", "core.trustctime", "core.checkstat", "core.ignorecase", "core.precomposeunicode"} {
		ctx := testContext(t)
		ctx.Overrides.Replay = []ConfigReplay{{Key: key, Value: "false"}}
		invocation, err := Build(OpStatus, ctx, Parameters{})
		if err != nil || !contains(invocation.Args, key+"=false") {
			t.Errorf("%s not replayed: %v", key, err)
		}
	}
}
