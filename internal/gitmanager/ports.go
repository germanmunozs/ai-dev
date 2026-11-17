package gitmanager

import (
	"context"
	"errors"
	"io/fs"
	"time"
)

// GitRunner is the consumer-owned boundary for one already validated Git
// invocation. Building the closed operation matrix and executing Git belong to
// later phases; this port grants neither capability by itself.
type GitRunner interface {
	Run(context.Context, GitInvocation) (GitRunResult, error)
}

// GitInvocation carries exact process inputs. Args never includes the
// executable, and no field contains a shell command string.
type GitInvocation struct {
	Executable  string
	Args        []string
	Directory   string
	Environment []string
	Stdin       []byte
	StdoutLimit int64
	StderrLimit int64
}

// GitRunResult contains bounded bytes captured from one Git invocation.
// StdoutObserved and StderrObserved count every byte Git wrote, including
// bytes beyond the capture limit that were discarded, so the manager can
// apply CheckCapturedStream with the governing reason of the operation.
type GitRunResult struct {
	Stdout         []byte
	Stderr         []byte
	ExitCode       int
	StdoutObserved int64
	StderrObserved int64
}

// ErrCapturedOutputExceeded reports that Git wrote more than an invocation's
// capture limit; the runner stops the process and keeps only the bounded
// prefix.
var ErrCapturedOutputExceeded = errors.New("captured Git output exceeded its limit")

// Store outcomes the manager must tell apart. An adapter wraps one of these
// so callers can classify with errors.Is; any other error is an ordinary
// failure that proves the requested write did not become durable.
var (
	// ErrAuthorizationExists reports an exclusive-create collision. The
	// existing record is never overwritten, merged, or widened.
	ErrAuthorizationExists = errors.New("git authorization already exists")
	// ErrAuthorizationMissing reports that no record has the requested ID.
	ErrAuthorizationMissing = errors.New("git authorization not found")
	// ErrAuthorizationConflict reports a compare-and-swap proven not to have
	// been applied because state, version, or owner differed.
	ErrAuthorizationConflict = errors.New("git authorization compare-and-swap conflict")
	// ErrApplyLockHeld reports an active advisory holder or an apply-lock
	// resource whose current identity could not be verified.
	ErrApplyLockHeld = errors.New("git apply lock unavailable")
	// ErrStoreOutcomeUnverifiable reports that the adapter cannot establish
	// whether a durable write or compare-and-swap took effect. Callers must
	// classify conservatively and must not retry.
	ErrStoreOutcomeUnverifiable = errors.New("git store outcome unverifiable")
	// ErrOwnedResourceReplaced reports that a resource this attempt created
	// is missing or no longer the object it created; it was not released.
	ErrOwnedResourceReplaced = errors.New("owned resource replaced or missing")
)

// GitStore is the consumer-owned durable authorization, audit, and apply-lock
// boundary. Every method is one step of the authorization/audit contract;
// there is no generic operation envelope, so an adapter cannot be asked for a
// transition the manager did not name.
type GitStore interface {
	// CreateAttempt exclusively creates and syncs the attempt record that
	// must precede any business result. The attempt record is the
	// attempt_created record; events of the attempt start at sequence 1.
	CreateAttempt(context.Context, GitApplyAttempt) error
	// RecordAttemptResult publishes, without replacement, the canonical
	// result an attempt returned.
	RecordAttemptResult(context.Context, ApplyID, []byte) (ContentDigest, error)
	// ListAuditEvents reads the published events of one attempt in sequence
	// order. An attempt directory a single writer could not have produced
	// is an error, never a partial trail.
	ListAuditEvents(context.Context, ApplyID) (AuditTrail, error)
	// IssueAuthorization exclusively creates an authorization in its initial
	// state.
	IssueAuthorization(context.Context, GitAuthorization) error
	// LoadAuthorization rereads the durable authorization record.
	LoadAuthorization(context.Context, ProjectIdentityDigest, AuthorizationID) (GitAuthorization, error)
	// TransitionAuthorization performs one compare-and-swap over ID, state,
	// version, and owner, returning the record it made durable.
	TransitionAuthorization(context.Context, AuthorizationTransitionRequest) (GitAuthorization, error)
	// AppendAuditEvent publishes one immutable event without replacement and
	// returns the digest of the durable record.
	AppendAuditEvent(context.Context, AuditEvent) (ContentDigest, error)
	// AcquireApplyLock takes the path-free device/inode advisory lock for the
	// physical project on behalf of one apply attempt.
	AcquireApplyLock(context.Context, PhysicalProjectIdentity, ApplyID) (ApplyLock, error)
}

// ApplyLock is one held apply lock. Release is the planned owned-resource
// release for it and never deletes the persistent lock file.
type ApplyLock interface {
	Release(context.Context) error
}

// AuditTrail is the published audit state of one attempt. CrashEvidence
// reports unpublished temporaries: a write may have been interrupted, so
// the trail cannot prove the absence of any record.
type AuditTrail struct {
	Events        []AuditEventKind
	CrashEvidence bool
}

// Contains reports whether the trail holds a published event of kind.
func (v AuditTrail) Contains(kind AuditEventKind) bool { return containsValue(v.Events, kind) }

// AuthorizationTransitionRequest is one authorization compare-and-swap. The
// Expected fields are compared; the remaining fields are written only when
// every comparison holds.
type AuthorizationTransitionRequest struct {
	ProjectIdentityDigest ProjectIdentityDigest
	AuthorizationID       AuthorizationID
	ExpectedState         AuthorizationState
	ExpectedVersion       uint64
	ExpectedOwner         ApplyID
	To                    AuthorizationState
	Owner                 ApplyID
	TerminalResultDigest  ContentDigest
}

// Validate reports whether the request names a transition the authorization
// state machine allows.
func (v AuthorizationTransitionRequest) Validate() error {
	if err := v.ProjectIdentityDigest.Validate(); err != nil {
		return err
	}
	if err := v.AuthorizationID.Validate(); err != nil {
		return err
	}
	if !v.ExpectedState.Valid() || !v.To.Valid() || v.ExpectedVersion == 0 || !authorizationTransitionAllowed(v.ExpectedState, v.To) {
		return invalid("authorization transition request")
	}
	if err := v.Owner.Validate(); err != nil {
		return err
	}
	if (v.ExpectedState == AuthorizationStateIssued) != (v.ExpectedOwner == "") {
		return invalid("authorization transition expected owner")
	}
	if v.ExpectedOwner != "" {
		if err := v.ExpectedOwner.Validate(); err != nil {
			return err
		}
	}
	// Only a proven orphan reassignment (claimed -> claimed) changes owner.
	sameOwner := v.ExpectedOwner == v.Owner
	if v.ExpectedState == AuthorizationStateClaimed && v.To == AuthorizationStateClaimed {
		if sameOwner {
			return invalid("authorization reassignment owner")
		}
	} else if v.ExpectedOwner != "" && !sameOwner {
		return invalid("authorization transition owner")
	}
	if v.To.terminal() {
		return v.TerminalResultDigest.Validate()
	}
	if v.TerminalResultDigest != "" {
		return invalid("premature terminal digest")
	}
	return nil
}

// AuditEventKind is the closed set of durable audit event variants.
type AuditEventKind string

const (
	AuditEventAttemptCreated           AuditEventKind = "attempt_created"
	AuditEventAuthorizationIssued      AuditEventKind = "authorization_issued"
	AuditEventAuthorizationTransition  AuditEventKind = "authorization_transition"
	AuditEventMutationIntent           AuditEventKind = "mutation_intent"
	AuditEventOperationStarted         AuditEventKind = "operation_started"
	AuditEventOperationCompleted       AuditEventKind = "operation_completed"
	AuditEventOperationFailed          AuditEventKind = "operation_failed"
	AuditEventOperationIndeterminate   AuditEventKind = "operation_indeterminate"
	AuditEventResourceReleaseStarted   AuditEventKind = "resource_release_started"
	AuditEventResourceReleaseResult    AuditEventKind = "resource_release_result"
	AuditEventPreparedTerminalResult   AuditEventKind = "prepared_terminal_result"
	AuditEventTerminalAuthorizationCAS AuditEventKind = "terminal_authorization_cas"
)

func (v AuditEventKind) Valid() bool {
	switch v {
	case AuditEventAttemptCreated, AuditEventAuthorizationIssued, AuditEventAuthorizationTransition,
		AuditEventMutationIntent, AuditEventOperationStarted, AuditEventOperationCompleted,
		AuditEventOperationFailed, AuditEventOperationIndeterminate, AuditEventResourceReleaseStarted,
		AuditEventResourceReleaseResult, AuditEventPreparedTerminalResult, AuditEventTerminalAuthorizationCAS:
		return true
	default:
		return false
	}
}

func (v AuditEventKind) operationScoped() bool {
	return v == AuditEventOperationStarted || v == AuditEventOperationCompleted ||
		v == AuditEventOperationFailed || v == AuditEventOperationIndeterminate
}

// AuditEvent is one immutable, sequenced audit record. Record holds the
// manager-rendered canonical bytes; it never carries file or message content,
// secrets, raw environment, temporary paths, tokens, or descriptors.
type AuditEvent struct {
	Kind                  AuditEventKind
	ProjectIdentityDigest ProjectIdentityDigest
	ApplyID               ApplyID
	Sequence              uint64
	AuthorizationID       AuthorizationID
	OperationID           OperationID
	Record                []byte
}

func (v AuditEvent) Validate() error {
	if !v.Kind.Valid() || len(v.Record) == 0 {
		return invalid("audit event")
	}
	if err := v.ProjectIdentityDigest.Validate(); err != nil {
		return err
	}
	if err := v.ApplyID.Validate(); err != nil {
		return err
	}
	if v.AuthorizationID != "" {
		if err := v.AuthorizationID.Validate(); err != nil {
			return err
		}
	}
	if v.Kind.operationScoped() != (v.OperationID != "") {
		return invalid("audit event operation ID")
	}
	if v.OperationID != "" {
		return v.OperationID.Validate()
	}
	return nil
}

// Filesystem is the consumer-owned boundary for descriptor-safe filesystem
// work. No operating-system handle crosses the port, and paths are exact
// bytes that are never normalized. Later Feature 008 tasks extend this
// closed method set; they do not add a generic operation.
type Filesystem interface {
	// Identify observes one path without following a final symlink.
	Identify(ctx context.Context, path string) (FileIdentity, error)
	// ReadRegular reads at most limit bytes of a regular file through a
	// descriptor whose identity is verified before and after the read.
	ReadRegular(ctx context.Context, path string, limit int64) (FileContent, error)
	// CreateExclusive creates a new regular file without following symlinks
	// or replacing an existing entry, then syncs the file and its directory.
	CreateExclusive(ctx context.Context, path string, data []byte, mode fs.FileMode) (FileIdentity, error)
	// CreateDirectory creates one directory level and syncs its parent.
	CreateDirectory(ctx context.Context, path string, mode fs.FileMode) (FileIdentity, error)
}

// FileIdentity is the descriptor-derived identity of one filesystem object.
type FileIdentity struct {
	DeviceDecimal string
	InodeDecimal  string
	Mode          fs.FileMode
	Size          int64
}

func (v FileIdentity) Validate() error {
	if err := ValidateDecimalIdentity(v.DeviceDecimal); err != nil {
		return err
	}
	if err := ValidateDecimalIdentity(v.InodeDecimal); err != nil {
		return err
	}
	if v.Size < 0 {
		return invalid("file size")
	}
	return nil
}

// FileContent is the bounded content of one regular file together with the
// identity it was read through.
type FileContent struct {
	Identity FileIdentity
	Data     []byte
}

// Clock supplies time without consulting ambient time from domain code.
type Clock interface {
	Now() time.Time
}

// RandomSource supplies cryptographic random bytes through an injectable
// io.Reader-compatible boundary.
type RandomSource interface {
	Read([]byte) (int, error)
}
