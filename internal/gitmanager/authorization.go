package gitmanager

import (
	"bytes"
	"sort"
	"strings"
	"time"
)

// IssueRequest is the non-interactive issuance input of apply
// (F008-FR-178). Every value is the operator's explicit declaration.
type IssueRequest struct {
	AuthorizationID      string
	AuthorizedBy         string
	ExpiresAt            string // RFC 3339, empty when the authorization does not expire
	ConfirmInitialCommit bool
	Approvals            []string
	IgnoredApprovals     []string
}

// approval is one parsed PATH|MODE|sha256:DIGEST|blob:OID tuple.
type approval struct {
	path, mode string
	content    ContentDigest
	blob       BlobOID
	ignored    bool
}

// parseApproval splits one approval tuple. A path cannot hold '|', so the
// tuple has exactly four fields (F008-FR-177).
func parseApproval(tuple string, ignored bool) (approval, bool) {
	fields := strings.Split(tuple, "|")
	if len(fields) != 4 || !strings.HasPrefix(fields[3], "blob:") {
		return approval{}, false
	}
	a := approval{path: fields[0], mode: fields[1], content: ContentDigest(fields[2]), blob: BlobOID(strings.TrimPrefix(fields[3], "blob:")), ignored: ignored}
	if a.path == "" || a.content.Validate() != nil || a.blob.Validate() != nil {
		return approval{}, false
	}
	return a, true
}

// checkApprovals proves the approval set equals the plan allowlist exactly:
// one approval per file, in the matching normal or ignored form, with equal
// mode, content digest and blob, and nothing more (F008-FR-159–161).
func checkApprovals(intent ImmutableIntent, req IssueRequest) *PlanRejection {
	files := intent.AuthorizedFiles()
	if !intent.InitialCommit {
		if req.ConfirmInitialCommit || len(req.Approvals)+len(req.IgnoredApprovals) > 0 {
			return reject(ReasonAuthorizationMismatch, "the plan has no initial commit to approve")
		}
		return nil
	}
	if !req.ConfirmInitialCommit {
		return reject(ReasonAuthorizationInputIncomplete, "the initial commit needs --confirm-initial-commit")
	}
	given := map[string]approval{}
	add := func(tuples []string, ignored bool) *PlanRejection {
		for _, tuple := range tuples {
			a, ok := parseApproval(tuple, ignored)
			if !ok {
				return reject(ReasonAuthorizationMismatch, "malformed file approval")
			}
			if _, duplicate := given[a.path]; duplicate {
				return reject(ReasonAuthorizationMismatch, "duplicate approval for %q", a.path)
			}
			given[a.path] = a
		}
		return nil
	}
	if rejection := add(req.Approvals, false); rejection != nil {
		return rejection
	}
	if rejection := add(req.IgnoredApprovals, true); rejection != nil {
		return rejection
	}
	for _, file := range files {
		a, ok := given[file.PathUTF8]
		if !ok {
			return reject(ReasonAuthorizationInputIncomplete, "no approval for %q", file.PathUTF8)
		}
		if a.mode != file.Mode || a.content != file.ContentSHA256 || a.blob != file.BlobOID || a.ignored != file.Ignored {
			return reject(ReasonAuthorizationMismatch, "the approval for %q does not match the plan", file.PathUTF8)
		}
		delete(given, file.PathUTF8)
	}
	if len(given) > 0 {
		return reject(ReasonAuthorizationMismatch, "approvals name files outside the plan allowlist")
	}
	return nil
}

// authorizationScope is the exact scope a plan grants: every operation
// ID, and the refs and configuration keys the operations affect.
type authorizationScope struct {
	operations []OperationID
	refs       []ObservedRef
	configKeys []string
}

func planScope(plan GitInitializationPlan) authorizationScope {
	var scope authorizationScope
	refs := map[string]ObservedRef{}
	keys := map[string]bool{}
	for _, operation := range plan.DerivedOperations() {
		scope.operations = append(scope.operations, operation.OperationID)
		for _, ref := range operation.AffectedRefs() {
			refs[ref.ReversibleBytesBase64URL] = ref
		}
		for _, key := range operation.AffectedConfigKeys() {
			keys[key] = true
		}
	}
	for _, ref := range refs {
		scope.refs = append(scope.refs, ref)
	}
	sort.Slice(scope.refs, func(i, j int) bool { return bytes.Compare(scope.refs[i].Bytes(), scope.refs[j].Bytes()) < 0 })
	for key := range keys {
		scope.configKeys = append(scope.configKeys, key)
	}
	sort.Strings(scope.configKeys)
	return scope
}

// newAuthorization builds the issued authorization for a regenerated plan
// (T064). It is a pure domain value; the store alone makes it durable.
func newAuthorization(req IssueRequest, plan GitInitializationPlan, now time.Time) (GitAuthorization, *PlanRejection) {
	if req.AuthorizationID == "" || req.AuthorizedBy == "" {
		return GitAuthorization{}, reject(ReasonAuthorizationInputIncomplete, "issuance needs --authorization-id and --authorized-by")
	}
	if AuthorizationID(req.AuthorizationID).Validate() != nil {
		return GitAuthorization{}, reject(ReasonAuthorizationInputIncomplete, "the authorization ID is not a UUIDv4")
	}
	if validateOpaque("declared operator", req.AuthorizedBy) != nil {
		return GitAuthorization{}, reject(ReasonAuthorizationInputIncomplete, "the declared operator is empty or contains control characters")
	}
	issuedAt := now.UTC().Truncate(time.Second)
	var expires time.Time
	if req.ExpiresAt != "" {
		parsed, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil || parsed.Nanosecond() != 0 || !parsed.After(issuedAt) {
			return GitAuthorization{}, reject(ReasonAuthorizationInputIncomplete, "the expiry must be a whole-second RFC 3339 time after now")
		}
		expires = parsed
	}
	if rejection := checkApprovals(plan.ImmutableIntent, req); rejection != nil {
		return GitAuthorization{}, rejection
	}
	intent := plan.ImmutableIntent
	scope := planScope(plan)
	var approved []AuthorizedFile
	if req.ConfirmInitialCommit {
		approved = intent.AuthorizedFiles()
	}
	authorization, err := NewGitAuthorization(GitAuthorizationInput{
		SchemaVersion: GitAuthorizationSchema, AuthorizationID: AuthorizationID(req.AuthorizationID), PlanDigest: plan.PlanDigest,
		ProjectIdentity: plan.PreconditionSnapshot.ProjectIdentity, AuthorizedOperationIDs: scope.operations,
		Strategy: intent.Strategy, GitFlowDialect: intent.GitFlowDialect, AuthorizedRefs: scope.refs,
		AuthorizedConfigKeys: scope.configKeys, InitialCommitAuthorized: req.ConfirmInitialCommit && intent.InitialCommit,
		ApprovedFiles: approved, DeclaredOperator: req.AuthorizedBy, IssuedAt: issuedAt, ExpiresAt: expires,
		State: AuthorizationStateIssued, StateVersion: 1,
	})
	if err != nil {
		return GitAuthorization{}, reject(ReasonAuthorizationInputIncomplete, "the authorization cannot be formed: %v", err)
	}
	return authorization, nil
}

// checkScope binds an authorization to the regenerated plan and the
// freshly verified project: the plan digest, project identity, strategy,
// every operation ID, ref, configuration key and file approval must be
// exactly those of the plan (F008-FR-152, 167).
func checkScope(authorization GitAuthorization, plan GitInitializationPlan) *PlanRejection {
	mismatch := func(what string) *PlanRejection {
		return reject(ReasonAuthorizationMismatch, "the authorization %s differs from the plan", what)
	}
	intent, scope := plan.ImmutableIntent, planScope(plan)
	switch {
	case authorization.PlanDigest != plan.PlanDigest:
		return mismatch("plan digest")
	case authorization.ProjectIdentity.Digest() != plan.PreconditionSnapshot.ProjectIdentity.Digest():
		return mismatch("project identity")
	case authorization.Strategy != intent.Strategy || authorization.GitFlowDialect != intent.GitFlowDialect:
		return mismatch("strategy")
	case !equalLists(authorization.AuthorizedOperationIDs(), scope.operations):
		return mismatch("operations")
	case !equalLists(authorization.AuthorizedRefs(), scope.refs):
		return mismatch("refs")
	case !equalLists(authorization.AuthorizedConfigKeys(), scope.configKeys):
		return mismatch("configuration keys")
	case authorization.InitialCommitAuthorized != intent.InitialCommit:
		return mismatch("initial-commit decision")
	case !equalLists(authorization.ApprovedFiles(), intent.AuthorizedFiles()):
		return mismatch("file approvals")
	}
	return nil
}

func equalLists[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
