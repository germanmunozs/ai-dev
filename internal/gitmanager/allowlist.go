package gitmanager

import (
	"bytes"
	"errors"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"ai-dev/internal/conventionalcommit"
)

// CommitRequest is the optional initial-commit part of a plan request,
// exactly as the operator gave it (F008-FR-080, 096–107). Nothing is read
// from Git configuration or defaulted.
type CommitRequest struct {
	// MessageBytes are the exact approved message bytes.
	MessageBytes []byte
	// Includes are literal repository-relative paths. .ai-dev/config.json
	// is always part of the allowlist and may also be named here.
	Includes      []string
	Author        GitIdentity
	Committer     GitIdentity
	AuthorDate    time.Time
	CommitterDate time.Time
}

// CommitCandidates are the paths an inspection must observe for an
// initial commit: the literal includes plus the mandatory project
// configuration, in byte order without repeats.
func CommitCandidates(includes []string) [][]byte {
	seen := map[string]bool{}
	var candidates [][]byte
	for _, path := range append([]string{ProjectConfigPath}, includes...) {
		if !seen[path] {
			seen[path] = true
			candidates = append(candidates, []byte(path))
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return bytes.Compare(candidates[i], candidates[j]) < 0 })
	return candidates
}

// intentCandidates are the candidates of a frozen intent.
func intentCandidates(intent ImmutableIntent) [][]byte {
	var paths []string
	for _, file := range intent.AuthorizedFiles() {
		paths = append(paths, file.PathUTF8)
	}
	if len(paths) == 0 {
		return nil
	}
	return CommitCandidates(paths)
}

// checkCommitMessage enforces the canonical bytes of F008-FR-104 and the
// Conventional Commits grammar of F008-FR-105–106.
func checkCommitMessage(message []byte) *PlanRejection {
	if err := CheckLimit(LimitCommitMessageBytes, int64(len(message))); err != nil {
		return reject(ReasonCommitMessageInvalid, "the commit message exceeds its bound")
	}
	if !utf8.Valid(message) || bytes.ContainsAny(message, "\x00\r") || !bytes.HasSuffix(message, []byte("\n")) ||
		bytes.HasSuffix(message, []byte("\n\n")) {
		return reject(ReasonCommitMessageInvalid, "the commit message must be UTF-8 without NUL or CR and end with exactly one LF")
	}
	if v := conventionalcommit.Validate(message); !v.Valid {
		return reject(ReasonCommitMessageInvalid, "the commit message is not a Conventional Commit: %s", strings.Join(v.Diagnostics, "; "))
	}
	return nil
}

// identityCrud are the bytes Git strips from either end of an identity
// name or email; such a value would not be stored as approved.
const identityCrud = ".,:;<>\"\\'"

// checkIdentity enforces F008-FR-098–099 for the exact bytes commit-tree
// stores.
func checkIdentity(identity GitIdentity) *PlanRejection {
	invalid := reject(ReasonIdentityInvalid, "author and committer need a safe explicit name and email")
	for _, value := range []string{identity.Name, identity.Email} {
		if value == "" || !utf8.ValidString(value) || strings.ContainsAny(value, "<>") ||
			strings.ContainsAny(value[:1], identityCrud+" ") || strings.ContainsAny(value[len(value)-1:], identityCrud+" ") {
			return invalid
		}
		for _, r := range value {
			if unicode.IsControl(r) {
				return invalid
			}
		}
	}
	local, domain, found := strings.Cut(identity.Email, "@")
	if !found || local == "" || domain == "" || strings.Contains(domain, "@") || strings.ContainsAny(identity.Email, " \t,\"()[]") {
		return invalid
	}
	return nil
}

// repositoryAllowsInitialCommit is F008-FR-112 at planning: an unborn HEAD
// without history, an absent initial ref and an empty index without
// conflicts.
func repositoryAllowsInitialCommit(observation RepositoryObservation, initialRef string) *PlanRejection {
	incompatible := func(detail string) *PlanRejection { return reject(ReasonRepositoryStateIncompatible, "%s", detail) }
	if observation.RepositoryKind == RepositoryKindAbsent {
		return nil
	}
	head := observation.Head
	switch {
	case head.State != HeadStateUnborn || head.HistoryPresent:
		return incompatible("an initial commit needs an unborn HEAD without history")
	case observation.RefsObservationState != ObservationObserved && observation.RefsObservationState != ObservationObservedEmpty:
		return incompatible("the refs could not be observed")
	}
	for _, ref := range observation.Refs() {
		if string(ref.Name.Bytes()) == initialRef {
			return incompatible("the initial ref already exists")
		}
	}
	index := observation.Index
	switch {
	case index.Presence == IndexUnreadable:
		return incompatible("the index cannot be read")
	case index.Presence == IndexPresent && index.EntryCountDecimal != "0":
		return incompatible("an initial commit needs an empty index")
	case len(index.Changes()) > 0:
		return incompatible("the index has changes or conflicts")
	}
	return nil
}

// configFlag reads a boolean core setting from the frozen configuration;
// absent is fallback.
func configFlag(observation RepositoryObservation, key string, fallback bool) bool {
	value, found := fallback, false
	for _, entry := range observation.EffectiveConfiguration() {
		if entry.Key == key {
			value, found = !entry.ValuePresent || configTrue(entry.ValueRedacted), true
		}
	}
	if !found {
		return fallback
	}
	return value
}

// pathBehaviour is the case and Unicode behaviour paths will have in the
// repository. A new repository on macOS gets core.ignorecase and
// core.precomposeunicode from git init, so both are assumed.
func pathBehaviour(observation RepositoryObservation) (ignoreCase, precompose bool) {
	darwin := runtime.GOOS == "darwin"
	if observation.RepositoryKind == RepositoryKindAbsent {
		return darwin, darwin
	}
	return configFlag(observation, "core.ignorecase", false), configFlag(observation, "core.precomposeunicode", false)
}

// resolveAllowlist builds the per-file approvals of an initial commit
// (T083; F008-FR-082–090, 159–161, 183): literal regular files beneath the
// physical root, read through descriptors and matched with the frozen
// inspection, with their exact mode, SHA-256, blob OID and ignored status,
// plus the in-memory project configuration. Every file is screened before
// the result is returned; any finding refuses the whole group.
func resolveAllowlist(inspection GitInspection, includes []string, config []byte, format ObjectFormat) ([]AuthorizedFile, *PlanRejection) {
	observation := inspection.Observation
	root := inspection.ProjectIdentity.ReversiblePathBytes()
	paths := []string{ProjectConfigPath}
	for _, path := range includes {
		if path == ProjectConfigPath {
			continue
		}
		if !literalPath(path) {
			return nil, reject(ReasonPathUnrepresentable, "an included path is not a literal repository-relative path")
		}
		paths = append(paths, path)
	}
	if err := CheckLimit(LimitAuthorizedFiles, int64(len(paths))); err != nil {
		return nil, reject(ReasonPlanInvalid, "%v", err)
	}
	raw := make([][]byte, len(paths))
	for i, path := range paths {
		raw[i] = []byte(path)
	}
	ignoreCase, precompose := pathBehaviour(observation)
	if reason := pathEquivalenceIncompatibility(raw, ignoreCase, precompose); reason != "" {
		return nil, reject(reason, "the allowlisted paths cannot stay distinct and byte-exact")
	}
	conversion := observation.ConversionSettings
	if !conversion.ByteIdentityProven || conversion.ExternalCodeRequired {
		return nil, reject(ReasonAttributesIncompatible, "attributes or conversions could change the committed bytes")
	}
	identities := map[string]WorktreePathIdentity{}
	for _, identity := range observation.WorktreePathIdentities() {
		identities[strings.TrimSuffix(string(identity.Path.Bytes()), "/")] = identity
	}
	ignored := map[string]IgnoreEvaluation{}
	for _, evaluation := range observation.IgnoreEvaluations() {
		ignored[string(evaluation.Path.Bytes())] = evaluation
	}
	fileMode := configFlag(observation, "core.filemode", true)

	var files []AuthorizedFile
	var findings []SecurityFinding
	for _, path := range paths {
		evaluation, evaluated := ignored[path]
		if !evaluated || !evaluation.EquivalenceProven {
			return nil, reject(ReasonAttributesIncompatible, "the ignore status of an allowlisted path is not proven")
		}
		file := AuthorizedFile{
			PathUTF8: path, ReversiblePathBytesBase64URL: EncodeReversiblePath([]byte(path)), Mode: "100644",
			Ignored: evaluation.Ignored, ApprovalKind: ApprovalKindNormal, IndividuallyApproved: true,
		}
		if file.Ignored {
			file.ApprovalKind = ApprovalKindIgnored
		}
		frozen, present := identities[path]
		var data []byte
		switch {
		case path == ProjectConfigPath && !present:
			// Written by this attempt before the commit group: its bytes are
			// the in-memory rendering and it has no identity yet.
			data = config
			file.ObservedIdentity = FileIdentity{DeviceDecimal: "0", InodeDecimal: "0", Mode: 0o644, Size: int64(len(config))}
		default:
			if rejection := literalAncestors(identities, path); rejection != nil {
				return nil, rejection
			}
			if !present || !frozen.Identity.Mode.IsRegular() {
				return nil, reject(ReasonRepositoryStateIncompatible, "an allowlisted path is not a regular file")
			}
			content, err := readLiteralFile(root, path, LimitAuthorizedFileBytes.Max())
			if err != nil {
				var limit *LimitError
				if errors.As(err, &limit) {
					return nil, reject(ReasonRepositoryStateIncompatible, "an allowlisted file exceeds its bound")
				}
				return nil, reject(ReasonRepositoryStateIncompatible, "an allowlisted path is not a literal regular file")
			}
			if content.Identity != frozen.Identity || contentDigest(content.Data) != frozen.ContentDigest {
				return nil, reject(ReasonRepositoryStateIncompatible, "an allowlisted file changed after inspection")
			}
			data, file.ObservedIdentity = content.Data, content.Identity
			if fileMode && content.Identity.Mode.Perm()&0o100 != 0 {
				file.Mode = "100755"
			}
			if path == ProjectConfigPath && (file.Mode != "100644" || !bytes.Equal(data, config)) {
				return nil, reject(ReasonConfigurationConflict, ".ai-dev/config.json must hold the planned bytes with mode 100644")
			}
		}
		file.ContentSHA256, file.BlobOID = contentDigest(data), blobOID(format, data)
		findings = append(findings, secretFindings([]byte(path), data)...)
		files = append(files, file)
	}
	if len(findings) > 0 {
		return nil, &PlanRejection{Reason: ReasonSensitiveContentSuspected, Detail: "high-risk content was found in allowlisted files", Findings: findings}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].PathUTF8 < files[j].PathUTF8 })
	if err := validateAuthorizedFiles(files); err != nil {
		var limit *LimitError
		if errors.As(err, &limit) {
			return nil, reject(limit.Reason, "%v", err)
		}
		return nil, reject(ReasonPlanInvalid, "%v", err)
	}
	return files, nil
}

// literalPath is F008-FR-086–087: a literal relative UTF-8 path with no
// glob, traversal, control, NUL, '|' or .git component.
func literalPath(path string) bool {
	if !validAuthorizedPath(path) || CheckPathBytes([]byte(path)) != nil {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if strings.EqualFold(part, ".git") {
			return false
		}
	}
	return true
}

// literalAncestors requires every frozen ancestor of path to be a real
// directory: a symlink component is refused.
func literalAncestors(identities map[string]WorktreePathIdentity, path string) *PlanRejection {
	parts := strings.Split(path, "/")
	for i := 1; i < len(parts); i++ {
		ancestor, ok := identities[strings.Join(parts[:i], "/")]
		if !ok || !ancestor.Identity.Mode.IsDir() {
			return reject(ReasonPathUnrepresentable, "an allowlisted path has a component that is not a directory")
		}
	}
	return nil
}
