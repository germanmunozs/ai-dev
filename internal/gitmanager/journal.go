package gitmanager

import (
	"context"
	"errors"
	"fmt"
)

// errAuditOrder reports an event a correct apply attempt could not append
// at this point of its trail.
var errAuditOrder = errors.New("audit event out of order")

// CheckAuditEventOrder reports whether next may follow the published kinds
// of one attempt. It is the record order of the authorization and audit
// contract (§6): attempt_created is the attempt record itself; operations
// run one at a time after mutation_intent and each has exactly one result;
// releases follow operations and pair started with result; the prepared
// terminal result closes the trail except for its terminal CAS record. The
// rule makes a record that claims an effect without a started operation, or
// a closure without a prepared result, unpublishable.
func CheckAuditEventOrder(previous []AuditEventKind, next AuditEventKind) error {
	refuse := func(why string) error { return fmt.Errorf("%w: %s %s", errAuditOrder, next, why) }
	if !next.Valid() || next == AuditEventAttemptCreated {
		return refuse("is not an appendable event")
	}
	var intent, issued, prepared, closed bool
	var openOperation, openRelease, released bool
	for _, kind := range previous {
		switch kind {
		case AuditEventAuthorizationIssued:
			issued = true
		case AuditEventMutationIntent:
			intent = true
		case AuditEventOperationStarted:
			openOperation = true
		case AuditEventOperationCompleted, AuditEventOperationFailed, AuditEventOperationIndeterminate:
			openOperation = false
		case AuditEventResourceReleaseStarted:
			openRelease, released = true, true
		case AuditEventResourceReleaseResult:
			openRelease = false
		case AuditEventPreparedTerminalResult:
			prepared = true
		case AuditEventTerminalAuthorizationCAS:
			closed = true
		}
	}
	last := AuditEventKind("")
	if len(previous) > 0 {
		last = previous[len(previous)-1]
	}
	switch {
	case closed:
		return refuse("after the terminal CAS record")
	case next == AuditEventTerminalAuthorizationCAS:
		if last != AuditEventPreparedTerminalResult {
			return refuse("without a prepared terminal result")
		}
		return nil
	case prepared:
		return refuse("after the prepared terminal result")
	case openOperation && !next.operationScoped():
		return refuse("while an operation is open")
	case openRelease && next != AuditEventResourceReleaseResult:
		return refuse("while a release is open")
	}
	switch next {
	case AuditEventAuthorizationIssued:
		if issued || intent || len(previous) > 0 {
			return refuse("after other records")
		}
	case AuditEventMutationIntent:
		if intent || released {
			return refuse("twice or after release")
		}
	case AuditEventOperationStarted:
		if !intent || openOperation || released {
			return refuse("without intent, while open or after release")
		}
	case AuditEventOperationCompleted, AuditEventOperationFailed, AuditEventOperationIndeterminate:
		if !openOperation {
			return refuse("without a started operation")
		}
	case AuditEventResourceReleaseResult:
		if !openRelease {
			return refuse("without a started release")
		}
	case AuditEventPreparedTerminalResult:
		if !intent {
			return refuse("without mutation intent")
		}
	}
	return nil
}

// attemptJournal is the manager's single sequence owner for one attempt:
// every audit event of the attempt is appended through it, in order, and
// the store's no-replace publication and CheckAuditEventOrder back it.
type attemptJournal struct {
	store         GitStore
	applyID       ApplyID
	project       ProjectIdentityDigest
	authorization AuthorizationID
	next          uint64
	failed        bool
}

// openAttemptJournal durably creates the attempt record. Its failure is
// audit_unavailable and permits no mutation.
func openAttemptJournal(ctx context.Context, store GitStore, attempt GitApplyAttempt) (*attemptJournal, error) {
	if err := store.CreateAttempt(ctx, attempt); err != nil {
		return nil, err
	}
	journal := &attemptJournal{store: store, applyID: attempt.ApplyID, authorization: attempt.AuthorizationID, next: 1}
	if !attempt.ProjectIdentity.IsZero() {
		journal.project = attempt.ProjectIdentity.Digest()
	}
	return journal, nil
}

// bind records the project and authorization later events refer to.
func (j *attemptJournal) bind(project ProjectIdentityDigest, authorization AuthorizationID) {
	j.project, j.authorization = project, authorization
}

// append publishes one event whose record is the canonical rendering of
// details plus the identifying members every event carries. An outcome the
// store cannot verify still consumes the sequence number: the record may
// exist, and a later gap is crash evidence rather than a reused number.
func (j *attemptJournal) append(ctx context.Context, kind AuditEventKind, operation OperationID, details map[string]any) (ContentDigest, error) {
	record := map[string]any{
		"schema_version":          AuditEventSchema,
		"kind":                    string(kind),
		"apply_id":                string(j.applyID),
		"sequence":                canonicalInteger(j.next),
		"project_identity_digest": string(j.project),
	}
	optional(record, "authorization_id", string(j.authorization), j.authorization != "")
	optional(record, "operation_id", string(operation), operation != "")
	for key, value := range details {
		if _, reserved := record[key]; reserved {
			return "", fmt.Errorf("%w: audit detail %q", errInvalidModel, key)
		}
		record[key] = value
	}
	data, err := MarshalCanonical(record)
	if err != nil {
		return "", err
	}
	digest, err := j.store.AppendAuditEvent(ctx, AuditEvent{
		Kind: kind, ProjectIdentityDigest: j.project, ApplyID: j.applyID, Sequence: j.next,
		AuthorizationID: j.authorization, OperationID: operation, Record: data,
	})
	if err == nil || errors.Is(err, ErrStoreOutcomeUnverifiable) {
		j.next++
	}
	if err != nil {
		j.failed = true
		return "", err
	}
	return digest, nil
}

// recordResult publishes the canonical result the attempt returns.
func (j *attemptJournal) recordResult(ctx context.Context, result GitApplyResult) (ContentDigest, error) {
	data, err := encodeApplyResult(result)
	if err != nil {
		return "", err
	}
	return j.store.RecordAttemptResult(ctx, j.applyID, data)
}
