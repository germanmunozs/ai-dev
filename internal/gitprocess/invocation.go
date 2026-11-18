package gitprocess

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"ai-dev/internal/gitmanager"
)

// Operation is one row of the closed Git operation matrix (plan P-04).
// There is no way to build an invocation for anything else.
type Operation string

const (
	OpIdentifyVersion    Operation = "identify_version"
	OpIdentifyExecPath   Operation = "identify_exec_path"
	OpDiscoverRepository Operation = "discover_repository"
	OpHeadSymbolicRef    Operation = "head_symbolic_ref"
	OpHeadCommit         Operation = "head_commit"
	OpShowRefs           Operation = "show_refs"
	OpStatus             Operation = "status"
	OpWorktreeDiff       Operation = "worktree_diff"
	OpIndexDiff          Operation = "index_diff"
	OpCheckRefFormat     Operation = "check_ref_format"
	OpObjectExists       Operation = "object_exists"
	OpObjectType         Operation = "object_type"
	OpObjectContent      Operation = "object_content"
	OpConfigReadLocal    Operation = "config_read_local"
	OpInit               Operation = "init"
	OpHashBlob           Operation = "hash_blob"
	OpHashTree           Operation = "hash_tree"
	OpCommitTree         Operation = "commit_tree"
	OpUpdateRef          Operation = "update_ref"
	OpConfigWriteLocal   Operation = "config_write_local"
	OpPreparedIndex      Operation = "prepared_index"
)

// Operations returns every repository row; the two identify rows are built
// by BuildIdentify.
func Operations() []Operation {
	return []Operation{
		OpDiscoverRepository, OpHeadSymbolicRef, OpHeadCommit, OpShowRefs, OpStatus, OpWorktreeDiff, OpIndexDiff,
		OpCheckRefFormat, OpObjectExists, OpObjectType, OpObjectContent, OpConfigReadLocal, OpInit, OpHashBlob,
		OpHashTree, OpCommitTree, OpUpdateRef, OpConfigWriteLocal, OpPreparedIndex,
	}
}

// Capabilities the frozen toolchain must report for the rows that need them.
const (
	CapabilityInitialBranch      = "initial-branch"
	CapabilityObjectFormat       = "object-format"
	CapabilityPathFormat         = "path-format"
	CapabilityRefFormat          = "ref-format"
	CapabilityConfigEndOfOptions = "config-end-of-options"
)

// Parameters carries the typed inputs of one row. Each row requires exactly
// the parameters it uses; any other parameter is refused.
type Parameters struct {
	BranchName     []byte
	RefName        []byte
	ObjectOID      gitmanager.ObjectOID
	ExpectedOldOID gitmanager.ObjectOID
	TreeOID        gitmanager.ObjectOID
	PhysicalRoot   []byte
	IndexFile      []byte
	ConfigKey      string
	ConfigValue    string
	Author         gitmanager.GitIdentity
	Committer      gitmanager.GitIdentity
	AuthorDate     time.Time
	CommitterDate  time.Time
	Stdin          []byte
}

// ConfigReplay is one safely parsed configuration value replayed to Git as
// an exact -c argument so ordinary status and ignore results stay
// equivalent under the private environment.
type ConfigReplay struct {
	Key   string
	Value string
}

// Overrides are the per-plan additions to the baseline override vector.
type Overrides struct {
	FilterDrivers []string
	Replay        []ConfigReplay
}

// Context is the frozen toolchain, private environment, and working
// directory every row runs under.
type Context struct {
	Tool        gitmanager.GitToolIdentity
	Environment gitmanager.EnvironmentProfile
	Directory   []byte
	Overrides   Overrides
}

// BuildError reports why a row cannot be built, with the closed reason the
// caller returns.
type BuildError struct {
	Operation Operation
	Reason    gitmanager.Reason
	Detail    string
}

// ClosedReason is the closed reason the caller returns for this refusal.
func (e *BuildError) ClosedReason() gitmanager.Reason { return e.Reason }

func (e *BuildError) Error() string {
	return fmt.Sprintf("git operation %s cannot be built (%s): %s", e.Operation, e.Reason, e.Detail)
}

func refuse(op Operation, reason gitmanager.Reason, detail string) error {
	return &BuildError{Operation: op, Reason: reason, Detail: detail}
}

// replayableKeys are the only configuration values that may be replayed:
// each changes ordinary status, conversion or ignore results and none can
// name a command. Anything else cannot be reproduced without risking
// configured code, so the plan is refused.
var replayableKeys = map[string]bool{
	"core.attributesfile":         true,
	"core.autocrlf":               true,
	"core.checkroundtripencoding": true,
	"core.checkstat":              true,
	"core.eol":                    true,
	"core.excludesfile":           true,
	"core.filemode":               true,
	"core.ignorecase":             true,
	"core.precomposeunicode":      true,
	"core.safecrlf":               true,
	"core.symlinks":               true,
	"core.trustctime":             true,
}

// baselineOverrides is the versioned override vector inserted before every
// repository subcommand. hooksPath points beneath the absent HOME leaf, so
// it names a directory that never exists.
func baselineOverrides(environment gitmanager.EnvironmentProfile) []string {
	return []string{
		"core.fsmonitor=false",
		"core.hooksPath=" + string(environment.Home.Bytes()) + "/hooks",
		"credential.helper=",
		"credential.interactive=never",
		"core.askPass=",
		"core.editor=true",
		"sequence.editor=true",
		"core.pager=cat",
		"diff.external=",
		"commit.gpgSign=false",
		"tag.gpgSign=false",
		"core.sshCommand=false",
		"protocol.allow=never",
	}
}

// row describes one matrix row: its subcommand arguments, extra -c
// arguments, extra environment, whether it only reads, which capabilities
// it needs, and whether it takes stdin.
type row struct {
	args         []string
	extraConfig  []string
	extraEnv     []string
	readOnly     bool
	capabilities []string
	stdin        bool
}

// Build returns the exact invocation for one repository row.
func Build(op Operation, ctx Context, p Parameters) (gitmanager.GitInvocation, error) {
	if err := ctx.Tool.Validate(); err != nil {
		return gitmanager.GitInvocation{}, refuse(op, gitmanager.ReasonPlanInvalid, "toolchain identity: "+err.Error())
	}
	r, used, err := buildRow(op, ctx, p)
	if err != nil {
		return gitmanager.GitInvocation{}, err
	}
	if extra := unexpectedParameters(p, used); extra != "" {
		return gitmanager.GitInvocation{}, refuse(op, gitmanager.ReasonPlanInvalid, "unexpected parameter "+extra)
	}
	if !r.stdin && p.Stdin != nil {
		return gitmanager.GitInvocation{}, refuse(op, gitmanager.ReasonPlanInvalid, "row takes no stdin")
	}
	for _, capability := range r.capabilities {
		if !hasCapability(ctx.Tool, capability) {
			return gitmanager.GitInvocation{}, refuse(op, gitmanager.ReasonGitCapabilityMissing, "toolchain lacks "+capability)
		}
	}
	overrides, err := overrideArguments(op, ctx)
	if err != nil {
		return gitmanager.GitInvocation{}, err
	}
	var args []string
	for _, value := range overrides {
		args = append(args, "-c", value)
	}
	for _, value := range r.extraConfig {
		args = append(args, "-c", value)
	}
	args = append(args, r.args...)
	environment, err := controlledEnvironment(op, ctx.Tool.EnvironmentSelectors(), ctx.Environment)
	if err != nil {
		return gitmanager.GitInvocation{}, err
	}
	environment = append(environment, r.extraEnv...)
	if r.readOnly {
		environment = append(environment, "GIT_OPTIONAL_LOCKS=0")
	}
	return newInvocation(op, ctx.Tool.InvocationPath, args, ctx.Directory, environment, p.Stdin)
}

// BuildIdentify returns one of the two toolchain identification rows. They
// run before a toolchain identity exists and carry no overrides, but they
// already use the private environment and the frozen selectors.
func BuildIdentify(op Operation, executable string, selectors map[string]string, environment gitmanager.EnvironmentProfile, directory []byte) (gitmanager.GitInvocation, error) {
	var args []string
	switch op {
	case OpIdentifyVersion:
		args = []string{"--version"}
	case OpIdentifyExecPath:
		args = []string{"--exec-path"}
	default:
		return gitmanager.GitInvocation{}, refuse(op, gitmanager.ReasonPlanInvalid, "not an identify row")
	}
	env, err := controlledEnvironment(op, selectors, environment)
	if err != nil {
		return gitmanager.GitInvocation{}, err
	}
	return newInvocation(op, executable, args, directory, append(env, "GIT_OPTIONAL_LOCKS=0"), nil)
}

func newInvocation(op Operation, executable string, args []string, directory []byte, environment []string, stdin []byte) (gitmanager.GitInvocation, error) {
	if !strings.HasPrefix(executable, "/") || strings.ContainsRune(executable, 0) {
		return gitmanager.GitInvocation{}, refuse(op, gitmanager.ReasonPlanInvalid, "executable is not an absolute path")
	}
	if err := absolutePath(directory); err != nil {
		return gitmanager.GitInvocation{}, refuse(op, gitmanager.ReasonPlanInvalid, "working directory: "+err.Error())
	}
	limit := gitmanager.LimitCapturedStreamBytes.Max()
	return gitmanager.GitInvocation{
		Executable:  executable,
		Args:        args,
		Directory:   string(directory),
		Environment: environment,
		Stdin:       append([]byte(nil), stdin...),
		StdoutLimit: limit,
		StderrLimit: limit,
	}, nil
}

func buildRow(op Operation, ctx Context, p Parameters) (row, []string, error) {
	switch op {
	case OpDiscoverRepository:
		return row{args: []string{"rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir", "--show-toplevel"}, readOnly: true,
			capabilities: []string{CapabilityPathFormat}}, nil, nil
	case OpHeadSymbolicRef:
		return row{args: []string{"symbolic-ref", "-q", "HEAD"}, readOnly: true}, nil, nil
	case OpHeadCommit:
		return row{args: []string{"rev-parse", "--verify", "HEAD^{commit}"}, readOnly: true}, nil, nil
	case OpShowRefs:
		return row{args: []string{"show-ref", "--head", "-d"}, readOnly: true}, nil, nil
	case OpStatus:
		// --ignore-submodules=all keeps Git from descending into a
		// submodule, where its own unparsed filters would run (RUN-10).
		return row{args: []string{"status", "--porcelain=v2", "-z", "--untracked-files=all", "--ignored=matching", "--no-ahead-behind", "--ignore-submodules=all", "--"}, readOnly: true}, nil, nil
	case OpWorktreeDiff:
		return row{args: []string{"diff", "--no-ext-diff", "--no-textconv", "--raw", "-z", "--no-renames", "--ignore-submodules=all", "--"}, readOnly: true}, nil, nil
	case OpIndexDiff:
		return row{args: []string{"diff", "--cached", "--no-ext-diff", "--no-textconv", "--raw", "-z", "--no-renames", "--ignore-submodules=all", "--"}, readOnly: true}, nil, nil
	case OpConfigReadLocal:
		return row{args: []string{"config", "--local", "--null", "--show-origin", "--show-scope", "--list"}, readOnly: true}, nil, nil
	case OpCheckRefFormat:
		branch, err := branchName(op, p.BranchName)
		if err != nil {
			return row{}, nil, err
		}
		return row{args: []string{"check-ref-format", "refs/heads/" + branch}, readOnly: true}, []string{"BranchName"}, nil
	case OpObjectExists, OpObjectType, OpObjectContent:
		if err := p.ObjectOID.Validate(); err != nil {
			return row{}, nil, refuse(op, gitmanager.ReasonPlanInvalid, "object ID: "+err.Error())
		}
		args := map[Operation][]string{
			OpObjectExists:  {"cat-file", "-e", string(p.ObjectOID) + "^{commit}"},
			OpObjectType:    {"cat-file", "-t", string(p.ObjectOID)},
			OpObjectContent: {"cat-file", "-p", string(p.ObjectOID)},
		}[op]
		return row{args: args, readOnly: true}, []string{"ObjectOID"}, nil
	case OpInit:
		branch, err := branchName(op, p.BranchName)
		if err != nil {
			return row{}, nil, err
		}
		if branch == "HEAD" {
			return row{}, nil, refuse(op, gitmanager.ReasonInvalidStrategy, "literal initial branch HEAD")
		}
		if err := absolutePath(p.PhysicalRoot); err != nil {
			return row{}, nil, refuse(op, gitmanager.ReasonPlanInvalid, "physical root: "+err.Error())
		}
		return row{
			args:         []string{"init", "--template=", "--object-format=sha1", "--ref-format=files", "--initial-branch=" + branch, "--", string(p.PhysicalRoot)},
			capabilities: []string{CapabilityInitialBranch, CapabilityObjectFormat, CapabilityRefFormat},
		}, []string{"BranchName", "PhysicalRoot"}, nil
	case OpHashBlob:
		return row{args: []string{"hash-object", "-w", "--stdin", "--no-filters"}, stdin: true}, []string{"Stdin"}, nil
	case OpHashTree:
		return row{args: []string{"hash-object", "-t", "tree", "-w", "--stdin"}, stdin: true}, []string{"Stdin"}, nil
	case OpCommitTree:
		if err := p.TreeOID.Validate(); err != nil {
			return row{}, nil, refuse(op, gitmanager.ReasonPlanInvalid, "tree ID: "+err.Error())
		}
		env, err := commitIdentityEnvironment(op, p)
		if err != nil {
			return row{}, nil, err
		}
		return row{args: []string{"commit-tree", string(p.TreeOID)}, extraConfig: []string{"commit.gpgSign=false"}, extraEnv: env, stdin: true},
			[]string{"TreeOID", "Author", "Committer", "AuthorDate", "CommitterDate", "Stdin"}, nil
	case OpUpdateRef:
		if err := refName(p.RefName); err != nil {
			return row{}, nil, refuse(op, gitmanager.ReasonPlanInvalid, err.Error())
		}
		if err := p.ObjectOID.Validate(); err != nil {
			return row{}, nil, refuse(op, gitmanager.ReasonPlanInvalid, "new object ID: "+err.Error())
		}
		used := []string{"RefName", "ObjectOID"}
		if p.ExpectedOldOID != "" {
			if err := p.ExpectedOldOID.Validate(); err != nil {
				return row{}, nil, refuse(op, gitmanager.ReasonPlanInvalid, "expected old object ID: "+err.Error())
			}
			used = append(used, "ExpectedOldOID")
		}
		// An empty expected-old argument makes the update require that the
		// ref does not exist yet.
		return row{args: []string{"update-ref", "--no-deref", string(p.RefName), string(p.ObjectOID), string(p.ExpectedOldOID)},
			extraConfig: []string{"core.logAllRefUpdates=false"}}, used, nil
	case OpConfigWriteLocal:
		if err := gitmanager.ValidateConfigKey(p.ConfigKey); err != nil {
			return row{}, nil, refuse(op, gitmanager.ReasonPlanInvalid, err.Error())
		}
		if !configValueSafe(p.ConfigValue) {
			return row{}, nil, refuse(op, gitmanager.ReasonPlanInvalid, "configuration value with NUL or newline")
		}
		args := []string{"config", "--local", "--replace-all"}
		if hasCapability(ctx.Tool, CapabilityConfigEndOfOptions) {
			args = append(args, "--")
		} else if strings.HasPrefix(p.ConfigValue, "-") {
			return row{}, nil, refuse(op, gitmanager.ReasonGitCapabilityMissing, "leading-dash value without a confirmed option separator")
		}
		return row{args: append(args, p.ConfigKey, p.ConfigValue)}, []string{"ConfigKey", "ConfigValue"}, nil
	case OpPreparedIndex:
		if err := p.TreeOID.Validate(); err != nil {
			return row{}, nil, refuse(op, gitmanager.ReasonPlanInvalid, "tree ID: "+err.Error())
		}
		if err := absolutePath(p.IndexFile); err != nil {
			return row{}, nil, refuse(op, gitmanager.ReasonPlanInvalid, "prepared index: "+err.Error())
		}
		return row{
			args:        []string{"read-tree", string(p.TreeOID)},
			extraConfig: []string{"core.splitIndex=false", "core.fsmonitor=false", "core.untrackedCache=false", "index.version=2", "index.skipHash=false"},
			extraEnv:    []string{"GIT_INDEX_FILE=" + string(p.IndexFile)},
		}, []string{"TreeOID", "IndexFile"}, nil
	default:
		return row{}, nil, refuse(op, gitmanager.ReasonPlanInvalid, "not a repository row of the operation matrix")
	}
}

// unexpectedParameters names the first parameter that is set although the
// row does not use it.
func unexpectedParameters(p Parameters, used []string) string {
	set := map[string]bool{
		"BranchName":     p.BranchName != nil,
		"RefName":        p.RefName != nil,
		"ObjectOID":      p.ObjectOID != "",
		"ExpectedOldOID": p.ExpectedOldOID != "",
		"TreeOID":        p.TreeOID != "",
		"PhysicalRoot":   p.PhysicalRoot != nil,
		"IndexFile":      p.IndexFile != nil,
		"ConfigKey":      p.ConfigKey != "",
		"ConfigValue":    p.ConfigValue != "",
		"Author":         p.Author != (gitmanager.GitIdentity{}),
		"Committer":      p.Committer != (gitmanager.GitIdentity{}),
		"AuthorDate":     !p.AuthorDate.IsZero(),
		"CommitterDate":  !p.CommitterDate.IsZero(),
	}
	allowed := map[string]bool{}
	for _, name := range used {
		allowed[name] = true
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if set[name] && !allowed[name] {
			return name
		}
	}
	return ""
}

// branchName validates a user-controlled branch before it can reach argv:
// no controls, no leading dash, no previous-checkout shorthand, and bytes
// that round-trip as UTF-8. Git's check-ref-format row validates the rest.
func branchName(op Operation, name []byte) (string, error) {
	if len(name) == 0 {
		return "", refuse(op, gitmanager.ReasonMissingRequiredBranch, "branch name required")
	}
	if !utf8.Valid(name) {
		return "", refuse(op, gitmanager.ReasonInvalidStrategy, "branch name is not valid UTF-8")
	}
	value := string(name)
	for _, r := range value {
		if unicode.IsControl(r) {
			return "", refuse(op, gitmanager.ReasonInvalidStrategy, "branch name contains a control character")
		}
	}
	if strings.HasPrefix(value, "-") || strings.Contains(value, "@{-") {
		return "", refuse(op, gitmanager.ReasonInvalidStrategy, "branch name could be read as an option or previous checkout")
	}
	return value, nil
}

func refName(name []byte) error {
	if !bytes.HasPrefix(name, []byte("refs/")) || !utf8.Valid(name) {
		return fmt.Errorf("ref %q is not a full UTF-8 name below refs/", name)
	}
	for _, r := range string(name) {
		if unicode.IsControl(r) {
			return fmt.Errorf("ref contains a control character")
		}
	}
	return nil
}

func absolutePath(path []byte) error {
	if len(path) == 0 || path[0] != '/' {
		return fmt.Errorf("not an absolute path")
	}
	if bytes.IndexByte(path, 0) >= 0 {
		return fmt.Errorf("path contains NUL")
	}
	return nil
}

func configValueSafe(value string) bool {
	return !strings.ContainsAny(value, "\x00\n")
}

// commitIdentityEnvironment freezes author and committer through the
// environment, so commit-tree never reads identity or date from config.
func commitIdentityEnvironment(op Operation, p Parameters) ([]string, error) {
	for _, identity := range []gitmanager.GitIdentity{p.Author, p.Committer} {
		if identity.Validate() != nil || strings.ContainsAny(identity.Name+identity.Email, "<>\n\x00") {
			return nil, refuse(op, gitmanager.ReasonIdentityInvalid, "author or committer identity")
		}
	}
	if p.AuthorDate.IsZero() || p.CommitterDate.IsZero() {
		return nil, refuse(op, gitmanager.ReasonIdentityInvalid, "author or committer date")
	}
	date := func(t time.Time) string { return "@" + strconv.FormatInt(t.Unix(), 10) + " " + t.Format("-0700") }
	return []string{
		"GIT_AUTHOR_NAME=" + p.Author.Name, "GIT_AUTHOR_EMAIL=" + p.Author.Email, "GIT_AUTHOR_DATE=" + date(p.AuthorDate),
		"GIT_COMMITTER_NAME=" + p.Committer.Name, "GIT_COMMITTER_EMAIL=" + p.Committer.Email, "GIT_COMMITTER_DATE=" + date(p.CommitterDate),
	}, nil
}

// overrideArguments returns the baseline vector, then sorted filter
// neutralization, then sorted configuration replay.
func overrideArguments(op Operation, ctx Context) ([]string, error) {
	if err := ctx.Environment.Validate(); err != nil {
		return nil, refuse(op, gitmanager.ReasonPlanInvalid, "environment profile: "+err.Error())
	}
	args := baselineOverrides(ctx.Environment)
	var filters []string
	seenDriver := map[string]bool{}
	for _, driver := range ctx.Overrides.FilterDrivers {
		if driver == "" || seenDriver[driver] || strings.ContainsAny(driver, "=.\n\x00") || !utf8.ValidString(driver) {
			return nil, refuse(op, gitmanager.ReasonAttributesIncompatible, fmt.Sprintf("filter driver %q cannot be neutralized", driver))
		}
		seenDriver[driver] = true
		for _, member := range []string{"clean=", "smudge=", "process=", "required=false"} {
			filters = append(filters, "filter."+driver+"."+member)
		}
	}
	sort.Strings(filters)
	replay := append([]ConfigReplay(nil), ctx.Overrides.Replay...)
	seenKey := map[string]bool{}
	for _, entry := range replay {
		if !replayableKeys[entry.Key] || seenKey[entry.Key] || !configValueSafe(entry.Value) {
			return nil, refuse(op, gitmanager.ReasonAttributesIncompatible, fmt.Sprintf("configuration %q cannot be replayed safely", entry.Key))
		}
		seenKey[entry.Key] = true
	}
	sort.Slice(replay, func(i, j int) bool { return replay[i].Key < replay[j].Key })
	args = append(args, filters...)
	for _, entry := range replay {
		args = append(args, entry.Key+"="+entry.Value)
	}
	return args, nil
}

// controlledEnvironment builds the complete subprocess environment. Nothing
// is inherited: every entry is listed here, sorted, so ambient GIT_*,
// PATH, HOME and XDG values never reach Git.
func controlledEnvironment(op Operation, selectors map[string]string, environment gitmanager.EnvironmentProfile) ([]string, error) {
	if err := environment.Validate(); err != nil {
		return nil, refuse(op, gitmanager.ReasonPlanInvalid, "environment profile: "+err.Error())
	}
	entries := []string{
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
		"PAGER=cat",
		"SSH_ASKPASS=",
	}
	entries = append(entries, environment.GitEnvironmentEntries()...)
	for name, value := range selectors {
		if name != "DEVELOPER_DIR" || strings.ContainsAny(value, "\x00\n") {
			return nil, refuse(op, gitmanager.ReasonPlanInvalid, fmt.Sprintf("environment selector %q is not allowed", name))
		}
		entries = append(entries, name+"="+value)
	}
	sort.Strings(entries)
	return entries, nil
}

func hasCapability(tool gitmanager.GitToolIdentity, capability string) bool {
	for _, value := range tool.Capabilities() {
		if value == capability {
			return true
		}
	}
	return false
}
