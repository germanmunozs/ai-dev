package gitmanager

import (
	"bytes"
	"context"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// GitInspectionSchema is the schema of a GitInspection.
const GitInspectionSchema = "ai-dev.git-inspection/v1"

// UserContext is the real user context the in-process evaluators read.
// It is never passed to a Git subprocess.
type UserContext struct {
	Home                 []byte
	XDGConfigHome        []byte
	EnvironmentOverrides bool
}

// InspectRequest is everything one inspection observes from. It carries no
// mutation store and no authorization (inspection contract §1).
type InspectRequest struct {
	ProjectDir   string
	WorkingDir   string
	Tool         GitToolIdentity
	Environment  EnvironmentProfile
	User         UserContext
	InspectionID InspectionID
	InspectedAt  time.Time
	// Candidates are literal repository-relative paths an initial commit
	// names; their identities, ancestors and ignore status are frozen.
	Candidates [][]byte
}

// InspectorDependencies are the read-only ports an inspection uses.
type InspectorDependencies struct {
	Git               GitReader
	ObserveFilesystem func(path string) (FilesystemObservation, error)
	// layout replaces the toolchain layout table in tests.
	layout func(GitToolIdentity) (toolchainLayout, bool)
}

// inspection accumulates one inspection.
type inspection struct {
	ctx               context.Context
	deps              InspectorDependencies
	req               InspectRequest
	root              string
	location          repositoryLocation
	warnings          []string
	incompatibilities []Incompatibility
	obs               RepositoryObservationInput
	remotes           []RemoteRedacted
	remotesState      ObservationState
}

func (s *inspection) warn(text string) { s.warnings = append(s.warnings, text) }
func (s *inspection) incompatible(reason Reason, subject string) {
	s.incompatibilities = append(s.incompatibilities, Incompatibility{Reason: reason, Subject: subject})
}

// Inspect produces one complete read-only GitInspection (F008-FR-011–027).
// Configuration, attributes and ignores are evaluated in process before
// any Git row runs, and Git runs only in the project's own repository and
// only after that evaluation succeeded. Nothing is written. The only error
// is an unavailable project identity; every other finding is an explicit
// observation state, warning or incompatibility.
func Inspect(ctx context.Context, deps InspectorDependencies, req InspectRequest) (GitInspection, error) {
	root, err := ResolveProjectRoot(req.ProjectDir, req.WorkingDir)
	if err != nil {
		return GitInspection{}, err
	}
	s := &inspection{ctx: ctx, deps: deps, req: req, root: string(root.Identity.ReversiblePathBytes())}
	if root.Aliased {
		s.warn("project path is an alias of its physical directory")
	}
	s.location = locateRepository([]byte(s.root))
	s.classifyKind()
	s.checkOwnership(root)

	budget, _ := NewBudget(LimitAggregateSourceBytes)
	resolveLayout := resolveToolchainLayout
	if deps.layout != nil {
		resolveLayout = deps.layout
	}
	layout, layoutKnown := resolveLayout(req.Tool)
	if !layoutKnown {
		s.incompatible(ReasonAttributesIncompatible, "the Git toolchain layout is not a verified installation")
	}
	config := s.evaluateConfig(layout, budget)
	ignore, attributes := s.evaluators(config, layout, budget)
	s.observeRepository(config, ignore, attributes, layoutKnown)
	s.observeCandidates(ignore, attributes)
	s.observeSources(config, ignore, attributes)
	s.observeProjectConfig()
	s.observeFilesystems()
	s.observeRemotes(config)

	observation, err := NewRepositoryObservation(s.obs)
	if err != nil {
		return GitInspection{}, err
	}
	return NewGitInspection(GitInspectionInput{
		SchemaVersion: GitInspectionSchema, InspectionID: req.InspectionID, InspectedAt: req.InspectedAt,
		ProjectIdentity: root.Identity, Observation: observation, RemotesObservationState: s.remotesState,
		RemotesRedacted: s.remotes, ToolIdentity: req.Tool, Warnings: s.warnings, Incompatibilities: s.incompatibilities,
	})
}

// ownsRepository reports kinds whose repository belongs to the project.
func (s *inspection) ownsRepository() bool {
	switch s.location.Kind {
	case RepositoryKindNormal, RepositoryKindNested, RepositoryKindBare, RepositoryKindLinkedWorktree, RepositoryKindSubmodule:
		return true
	}
	return false
}

func (s *inspection) hasWorktree() bool {
	return s.ownsRepository() && s.location.Kind != RepositoryKindBare
}

// classifyKind records the incompatibilities the repository kind implies
// for planning (F008-FR-022, 023, 138); inspection itself still succeeds.
func (s *inspection) classifyKind() {
	switch s.location.Kind {
	case RepositoryKindAncestor:
		s.incompatible(ReasonAncestorRepository, "the project is inside another repository")
	case RepositoryKindLinkedWorktree, RepositoryKindSubmodule, RepositoryKindBare:
		s.incompatible(ReasonRepositoryKindUnsupported, string(s.location.Kind)+" repositories are inspected but not planned")
	case RepositoryKindInaccessible, RepositoryKindAmbiguous:
		s.incompatible(ReasonRepositoryStateIncompatible, s.location.Detail)
	case RepositoryKindNested:
		s.warn("the project repository is nested inside another repository")
	}
}

func (s *inspection) checkOwnership(root ProjectRoot) {
	gitOwner, hasGitDir := uint32(0), false
	if s.ownsRepository() {
		if owner, err := fileOwner(string(s.location.GitDir)); err == nil {
			gitOwner, hasGitDir = owner, true
		}
	}
	s.incompatibilities = append(s.incompatibilities, ownershipIncompatibilities(root.OwnerUID, gitOwner, currentEUID(), hasGitDir)...)
}

func (s *inspection) evaluateConfig(layout toolchainLayout, budget *Budget) ConfigEvaluation {
	in := ConfigInputs{
		Home: s.req.User.Home, XDGConfigHome: s.req.User.XDGConfigHome, ToolchainSources: layout.ConfigFiles,
		EnvironmentOverrides: s.req.User.EnvironmentOverrides, Budget: budget,
	}
	switch {
	case s.ownsRepository():
		in.GitDir, in.CommonDir = s.location.GitDir, s.location.CommonDir
		in.Branch = headBranch(string(s.location.GitDir))
	case s.location.Kind == RepositoryKindAbsent:
		// Conditional includes are evaluated for the Git directory that
		// initialization would create.
		in.GitDir = []byte(s.root + "/.git")
	}
	config := EvaluateConfig(in)
	s.incompatibilities = append(s.incompatibilities, config.Incompatibilities...)
	for _, file := range layout.MustBeAbsent {
		if pathExists(string(file)) {
			s.incompatible(ReasonAttributesIncompatible, "a toolchain file of unverified use is present")
		}
	}
	s.obs.ConfigurationObservationState = config.State
	if config.State == ObservationObserved {
		s.obs.EffectiveConfiguration = config.Entries
	}
	return config
}

func (s *inspection) ignoreCase(config ConfigEvaluation) bool {
	value, hasValue, found := config.Last("core.ignorecase")
	return found && (!hasValue || configTrue(value))
}

func (s *inspection) evaluators(config ConfigEvaluation, layout toolchainLayout, budget *Budget) (*IgnoreEvaluator, *AttributeEvaluator) {
	ignoreInputs := IgnoreInputs{Root: []byte(s.root), IgnoreCase: s.ignoreCase(config)}
	attributeInputs := AttributeInputs{Root: []byte(s.root), SystemFiles: layout.AttributeFiles, IgnoreCase: ignoreInputs.IgnoreCase}
	if s.ownsRepository() {
		ignoreInputs.GitDir, attributeInputs.GitDir = s.location.GitDir, s.location.GitDir
	}
	if excludes, ok := ExcludesFilePath(config, s.req.User.Home, s.req.User.XDGConfigHome); ok {
		ignoreInputs.ExcludesFile = excludes
	} else {
		s.incompatible(ReasonAttributesIncompatible, "core.excludesFile cannot be resolved as Git resolves it")
	}
	if value, hasValue, found := config.Last("core.attributesfile"); found {
		switch {
		case hasValue && strings.HasPrefix(value, "~/") && len(s.req.User.Home) > 0:
			attributeInputs.GlobalFile = []byte(string(s.req.User.Home) + value[1:])
		case hasValue && strings.HasPrefix(value, "/"):
			attributeInputs.GlobalFile = []byte(value)
		default:
			s.incompatible(ReasonAttributesIncompatible, "core.attributesFile cannot be resolved as Git resolves it")
		}
		attributeInputs.GlobalExplicit = true
	} else if xdg := xdgConfigHome(s.req.User); xdg != "" {
		attributeInputs.GlobalFile = []byte(xdg + "/git/attributes")
	}
	ignore := NewIgnoreEvaluator(ignoreInputs, budget)
	attributes := NewAttributeEvaluator(attributeInputs, budget)
	// The root sources apply to every path; deeper ones are read when a
	// path below them is evaluated.
	ignore.dirList(nil)
	attributes.dirList(nil)
	return ignore, attributes
}

func xdgConfigHome(user UserContext) string {
	if len(user.XDGConfigHome) > 0 {
		return string(user.XDGConfigHome)
	}
	if len(user.Home) > 0 {
		return string(user.Home) + "/.config"
	}
	return ""
}

// observeRepository fills the repository groups. Git rows run only for the
// project's own repository and only when the configuration was evaluated
// and the toolchain layout is known.
func (s *inspection) observeRepository(config ConfigEvaluation, ignore *IgnoreEvaluator, attributes *AttributeEvaluator, layoutKnown bool) {
	s.obs.RepositoryKind = s.location.Kind
	s.obs.SequencerState, s.obs.ConfigLockState = SequencerNone, LockFileAbsent
	s.obs.Head = HeadObservation{State: HeadStateUnborn}
	s.obs.RefsObservationState = ObservationNotApplicable
	s.obs.Index = IndexObservation{Presence: IndexAbsent, LockState: LockFileAbsent}
	s.obs.WorktreeObservationState = ObservationNotApplicable
	s.obs.LinkedWorktreesObservationState = ObservationNotApplicable
	s.obs.IgnoreEvaluationsObservationState = ObservationObservedEmpty
	if !s.ownsRepository() {
		if s.location.Kind != RepositoryKindAbsent {
			s.obs.Head = HeadObservation{State: HeadStateInvalid}
			s.obs.RefsObservationState = ObservationNotObserved
			s.obs.WorktreeObservationState = ObservationNotObserved
			s.obs.LinkedWorktreesObservationState = ObservationNotObserved
			s.obs.Index = IndexObservation{Presence: IndexUnreadable, LockState: LockFileUnobservable}
			s.obs.ConfigLockState = LockFileUnobservable
			s.warn("the project's repository is not its own, so it was not observed")
		}
		s.obs.SubmodulesObservationState = ObservationNotApplicable
		if s.location.Kind != RepositoryKindAbsent {
			s.obs.SubmodulesObservationState = ObservationNotObserved
		}
		return
	}
	gitDir, commonDir := string(s.location.GitDir), string(s.location.CommonDir)
	s.obs.GitDir, s.obs.CommonDir = NewObservedPath(s.location.GitDir), NewObservedPath(s.location.CommonDir)
	s.obs.ObjectFormat, s.obs.RefFormat = ObjectFormatSHA1, RefFormatFiles
	if value, _, found := config.Last("extensions.objectformat"); found {
		s.obs.ObjectFormat = ObjectFormat(strings.ToLower(value))
	}
	if value, _, found := config.Last("extensions.refstorage"); found {
		s.obs.RefFormat = RefFormat(strings.ToLower(value))
	}
	if !s.obs.ObjectFormat.Valid() || !s.obs.RefFormat.Valid() {
		s.incompatible(ReasonRepositoryStateIncompatible, "unknown repository object or ref format")
		s.obs.ObjectFormat, s.obs.RefFormat = ObjectFormatSHA1, RefFormatFiles
	}
	s.obs.SequencerState = sequencerState(gitDir)
	s.obs.ConfigLockState = lockState(commonDir + "/config.lock")
	hashSize := 20
	if s.obs.ObjectFormat == ObjectFormatSHA256 {
		hashSize = 32
	}
	indexIn, indexIssue := readIndex(gitDir, hashSize)
	if indexIssue != "" {
		s.warn(indexIssue)
	}
	if worktrees, ok := linkedWorktrees(commonDir); ok {
		s.obs.LinkedWorktrees = worktrees
		s.obs.LinkedWorktreesObservationState = observedState(len(worktrees))
	} else {
		s.obs.LinkedWorktreesObservationState = ObservationError
		s.warn("linked worktrees could not be observed")
	}
	if pathExists(s.root + "/.gitmodules") {
		s.obs.SubmodulesObservationState = ObservationNotObserved
		s.warn("submodule declarations are present; their recorded commits are not read by the inspection rows")
	} else {
		s.obs.SubmodulesObservationState = ObservationObservedEmpty
	}

	runnable := s.deps.Git != nil && config.State != ObservationError && layoutKnown && ignore.Usable() && attributes.Usable()
	if !runnable {
		s.obs.Head = HeadObservation{State: HeadStateInvalid}
		s.obs.RefsObservationState = ObservationNotObserved
		if s.hasWorktree() {
			s.obs.WorktreeObservationState = ObservationNotObserved
		}
		s.obs.Index, _ = NewIndexObservation(indexIn)
		s.warn("Git was not run because the configuration could not be proven inert")
		return
	}
	request := GitReadRequest{
		Tool: s.req.Tool, Environment: s.req.Environment, Directory: []byte(s.root),
		FilterDrivers: filterDrivers(config), Replay: s.replay(config),
	}
	if s.location.Kind == RepositoryKindBare {
		request.Directory = s.location.GitDir
	}
	s.crossCheckDiscovery(request)
	refs, refsOK := s.readRefs(request)
	s.readHead(request, refs, refsOK)
	if s.hasWorktree() {
		indexIn.Changes = s.readStatus(request, ignore)
	}
	index, err := NewIndexObservation(indexIn)
	if err != nil {
		index, _ = NewIndexObservation(IndexObservationInput{Presence: IndexUnreadable, LockState: indexIn.LockState})
		s.warn("index changes could not be reconciled with the index")
	}
	s.obs.Index = index
	_ = attributes
}

func observedState(n int) ObservationState {
	if n == 0 {
		return ObservationObservedEmpty
	}
	return ObservationObserved
}

// filterDrivers lists every configured filter driver, so each is
// neutralized for the read rows.
func filterDrivers(config ConfigEvaluation) []string {
	seen := map[string]bool{}
	var drivers []string
	for _, record := range config.values {
		key := record.line.key
		if !strings.HasPrefix(key, "filter.") {
			continue
		}
		last := strings.LastIndexByte(key, '.')
		if driver := key[len("filter."):last]; last > len("filter.") && !seen[driver] {
			seen[driver] = true
			drivers = append(drivers, driver)
		}
	}
	sort.Strings(drivers)
	return drivers
}

// replayKeys are the values that change ordinary status, conversion or
// ignore results; those set outside the repository are hidden from Git by
// the private environment and are replayed exactly.
var replayKeys = []string{
	"core.attributesfile", "core.autocrlf", "core.checkroundtripencoding", "core.checkstat", "core.eol",
	"core.excludesfile", "core.filemode", "core.ignorecase", "core.precomposeunicode", "core.safecrlf",
	"core.symlinks", "core.trustctime",
}

func (s *inspection) replay(config ConfigEvaluation) []GitConfigReplay {
	var replay []GitConfigReplay
	for _, key := range replayKeys {
		for i := len(config.values) - 1; i >= 0; i-- {
			record := config.values[i]
			if record.line.key != key {
				continue
			}
			if record.scope != ConfigScopeLocal && record.scope != ConfigScopeWorktree {
				value := record.line.value
				if !record.line.hasValue {
					value = "true"
				}
				replay = append(replay, GitConfigReplay{Key: key, Value: value})
			}
			break
		}
	}
	// Defaults that live under the hidden HOME/XDG are replayed explicitly.
	has := func(key string) bool { _, _, found := config.Last(key); return found }
	if xdg := xdgConfigHome(s.req.User); xdg != "" {
		if !has("core.excludesfile") && pathExists(xdg+"/git/ignore") {
			replay = append(replay, GitConfigReplay{Key: "core.excludesfile", Value: xdg + "/git/ignore"})
		}
		if !has("core.attributesfile") && pathExists(xdg+"/git/attributes") {
			replay = append(replay, GitConfigReplay{Key: "core.attributesfile", Value: xdg + "/git/attributes"})
		}
	}
	for i := range replay {
		if replay[i].Key == "core.excludesfile" || replay[i].Key == "core.attributesfile" {
			if strings.HasPrefix(replay[i].Value, "~/") {
				replay[i].Value = string(s.req.User.Home) + replay[i].Value[1:]
			}
		}
	}
	return replay
}

func (s *inspection) read(request GitReadRequest, row GitReadRow) (GitRunResult, bool) {
	request.Row = row
	result, err := s.deps.Git.Read(s.ctx, request)
	if err != nil {
		s.warn("Git row " + string(row) + " could not run")
		if reason := buildReason(err); reason != "" {
			s.incompatible(reason, "Git row "+string(row)+" cannot run safely")
		}
		return GitRunResult{}, false
	}
	return result, true
}

// buildReason extracts a closed reason from a row-building error.
func buildReason(err error) Reason {
	type reasoned interface{ ClosedReason() Reason }
	if r, ok := err.(reasoned); ok {
		return r.ClosedReason()
	}
	return ""
}

// crossCheckDiscovery compares Git's repository location with the
// in-process one; a disagreement is never resolved by trusting either.
func (s *inspection) crossCheckDiscovery(request GitReadRequest) {
	result, ok := s.read(request, GitReadDiscover)
	if !ok || result.ExitCode != 0 {
		s.incompatible(ReasonRepositoryStateIncompatible, "Git could not discover the repository")
		return
	}
	lines := strings.Split(strings.TrimSuffix(string(result.Stdout), "\n"), "\n")
	if len(lines) < 2 {
		s.incompatible(ReasonRepositoryStateIncompatible, "Git reported an unexpected repository location")
		return
	}
	physical := func(p string) string {
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			return resolved
		}
		return p
	}
	if physical(lines[0]) != string(s.location.GitDir) || physical(lines[1]) != string(s.location.CommonDir) ||
		(s.hasWorktree() && (len(lines) < 3 || physical(lines[2]) != s.root)) {
		s.incompatible(ReasonRepositoryStateIncompatible, "Git and the inspector disagree on the repository location")
	}
}

func (s *inspection) readRefs(request GitReadRequest) ([]RefObservation, bool) {
	result, ok := s.read(request, GitReadShowRefs)
	if !ok || (result.ExitCode != 0 && result.ExitCode != 1) {
		s.obs.RefsObservationState = ObservationError
		s.warn("refs could not be read")
		return nil, false
	}
	refs, err := parseShowRefs(result.Stdout, s.obs.ObjectFormat)
	if err != nil {
		s.obs.RefsObservationState = ObservationError
		s.warn("refs output could not be parsed")
		return nil, false
	}
	s.obs.Refs, s.obs.RefsObservationState = refs, observedState(len(refs))
	return refs, true
}

// parseShowRefs parses "show-ref --head -d": "<oid> <name>" lines, with
// peeled "<name>^{}" lines attached to their ref; HEAD is excluded.
func parseShowRefs(output []byte, format ObjectFormat) ([]RefObservation, error) {
	byName := map[string]*RefObservation{}
	var order []string
	for _, line := range strings.Split(strings.TrimSuffix(string(output), "\n"), "\n") {
		if line == "" {
			continue
		}
		oid, name, found := strings.Cut(line, " ")
		if !found || ObjectOID(oid).ValidateFor(format) != nil {
			return nil, invalid("show-ref line")
		}
		if name == "HEAD" {
			continue
		}
		if base, peeled := strings.CutSuffix(name, "^{}"); peeled {
			ref, ok := byName[base]
			if !ok {
				return nil, invalid("peeled ref without ref")
			}
			ref.PeeledOID = ObjectOID(oid)
			continue
		}
		if _, dup := byName[name]; dup {
			return nil, invalid("duplicate ref")
		}
		byName[name] = &RefObservation{Name: NewObservedRef([]byte(name)), TargetOID: ObjectOID(oid)}
		order = append(order, name)
	}
	sort.Strings(order)
	refs := make([]RefObservation, 0, len(order))
	for _, name := range order {
		if err := byName[name].Validate(); err != nil {
			return nil, err
		}
		refs = append(refs, *byName[name])
	}
	return refs, nil
}

// readHead classifies HEAD as unborn, attached, detached or invalid
// (F008-FR-162) from symbolic-ref and the commit HEAD resolves to.
func (s *inspection) readHead(request GitReadRequest, refs []RefObservation, refsOK bool) {
	invalidHead := func(why string) {
		s.obs.Head = HeadObservation{State: HeadStateInvalid}
		s.warn(why)
	}
	symbolic, ok := s.read(request, GitReadHeadSymbolicRef)
	if !ok {
		invalidHead("HEAD could not be read")
		return
	}
	commit, ok := s.read(request, GitReadHeadCommit)
	if !ok {
		invalidHead("HEAD could not be resolved")
		return
	}
	oid := ObjectOID(strings.TrimSuffix(string(commit.Stdout), "\n"))
	resolved := commit.ExitCode == 0 && oid.ValidateFor(s.obs.ObjectFormat) == nil
	history := resolved || (refsOK && len(refs) > 0)
	switch {
	case symbolic.ExitCode == 0:
		ref := NewObservedRef(bytes.TrimSuffix(symbolic.Stdout, []byte("\n")))
		if ref.Validate() != nil {
			invalidHead("HEAD names an invalid ref")
			return
		}
		if resolved {
			s.obs.Head = HeadObservation{State: HeadStateAttached, SymbolicRef: ref, CommitOID: oid, HistoryPresent: true}
		} else {
			s.obs.Head = HeadObservation{State: HeadStateUnborn, SymbolicRef: ref, HistoryPresent: history}
		}
	case symbolic.ExitCode == 1 && resolved:
		s.obs.Head = HeadObservation{State: HeadStateDetached, CommitOID: oid, HistoryPresent: true}
	default:
		invalidHead("HEAD is neither a branch nor a commit")
	}
}

// statusEntry is one porcelain v2 record.
type statusEntry struct {
	kind   byte
	x, y   byte
	mH, mI string
	mW     string
	path   []byte
}

// parseStatus parses "status --porcelain=v2 -z" output.
func parseStatus(output []byte) ([]statusEntry, error) {
	var entries []statusEntry
	records := bytes.Split(output, []byte{0})
	for i := 0; i < len(records); i++ {
		record := records[i]
		if len(record) == 0 {
			continue
		}
		fields := func(n int) ([][]byte, []byte, error) {
			parts := bytes.SplitN(record, []byte(" "), n+1)
			if len(parts) != n+1 || len(parts[n]) == 0 {
				return nil, nil, invalid("status record")
			}
			return parts[:n], parts[n], nil
		}
		switch record[0] {
		case '?', '!':
			if len(record) < 3 || record[1] != ' ' {
				return nil, invalid("status record")
			}
			entries = append(entries, statusEntry{kind: record[0], path: record[2:]})
		case '1', '2', 'u':
			n := map[byte]int{'1': 8, '2': 9, 'u': 10}[record[0]]
			parts, path, err := fields(n)
			if err != nil || len(parts[1]) != 2 {
				return nil, invalid("status record")
			}
			entry := statusEntry{kind: record[0], x: parts[1][0], y: parts[1][1], path: path}
			if record[0] != 'u' {
				entry.mH, entry.mI, entry.mW = string(parts[3]), string(parts[4]), string(parts[5])
			}
			entries = append(entries, entry)
			if record[0] == '2' {
				i++ // the original path of a rename
			}
		case '#':
		default:
			return nil, invalid("status record")
		}
	}
	return entries, nil
}

// readStatus records index and worktree changes and checks the in-process
// ignore classification against Git's for every untracked and ignored
// path.
func (s *inspection) readStatus(request GitReadRequest, ignore *IgnoreEvaluator) []IndexChange {
	result, ok := s.read(request, GitReadStatus)
	if !ok || result.ExitCode != 0 {
		s.obs.WorktreeObservationState = ObservationError
		s.warn("status could not be read")
		return nil
	}
	if err := CheckCapturedStream(result.StdoutObserved, ReasonRepositoryStateIncompatible); err != nil {
		s.obs.WorktreeObservationState = ObservationError
		s.incompatible(ReasonRepositoryStateIncompatible, "status output exceeds its bound")
		return nil
	}
	entries, err := parseStatus(result.Stdout)
	if err != nil {
		s.obs.WorktreeObservationState = ObservationError
		s.warn("status output could not be parsed")
		return nil
	}
	var indexChanges []IndexChange
	var worktreeChanges []WorktreeChange
	mismatch := false
	for _, entry := range entries {
		path := NewObservedPath(entry.path)
		switch entry.kind {
		case '?', '!':
			kind := WorktreeChangeUntracked
			if entry.kind == '!' {
				kind = WorktreeChangeIgnored
			}
			worktreeChanges = append(worktreeChanges, WorktreeChange{Path: path, Kind: kind})
			isDir := bytes.HasSuffix(entry.path, []byte("/"))
			evaluation := ignore.Evaluate(bytes.TrimSuffix(entry.path, []byte("/")), isDir)
			if evaluation.Ignored != (entry.kind == '!') || !evaluation.EquivalenceProven {
				mismatch = true
			}
			if value, err := NewIgnoreEvaluation(evaluation); err == nil {
				s.obs.IgnoreEvaluations = append(s.obs.IgnoreEvaluations, value)
			}
		case 'u':
			indexChanges = append(indexChanges, IndexChange{Path: path, Kind: IndexChangeConflicted})
		default:
			switch {
			case entry.x == 'T':
				indexChanges = append(indexChanges, IndexChange{Path: path, Kind: IndexChangeTypeChanged})
			case entry.x == 'M' && entry.mH != entry.mI:
				indexChanges = append(indexChanges, IndexChange{Path: path, Kind: IndexChangeModeChanged})
			case entry.x != '.':
				indexChanges = append(indexChanges, IndexChange{Path: path, Kind: IndexChangeStaged})
			}
			switch {
			case entry.y == 'T':
				worktreeChanges = append(worktreeChanges, WorktreeChange{Path: path, Kind: WorktreeChangeTypeChanged})
			case entry.y == 'M' && entry.mI != entry.mW:
				worktreeChanges = append(worktreeChanges, WorktreeChange{Path: path, Kind: WorktreeChangeModeChanged})
			case entry.y != '.':
				worktreeChanges = append(worktreeChanges, WorktreeChange{Path: path, Kind: WorktreeChangeUnstaged})
			}
		}
	}
	if mismatch {
		s.incompatible(ReasonAttributesIncompatible, "in-process ignore classification differs from Git status")
	}
	s.obs.IgnoreEvaluationsObservationState = observedState(len(s.obs.IgnoreEvaluations))
	sort.Slice(indexChanges, func(i, j int) bool {
		return pathKindLess(indexChanges[i].Path, string(indexChanges[i].Kind), indexChanges[j].Path, string(indexChanges[j].Kind))
	})
	sort.Slice(worktreeChanges, func(i, j int) bool {
		return pathKindLess(worktreeChanges[i].Path, string(worktreeChanges[i].Kind), worktreeChanges[j].Path, string(worktreeChanges[j].Kind))
	})
	identities := map[string]WorktreePathIdentity{}
	for _, change := range worktreeChanges {
		key := change.Path.ReversiblePathBytesBase64URL
		if _, done := identities[key]; done {
			continue
		}
		identity, ok := observePathIdentity(s.root + "/" + strings.TrimSuffix(string(change.Path.Bytes()), "/"))
		if !ok {
			s.obs.WorktreeObservationState = ObservationError
			s.warn("a changed worktree path has no observable identity")
			return indexChanges
		}
		identity.Path = change.Path
		identities[key] = identity
	}
	for _, identity := range identities {
		s.obs.WorktreePathIdentities = append(s.obs.WorktreePathIdentities, identity)
	}
	sort.Slice(s.obs.WorktreePathIdentities, func(i, j int) bool {
		return bytes.Compare(s.obs.WorktreePathIdentities[i].Path.Bytes(), s.obs.WorktreePathIdentities[j].Path.Bytes()) < 0
	})
	s.obs.WorktreeChanges = worktreeChanges
	s.obs.WorktreeObservationState = observedState(len(worktreeChanges))
	return indexChanges
}

// observeSources records attribute, ignore, conversion and Git Flow
// evidence.
func (s *inspection) observeSources(config ConfigEvaluation, ignore *IgnoreEvaluator, attributes *AttributeEvaluator) {
	s.obs.AttributeSources = attributes.Sources()
	s.obs.AttributeSourcesObservationState = sourceState(len(s.obs.AttributeSources), attributes.Usable())
	s.obs.IgnoreSources = ignore.Sources()
	s.obs.IgnoreSourcesObservationState = sourceState(len(s.obs.IgnoreSources), ignore.Usable())
	if !attributes.Usable() || !ignore.Usable() {
		s.incompatible(ReasonAttributesIncompatible, "an attribute or ignore source cannot be evaluated exactly")
		s.obs.IgnoreEvaluations, s.obs.IgnoreEvaluationsObservationState = nil, ObservationError
	}
	s.obs.ConversionSettings = conversionSettings(config, attributes)
	var local *InspectedSourceIdentity
	for i := range config.Sources {
		if config.Sources[i].Scope == "local" && config.Sources[i].ReadState == SourceReadRead {
			local = &config.Sources[i]
		}
	}
	evidence := GitFlowEvidenceInput{ObservationState: ObservationNotObserved, Classification: GitFlowUnobservable, Issues: []string{"local configuration was not observed"}}
	if config.State != ObservationError {
		if classified, ok := classifyGitFlow(config, local); ok {
			evidence = classified
		}
	}
	flow, err := NewGitFlowEvidence(evidence)
	if err != nil {
		flow, _ = NewGitFlowEvidence(GitFlowEvidenceInput{ObservationState: ObservationError, Classification: GitFlowUnobservable, Issues: []string{"Git Flow evidence could not be represented"}})
		s.warn("Git Flow evidence could not be represented")
	}
	if evidence.Classification != GitFlowAbsent && evidence.Classification != GitFlowUnobservable {
		s.warn("gitflow configuration is present: " + string(flow.Classification))
	}
	s.obs.GitFlowEvidence = flow
}

func sourceState(n int, usable bool) ObservationState {
	if !usable {
		return ObservationError
	}
	return observedState(n)
}

// conversionSettings summarizes every configured conversion mechanism and
// every loaded attribute rule that may convert content (F008-FR-030, 148).
func conversionSettings(config ConfigEvaluation, attributes *AttributeEvaluator) ConversionSettings {
	in := ConversionSettingsInput{}
	drivers := map[string]bool{}
	external := map[string]bool{}
	mayTransform := false
	for _, record := range config.values {
		key := record.line.key
		relevant := key == "core.autocrlf" || key == "core.eol" || key == "core.safecrlf" ||
			key == "core.checkroundtripencoding" || key == "core.attributesfile"
		if strings.HasPrefix(key, "filter.") {
			last := strings.LastIndexByte(key, '.')
			driver, member := key[len("filter."):last], key[last+1:]
			if last > len("filter.") {
				relevant = true
				drivers[driver] = true
				if (member == "clean" || member == "process") && record.line.value != "" {
					external[driver] = true
				}
			}
		}
		if !relevant {
			continue
		}
		_, value, _ := redactConfigEntry(record.line)
		if !record.line.hasValue {
			value = "true"
		}
		in.Settings = append(in.Settings, EffectiveConversionSetting{
			Key: key, ConfiguredValueRedacted: value, EffectiveValueRedacted: value, SourceScope: string(record.scope),
			SourceDigest: record.source.ContentDigest, InterpretationState: "parsed",
		})
		if key == "core.autocrlf" && (configTrue(record.line.value) || strings.EqualFold(record.line.value, "input") || !record.line.hasValue) {
			mayTransform = true
		}
	}
	for driver := range drivers {
		in.FilterDrivers = append(in.FilterDrivers, driver)
	}
	sort.Strings(in.FilterDrivers)
	for _, source := range config.Sources {
		if source.ReadState == SourceReadRead {
			in.SourceDigests = append(in.SourceDigests, source.ContentDigest)
		}
	}
	referencesExternal := false
	for _, source := range attributes.Sources() {
		if source.Identity.ReadState == SourceReadRead {
			in.SourceDigests = append(in.SourceDigests, source.Identity.ContentDigest)
		}
	}
	for _, list := range attributes.loadedLists() {
		for _, rule := range list.rules {
			for _, a := range rule.assigns {
				switch a.name {
				case "text", "eol", "crlf", "ident", "working-tree-encoding":
					if a.state.kind == 's' || a.state.kind == 'v' {
						mayTransform = true
					}
				case "filter":
					if a.state.kind == 'v' && external[a.state.value] {
						referencesExternal = true
					}
				}
			}
		}
	}
	relevant := len(in.Settings) + len(in.FilterDrivers)
	switch {
	case config.State == ObservationError || !attributes.Usable():
		in.ObservationState, in.Risk, in.IncompatibilityReason = ObservationError, ConversionRiskIncompatible, ReasonAttributesIncompatible
	case referencesExternal:
		in.ObservationState, in.Risk, in.ExternalCodeRequired = ObservationObserved, ConversionRiskExternalCode, true
	case mayTransform:
		in.ObservationState, in.Risk = ObservationObserved, ConversionRiskMayTransform
	case relevant == 0:
		in.ObservationState, in.Risk, in.ByteIdentityProven = ObservationObservedEmpty, ConversionRiskNone, true
	default:
		in.ObservationState, in.Risk, in.ByteIdentityProven = ObservationObserved, ConversionRiskNeutral, true
	}
	settings, err := NewConversionSettings(in)
	if err != nil {
		settings, _ = NewConversionSettings(ConversionSettingsInput{
			ObservationState: ObservationError, Risk: ConversionRiskIncompatible, IncompatibilityReason: ReasonAttributesIncompatible,
		})
	}
	return settings
}

// observeCandidates freezes each literal initial-commit candidate: the
// identity of the path and of every ancestor, its ignore status, and the
// attribute sources that apply to it, so conversion evidence covers it. A
// candidate that is not a literal path is not touched; planning refuses it.
func (s *inspection) observeCandidates(ignore *IgnoreEvaluator, attributes *AttributeEvaluator) {
	for _, candidate := range s.req.Candidates {
		path := string(candidate)
		if !literalPath(path) {
			continue
		}
		parts := strings.Split(path, "/")
		for i := 1; i <= len(parts); i++ {
			if rel := strings.Join(parts[:i], "/"); pathExists(s.root + "/" + rel) {
				s.addPathIdentity(rel)
			}
		}
		attributes.Attributes(candidate)
		if !ignore.Usable() {
			continue
		}
		if value, err := NewIgnoreEvaluation(ignore.Evaluate(candidate, false)); err == nil {
			s.addIgnoreEvaluation(value)
		}
	}
}

// addIgnoreEvaluation records one evaluation per path, in path order.
func (s *inspection) addIgnoreEvaluation(evaluation IgnoreEvaluation) {
	for _, existing := range s.obs.IgnoreEvaluations {
		if bytes.Equal(existing.Path.Bytes(), evaluation.Path.Bytes()) {
			return
		}
	}
	s.obs.IgnoreEvaluations = append(s.obs.IgnoreEvaluations, evaluation)
	sort.Slice(s.obs.IgnoreEvaluations, func(i, j int) bool {
		return bytes.Compare(s.obs.IgnoreEvaluations[i].Path.Bytes(), s.obs.IgnoreEvaluations[j].Path.Bytes()) < 0
	})
	s.obs.IgnoreEvaluationsObservationState = ObservationObserved
}

// observeProjectConfig observes .ai-dev/config.json without trusting it.
func (s *inspection) observeProjectConfig() {
	path := s.root + "/" + ProjectConfigPath
	observation := ProjectConfigObservation{State: ProjectConfigAbsent, Path: NewObservedPath([]byte(ProjectConfigPath))}
	switch identity, ok := observePathIdentity(path); {
	case !pathExists(path):
	case !ok || !identity.Identity.Mode.IsRegular():
		observation.State = ProjectConfigUnreadable
		s.warn(".ai-dev/config.json is not a readable regular file")
	default:
		read := readTreeSource("project_config", []byte(path), 0, nil)
		if read.identity.ReadState != SourceReadRead || read.identity.ContentDigest != identity.ContentDigest {
			observation.State = ProjectConfigUnreadable
			s.warn(".ai-dev/config.json changed or could not be read")
			break
		}
		observation.Identity, observation.ContentDigest = identity.Identity, identity.ContentDigest
		if id, valid := classifyProjectConfig(read.data); valid {
			observation.State, observation.ProjectID = ProjectConfigValid, id
		} else {
			observation.State = ProjectConfigInvalid
			s.warn(".ai-dev/config.json is not a valid project configuration")
		}
	}
	s.obs.ProjectConfig = observation
	// The project configuration directory and file are frozen so a plan
	// can bind its writes to their exact identity.
	for _, rel := range []string{".ai-dev", ProjectConfigPath} {
		if pathExists(s.root + "/" + rel) {
			s.addPathIdentity(rel)
		}
	}
}

// addPathIdentity freezes one repository-relative path once.
func (s *inspection) addPathIdentity(rel string) {
	for _, existing := range s.obs.WorktreePathIdentities {
		if strings.TrimSuffix(string(existing.Path.Bytes()), "/") == rel {
			return
		}
	}
	identity, ok := observePathIdentity(s.root + "/" + rel)
	if !ok {
		s.warn(rel + " has no observable identity")
		return
	}
	identity.Path = NewObservedPath([]byte(rel))
	s.obs.WorktreePathIdentities = append(s.obs.WorktreePathIdentities, identity)
	sort.Slice(s.obs.WorktreePathIdentities, func(i, j int) bool {
		return bytes.Compare(s.obs.WorktreePathIdentities[i].Path.Bytes(), s.obs.WorktreePathIdentities[j].Path.Bytes()) < 0
	})
}

// observeFilesystems records the project filesystem profile from mount
// observation and certification; inspection never probes by writing.
func (s *inspection) observeFilesystems() {
	if s.deps.ObserveFilesystem == nil {
		s.obs.FilesystemCapabilitiesObservationState = ObservationNotObserved
		s.incompatible(ReasonFilesystemUnsupported, "filesystem capabilities were not observed")
		return
	}
	obs, err := s.deps.ObserveFilesystem(s.root)
	if err != nil {
		s.obs.FilesystemCapabilitiesObservationState = ObservationError
		s.incompatible(ReasonFilesystemUnsupported, "the project filesystem could not be observed")
		return
	}
	profile, err := ClassifyFilesystem(FilesystemRoleProject, obs, CertifiedPrimitives(obs))
	if err != nil {
		s.obs.FilesystemCapabilitiesObservationState = ObservationError
		s.incompatible(ReasonFilesystemUnsupported, "the project filesystem could not be classified")
		return
	}
	if RequireProductionFilesystem(profile) != nil {
		s.incompatible(ReasonFilesystemUnsupported, "the project filesystem is not a supported production filesystem")
	}
	s.obs.FilesystemCapabilities = []FilesystemCapabilityProfile{profile}
	s.obs.FilesystemCapabilitiesObservationState = ObservationObserved
}

// observeRemotes builds redacted remotes from the configuration
// (F008-FR-027); no original URL is retained.
func (s *inspection) observeRemotes(config ConfigEvaluation) {
	if config.State == ObservationError {
		s.remotesState = ObservationNotObserved
		return
	}
	type remote struct {
		fetch, push []RedactedRemoteURL
		scope       string
		digest      ContentDigest
	}
	remotes := map[string]*remote{}
	var names []string
	for _, record := range config.values {
		key := record.line.key
		if !strings.HasPrefix(key, "remote.") {
			continue
		}
		last := strings.LastIndexByte(key, '.')
		name, member := key[len("remote."):max(last, len("remote."))], key[last+1:]
		if last <= len("remote.") || (member != "url" && member != "pushurl") {
			continue
		}
		r, ok := remotes[name]
		if !ok {
			r = &remote{}
			remotes[name] = r
			names = append(names, name)
		}
		display, state, credential := redactURL(record.line.value)
		url := RedactedRemoteURL{DisplayRedacted: display, RedactionState: state, CredentialMaterialDetected: credential}
		if member == "url" {
			url.Ordinal = uint64(len(r.fetch))
			r.fetch = append(r.fetch, url)
		} else {
			url.Ordinal = uint64(len(r.push))
			r.push = append(r.push, url)
		}
		r.scope, r.digest = string(record.scope), record.source.ContentDigest
	}
	sort.Strings(names)
	for _, name := range names {
		r := remotes[name]
		value, err := NewRemoteRedacted(RemoteRedactedInput{
			Name: name, FetchURLs: r.fetch, PushURLs: r.push, SourceScope: r.scope, SourceDigest: r.digest,
			ObservationState: ObservationObserved,
		})
		if err != nil {
			s.remotesState = ObservationError
			s.remotes = nil
			s.warn("a remote could not be represented safely")
			return
		}
		s.remotes = append(s.remotes, value)
	}
	s.remotesState = observedState(len(s.remotes))
}
