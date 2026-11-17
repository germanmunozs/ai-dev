package gitmanager

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strconv"
	"sync"
	"testing"
	"time"
)

var errInjected = errors.New("injected failure")

type fakeGitRunner struct {
	mu      sync.Mutex
	Calls   []GitInvocation
	Results []GitRunResult
	Errors  map[int]error
}

func (f *fakeGitRunner) Run(_ context.Context, request GitInvocation) (GitRunResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, cloneGitInvocation(request))
	index := len(f.Calls) - 1
	var result GitRunResult
	if index < len(f.Results) {
		result = cloneGitRunResult(f.Results[index])
	}
	return result, f.Errors[index]
}

func (f *fakeGitRunner) callsSnapshot() []GitInvocation {
	f.mu.Lock()
	defer f.mu.Unlock()
	calls := make([]GitInvocation, len(f.Calls))
	for index, call := range f.Calls {
		calls[index] = cloneGitInvocation(call)
	}
	return calls
}

// faultPlan injects failures by zero-based call index, counted across every
// method of one fake. A failure in before proves the call had no effect; a
// failure in after is returned once the effect is durable, which is how an
// adapter whose outcome cannot be verified looks to the manager.
type faultPlan struct {
	before   map[int]error
	after    map[int]error
	calls    []string
	byMethod map[string]map[int]methodFault
}

// methodFault fails the nth zero-based call of one method, before or after
// its effect.
type methodFault struct {
	err   error
	after bool
}

// begin records the call and returns its index and any pre-effect failure.
func (p *faultPlan) begin(method string) (int, error) {
	occurrence := 0
	for _, call := range p.calls {
		if call == method {
			occurrence++
		}
	}
	p.calls = append(p.calls, method)
	index := len(p.calls) - 1
	if fault, ok := p.byMethod[method][occurrence]; ok {
		if !fault.after {
			return index, fault.err
		}
		p.failAfter(index, fault.err)
	}
	return index, p.before[index]
}

// failCall fails the nth zero-based call of method.
func (p *faultPlan) failCall(method string, nth int, err error, after bool) {
	if p.byMethod == nil {
		p.byMethod = make(map[string]map[int]methodFault)
	}
	if p.byMethod[method] == nil {
		p.byMethod[method] = make(map[int]methodFault)
	}
	p.byMethod[method][nth] = methodFault{err: err, after: after}
}

func (p *faultPlan) failBefore(index int, err error) {
	if p.before == nil {
		p.before = make(map[int]error)
	}
	p.before[index] = err
}

func (p *faultPlan) failAfter(index int, err error) {
	if p.after == nil {
		p.after = make(map[int]error)
	}
	p.after[index] = err
}

// fakeGitStore is an in-memory GitStore that enforces the same exclusivity,
// compare-and-swap and audit-order rules as the contract, through the model
// itself.
type fakeGitStore struct {
	mu             sync.Mutex
	faults         faultPlan
	attempts       map[ApplyID]GitApplyAttempt
	results        map[ApplyID][]byte
	crashEvidence  map[ApplyID]bool
	authorizations map[AuthorizationID]GitAuthorization
	events         []AuditEvent
	locks          map[string]ApplyID
}

func newFakeGitStore() *fakeGitStore {
	return &fakeGitStore{
		attempts:       make(map[ApplyID]GitApplyAttempt),
		results:        make(map[ApplyID][]byte),
		crashEvidence:  make(map[ApplyID]bool),
		authorizations: make(map[AuthorizationID]GitAuthorization),
		locks:          make(map[string]ApplyID),
	}
}

func (f *fakeGitStore) RecordAttemptResult(_ context.Context, id ApplyID, record []byte) (ContentDigest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	index, err := f.faults.begin("RecordAttemptResult")
	if err != nil {
		return "", err
	}
	if _, exists := f.attempts[id]; !exists || len(record) == 0 {
		return "", errors.New("result without attempt")
	}
	if _, exists := f.results[id]; exists {
		return "", errors.New("attempt result already recorded")
	}
	f.results[id] = cloneSlice(record)
	if err := f.faults.after[index]; err != nil {
		return "", err
	}
	return recordDigest(record), nil
}

func (f *fakeGitStore) ListAuditEvents(_ context.Context, id ApplyID) (AuditTrail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, err := f.faults.begin("ListAuditEvents"); err != nil {
		return AuditTrail{}, err
	}
	if _, exists := f.attempts[id]; !exists {
		return AuditTrail{}, errors.New("unknown attempt")
	}
	return AuditTrail{Events: f.kindsLocked(id), CrashEvidence: f.crashEvidence[id]}, nil
}

func (f *fakeGitStore) kindsLocked(id ApplyID) []AuditEventKind {
	var kinds []AuditEventKind
	for _, event := range f.events {
		if event.ApplyID == id {
			kinds = append(kinds, event.Kind)
		}
	}
	return kinds
}

func (f *fakeGitStore) CreateAttempt(_ context.Context, attempt GitApplyAttempt) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	index, err := f.faults.begin("CreateAttempt")
	if err != nil {
		return err
	}
	if err := attempt.Validate(); err != nil {
		return err
	}
	if _, exists := f.attempts[attempt.ApplyID]; exists {
		return errors.New("attempt already exists")
	}
	f.attempts[attempt.ApplyID] = attempt
	return f.faults.after[index]
}

func (f *fakeGitStore) IssueAuthorization(_ context.Context, authorization GitAuthorization) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	index, err := f.faults.begin("IssueAuthorization")
	if err != nil {
		return err
	}
	if err := authorization.Validate(); err != nil {
		return err
	}
	if authorization.State != AuthorizationStateIssued || authorization.StateVersion != 1 {
		return invalid("issued authorization")
	}
	if _, exists := f.authorizations[authorization.AuthorizationID]; exists {
		return ErrAuthorizationExists
	}
	f.authorizations[authorization.AuthorizationID] = authorization
	return f.faults.after[index]
}

func (f *fakeGitStore) LoadAuthorization(_ context.Context, project ProjectIdentityDigest, id AuthorizationID) (GitAuthorization, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, err := f.faults.begin("LoadAuthorization"); err != nil {
		return GitAuthorization{}, err
	}
	authorization, exists := f.authorizations[id]
	if !exists || authorization.ProjectIdentity.Digest() != project {
		return GitAuthorization{}, ErrAuthorizationMissing
	}
	return authorization, nil
}

func (f *fakeGitStore) TransitionAuthorization(_ context.Context, request AuthorizationTransitionRequest) (GitAuthorization, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	index, err := f.faults.begin("TransitionAuthorization")
	if err != nil {
		return GitAuthorization{}, err
	}
	current, exists := f.authorizations[request.AuthorizationID]
	if !exists || current.ProjectIdentity.Digest() != request.ProjectIdentityDigest {
		return GitAuthorization{}, ErrAuthorizationMissing
	}
	next, err := current.Transition(request)
	if err != nil {
		return GitAuthorization{}, err
	}
	f.authorizations[request.AuthorizationID] = next
	if err := f.faults.after[index]; err != nil {
		return GitAuthorization{}, err
	}
	return next, nil
}

func (f *fakeGitStore) AppendAuditEvent(_ context.Context, event AuditEvent) (ContentDigest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	index, err := f.faults.begin("AppendAuditEvent")
	if err != nil {
		return "", err
	}
	if err := event.Validate(); err != nil {
		return "", err
	}
	if _, exists := f.attempts[event.ApplyID]; !exists {
		return "", errors.New("audit event without attempt")
	}
	previous := f.kindsLocked(event.ApplyID)
	if event.Sequence != uint64(len(previous))+1 {
		return "", errors.New("audit event out of sequence")
	}
	if err := CheckAuditEventOrder(previous, event.Kind); err != nil {
		return "", err
	}
	event.Record = cloneSlice(event.Record)
	f.events = append(f.events, event)
	if err := f.faults.after[index]; err != nil {
		return "", err
	}
	return recordDigest(event.Record), nil
}

func (f *fakeGitStore) AcquireApplyLock(_ context.Context, project PhysicalProjectIdentity, owner ApplyID) (ApplyLock, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, err := f.faults.begin("AcquireApplyLock"); err != nil {
		return nil, err
	}
	// The lock namespace is device/inode only: path aliases contend.
	key := project.DeviceDecimal() + "\x00" + project.InodeDecimal()
	if _, held := f.locks[key]; held {
		return nil, ErrApplyLockHeld
	}
	f.locks[key] = owner
	return &fakeApplyLock{store: f, key: key}, nil
}

type fakeApplyLock struct {
	store    *fakeGitStore
	key      string
	released bool
}

func (l *fakeApplyLock) Release(context.Context) error {
	l.store.mu.Lock()
	defer l.store.mu.Unlock()
	index, err := l.store.faults.begin("ApplyLock.Release")
	if err != nil {
		return err
	}
	if l.released {
		return errors.New("apply lock already released")
	}
	l.released = true
	delete(l.store.locks, l.key)
	return l.store.faults.after[index]
}

func (f *fakeGitStore) callsSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return cloneSlice(f.faults.calls)
}

func (f *fakeGitStore) eventsSnapshot() []AuditEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	events := make([]AuditEvent, len(f.events))
	for index, event := range f.events {
		event.Record = cloneSlice(event.Record)
		events[index] = event
	}
	return events
}

// recordDigest is the digest a store derives from the durable record bytes.
func recordDigest(record []byte) ContentDigest {
	return ContentDigest(fmt.Sprintf("sha256:%x", sha256.Sum256(record)))
}

type fakeFile struct {
	identity FileIdentity
	data     []byte
}

// fakeFilesystem is an in-memory Filesystem. Paths are exact byte strings and
// are never cleaned or normalized.
type fakeFilesystem struct {
	mu        sync.Mutex
	faults    faultPlan
	entries   map[string]fakeFile
	nextInode uint64
}

func newFakeFilesystem() *fakeFilesystem {
	return &fakeFilesystem{entries: make(map[string]fakeFile), nextInode: 100}
}

func (f *fakeFilesystem) create(path string, data []byte, mode fs.FileMode) (FileIdentity, error) {
	if _, exists := f.entries[path]; exists {
		return FileIdentity{}, fs.ErrExist
	}
	f.nextInode++
	identity := FileIdentity{DeviceDecimal: "1", InodeDecimal: strconv.FormatUint(f.nextInode, 10), Mode: mode, Size: int64(len(data))}
	f.entries[path] = fakeFile{identity: identity, data: cloneSlice(data)}
	return identity, nil
}

func (f *fakeFilesystem) Identify(_ context.Context, path string) (FileIdentity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, err := f.faults.begin("Identify"); err != nil {
		return FileIdentity{}, err
	}
	entry, exists := f.entries[path]
	if !exists {
		return FileIdentity{}, fs.ErrNotExist
	}
	return entry.identity, nil
}

func (f *fakeFilesystem) ReadRegular(_ context.Context, path string, limit int64) (FileContent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, err := f.faults.begin("ReadRegular"); err != nil {
		return FileContent{}, err
	}
	entry, exists := f.entries[path]
	if !exists {
		return FileContent{}, fs.ErrNotExist
	}
	if !entry.identity.Mode.IsRegular() {
		return FileContent{}, errors.New("not a regular file")
	}
	if int64(len(entry.data)) > limit {
		return FileContent{}, errors.New("file exceeds read limit")
	}
	return FileContent{Identity: entry.identity, Data: cloneSlice(entry.data)}, nil
}

func (f *fakeFilesystem) CreateExclusive(_ context.Context, path string, data []byte, mode fs.FileMode) (FileIdentity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	index, err := f.faults.begin("CreateExclusive")
	if err != nil {
		return FileIdentity{}, err
	}
	identity, err := f.create(path, data, mode.Perm())
	if err != nil {
		return FileIdentity{}, err
	}
	return identity, f.faults.after[index]
}

func (f *fakeFilesystem) CreateDirectory(_ context.Context, path string, mode fs.FileMode) (FileIdentity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	index, err := f.faults.begin("CreateDirectory")
	if err != nil {
		return FileIdentity{}, err
	}
	identity, err := f.create(path, nil, mode.Perm()|fs.ModeDir)
	if err != nil {
		return FileIdentity{}, err
	}
	return identity, f.faults.after[index]
}

func (f *fakeFilesystem) callsSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return cloneSlice(f.faults.calls)
}

type fakeClock struct {
	mu    sync.Mutex
	times []time.Time
}

func newFakeClock(times ...time.Time) *fakeClock {
	return &fakeClock{times: append([]time.Time(nil), times...)}
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.times) == 0 {
		panic("fakeClock exhausted")
	}
	now := f.times[0]
	f.times = f.times[1:]
	return now
}

type fakeRandomSource struct {
	mu          sync.Mutex
	data        []byte
	offset      int
	errorAt     int
	injectedErr error
}

func newFakeRandomSource(data []byte) *fakeRandomSource {
	return &fakeRandomSource{data: append([]byte(nil), data...), errorAt: -1}
}

func (f *fakeRandomSource) injectErrorAt(offset int, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errorAt = offset
	f.injectedErr = err
}

func (f *fakeRandomSource) Read(destination []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(destination) == 0 {
		return 0, nil
	}
	if f.injectedErr != nil && f.offset >= f.errorAt {
		return 0, f.injectedErr
	}
	if f.offset >= len(f.data) {
		return 0, io.ErrUnexpectedEOF
	}
	limit := len(f.data)
	if f.injectedErr != nil && f.errorAt < limit {
		limit = f.errorAt
	}
	n := copy(destination, f.data[f.offset:limit])
	f.offset += n
	if f.injectedErr != nil && f.offset >= f.errorAt {
		return n, f.injectedErr
	}
	if n < len(destination) && f.offset >= len(f.data) {
		return n, io.ErrUnexpectedEOF
	}
	return n, nil
}

func TestFakeGitRunnerInjectsFaultsAndCopies(t *testing.T) {
	ctx := context.Background()
	runner := &fakeGitRunner{Results: []GitRunResult{{Stdout: []byte("partial"), ExitCode: 1}}, Errors: map[int]error{0: errInjected}}
	request := GitInvocation{Args: []string{"status"}, Environment: []string{"A=B"}, Stdin: []byte("stdin")}
	result, err := runner.Run(ctx, request)
	if string(result.Stdout) != "partial" || result.ExitCode != 1 || !errors.Is(err, errInjected) {
		t.Fatalf("runner result=%+v err=%v", result, err)
	}
	if _, err := runner.Run(ctx, GitInvocation{Args: []string{"version"}}); err != nil {
		t.Fatalf("unconfigured call failed: %v", err)
	}
	request.Args[0], request.Environment[0], request.Stdin[0] = "changed", "changed", 'X'
	result.Stdout[0] = 'X'
	recorded := runner.callsSnapshot()[0]
	if recorded.Args[0] != "status" || recorded.Environment[0] != "A=B" || string(recorded.Stdin) != "stdin" || string(runner.Results[0].Stdout) != "partial" {
		t.Fatalf("runner retained or exposed caller-owned data: %+v", recorded)
	}
}

func TestFakeGitStoreEnforcesAuthorizationContract(t *testing.T) {
	ctx := context.Background()
	store := newFakeGitStore()
	authorization, err := NewGitAuthorization(validAuthorizationInput(t))
	if err != nil {
		t.Fatal(err)
	}
	project := authorization.ProjectIdentity.Digest()
	if _, err := store.LoadAuthorization(ctx, project, authorization.AuthorizationID); !errors.Is(err, ErrAuthorizationMissing) {
		t.Fatalf("load before issue: %v", err)
	}
	if err := store.IssueAuthorization(ctx, authorization); err != nil {
		t.Fatal(err)
	}
	if err := store.IssueAuthorization(ctx, authorization); !errors.Is(err, ErrAuthorizationExists) {
		t.Fatalf("second issue: %v", err)
	}
	if _, err := store.LoadAuthorization(ctx, testSHA256, authorization.AuthorizationID); !errors.Is(err, ErrAuthorizationMissing) {
		t.Fatalf("load from another project partition: %v", err)
	}

	claim := AuthorizationTransitionRequest{ProjectIdentityDigest: project, AuthorizationID: authorization.AuthorizationID, ExpectedState: AuthorizationStateIssued, ExpectedVersion: 1, To: AuthorizationStateClaimed, Owner: testApplyID}
	claimed, err := store.TransitionAuthorization(ctx, claim)
	if err != nil || claimed.State != AuthorizationStateClaimed {
		t.Fatalf("claim = %+v, %v", claimed, err)
	}
	// The losing claimer of the same state/version tuple is a proven conflict.
	loser := claim
	loser.Owner = testOtherApplyID
	if _, err := store.TransitionAuthorization(ctx, loser); !errors.Is(err, ErrAuthorizationConflict) {
		t.Fatalf("second claim: %v", err)
	}
	loaded, err := store.LoadAuthorization(ctx, project, authorization.AuthorizationID)
	if err != nil || loaded.OwnerApplyID != testApplyID || loaded.StateVersion != 2 {
		t.Fatalf("durable authorization = %+v, %v", loaded, err)
	}
}

func TestFakeGitStoreFaultInjection(t *testing.T) {
	ctx := context.Background()
	authorization, err := NewGitAuthorization(validAuthorizationInput(t))
	if err != nil {
		t.Fatal(err)
	}
	project := authorization.ProjectIdentity.Digest()

	// A failure before the effect proves nothing became durable.
	before := newFakeGitStore()
	before.faults.failBefore(0, errInjected)
	if err := before.IssueAuthorization(ctx, authorization); !errors.Is(err, errInjected) {
		t.Fatalf("pre-effect failure: %v", err)
	}
	if _, err := before.LoadAuthorization(ctx, project, authorization.AuthorizationID); !errors.Is(err, ErrAuthorizationMissing) {
		t.Fatalf("pre-effect failure left a record: %v", err)
	}

	// A failure after the effect leaves the transition durable although the
	// caller could not verify it.
	after := newFakeGitStore()
	if err := after.IssueAuthorization(ctx, authorization); err != nil {
		t.Fatal(err)
	}
	after.faults.failAfter(1, ErrStoreOutcomeUnverifiable)
	claim := AuthorizationTransitionRequest{ProjectIdentityDigest: project, AuthorizationID: authorization.AuthorizationID, ExpectedState: AuthorizationStateIssued, ExpectedVersion: 1, To: AuthorizationStateClaimed, Owner: testApplyID}
	if _, err := after.TransitionAuthorization(ctx, claim); !errors.Is(err, ErrStoreOutcomeUnverifiable) {
		t.Fatalf("post-effect failure: %v", err)
	}
	loaded, err := after.LoadAuthorization(ctx, project, authorization.AuthorizationID)
	if err != nil || loaded.State != AuthorizationStateClaimed {
		t.Fatalf("post-effect failure lost the durable transition: %+v, %v", loaded, err)
	}
	if calls := after.callsSnapshot(); len(calls) != 3 || calls[1] != "TransitionAuthorization" {
		t.Fatalf("recorded calls = %v", calls)
	}
}

func TestFakeGitStoreAuditAttemptsAndLock(t *testing.T) {
	ctx := context.Background()
	store := newFakeGitStore()
	project := mustProjectIdentity(t)
	attempt := GitApplyAttempt{SchemaVersion: "ai-dev.git-apply-attempt/v1", ApplyID: testApplyID, StartedAt: time.Unix(2, 0).UTC(), InvocationMode: InvocationModeDirect}
	if err := store.CreateAttempt(ctx, attempt); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateAttempt(ctx, attempt); err == nil {
		t.Fatal("attempt record replaced")
	}

	record := []byte(`{"kind":"mutation_intent"}`)
	event := AuditEvent{Kind: AuditEventMutationIntent, ProjectIdentityDigest: project.Digest(), ApplyID: testApplyID, Sequence: 1, Record: record}
	digest, err := store.AppendAuditEvent(ctx, event)
	if err != nil || digest.Validate() != nil {
		t.Fatalf("append = %q, %v", digest, err)
	}
	if _, err := store.AppendAuditEvent(ctx, event); err == nil {
		t.Fatal("audit event replaced")
	}
	record[0] = 'X'
	events := store.eventsSnapshot()
	events[0].Record[1] = 'X'
	if got := store.eventsSnapshot(); len(got) != 1 || string(got[0].Record) != `{"kind":"mutation_intent"}` {
		t.Fatalf("store retained or exposed caller-owned record: %q", got[0].Record)
	}

	lock, err := store.AcquireApplyLock(ctx, project, testApplyID)
	if err != nil {
		t.Fatal(err)
	}
	// A path alias of the same device/inode contends on the same lock.
	alias, err := NewPhysicalProjectIdentity("/alias/project", []byte("/alias/project"), project.DeviceDecimal(), project.InodeDecimal())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AcquireApplyLock(ctx, alias, testOtherApplyID); !errors.Is(err, ErrApplyLockHeld) {
		t.Fatalf("aliased acquisition: %v", err)
	}
	if err := lock.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if err := lock.Release(ctx); err == nil {
		t.Fatal("apply lock released twice")
	}
	if _, err := store.AcquireApplyLock(ctx, alias, testOtherApplyID); err != nil {
		t.Fatalf("reuse of a free lock: %v", err)
	}
}

func TestFakeFilesystemExclusiveAndBounded(t *testing.T) {
	ctx := context.Background()
	filesystem := newFakeFilesystem()
	directory, err := filesystem.CreateDirectory(ctx, "/p/.ai-dev", 0o700)
	if err != nil || !directory.Mode.IsDir() || directory.Validate() != nil {
		t.Fatalf("directory = %+v, %v", directory, err)
	}
	data := []byte("config")
	created, err := filesystem.CreateExclusive(ctx, "/p/.ai-dev/config.json", data, 0o600)
	if err != nil || created.Size != 6 || created.InodeDecimal == directory.InodeDecimal {
		t.Fatalf("created = %+v, %v", created, err)
	}
	if _, err := filesystem.CreateExclusive(ctx, "/p/.ai-dev/config.json", data, 0o600); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("replacement create: %v", err)
	}
	data[0] = 'X'
	content, err := filesystem.ReadRegular(ctx, "/p/.ai-dev/config.json", 6)
	if err != nil || string(content.Data) != "config" || content.Identity != created {
		t.Fatalf("content = %+v, %v", content, err)
	}
	content.Data[0] = 'X'
	if again, _ := filesystem.ReadRegular(ctx, "/p/.ai-dev/config.json", 6); string(again.Data) != "config" {
		t.Fatal("filesystem exposed stored bytes")
	}
	if _, err := filesystem.ReadRegular(ctx, "/p/.ai-dev/config.json", 5); err == nil {
		t.Fatal("read over the limit accepted")
	}
	if _, err := filesystem.ReadRegular(ctx, "/p/.ai-dev", 10); err == nil {
		t.Fatal("directory read as a regular file")
	}
	if identity, err := filesystem.Identify(ctx, "/p/.ai-dev/config.json"); err != nil || identity != created {
		t.Fatalf("identify = %+v, %v", identity, err)
	}
	if _, err := filesystem.Identify(ctx, "/p/missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("identify missing: %v", err)
	}

	faulty := newFakeFilesystem()
	faulty.faults.failBefore(0, errInjected)
	faulty.faults.failAfter(1, errInjected)
	if _, err := faulty.CreateExclusive(ctx, "/a", nil, 0o600); !errors.Is(err, errInjected) {
		t.Fatalf("pre-effect failure: %v", err)
	}
	if _, err := faulty.CreateExclusive(ctx, "/a", nil, 0o600); !errors.Is(err, errInjected) {
		t.Fatalf("post-effect failure: %v", err)
	}
	if _, err := faulty.Identify(ctx, "/a"); err != nil {
		t.Fatalf("post-effect failure lost the created file: %v", err)
	}
	if calls := faulty.callsSnapshot(); len(calls) != 3 {
		t.Fatalf("recorded calls = %v", calls)
	}
}

func TestFakeRandomSourceReadFullAndExhaustion(t *testing.T) {
	t.Run("normal and reproducible", func(t *testing.T) {
		for iteration := 0; iteration < 2; iteration++ {
			source := newFakeRandomSource([]byte{1, 2, 3, 4})
			buffer := make([]byte, 4)
			if n, err := io.ReadFull(source, buffer); n != 4 || err != nil || string(buffer) != string([]byte{1, 2, 3, 4}) {
				t.Fatalf("read=%d err=%v bytes=%v", n, err, buffer)
			}
		}
	})
	t.Run("exhaustion", func(t *testing.T) {
		source := newFakeRandomSource([]byte{1, 2})
		buffer := make([]byte, 4)
		if n, err := io.ReadFull(source, buffer); n != 2 || !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("read=%d err=%v", n, err)
		}
		if n, err := source.Read(buffer); n != 0 || !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("exhausted read=%d err=%v", n, err)
		}
	})
	t.Run("injected partial error", func(t *testing.T) {
		source := newFakeRandomSource([]byte{1, 2, 3, 4})
		source.injectErrorAt(2, errInjected)
		buffer := make([]byte, 4)
		if n, err := io.ReadFull(source, buffer); n != 2 || !errors.Is(err, errInjected) || buffer[0] != 1 || buffer[1] != 2 {
			t.Fatalf("read=%d err=%v bytes=%v", n, err, buffer)
		}
	})
}

func TestPortFakesAreRaceSafe(t *testing.T) {
	const calls = 64
	ctx := context.Background()
	runner := &fakeGitRunner{}
	store := newFakeGitStore()
	filesystem := newFakeFilesystem()
	random := newFakeRandomSource(make([]byte, calls))
	project := mustProjectIdentity(t)
	times := make([]time.Time, calls)
	for index := range times {
		times[index] = time.Unix(int64(index+1), 0).UTC()
	}
	clock := newFakeClock(times...)
	results := make(chan time.Time, calls)
	var wait sync.WaitGroup
	for index := 0; index < calls; index++ {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, _ = runner.Run(ctx, GitInvocation{Args: []string{"call"}, Stdin: []byte{byte(index)}})
			applyID := ApplyID(fmt.Sprintf("123e4567-e89b-42d3-a456-4266141741%02d", index))
			if err := store.CreateAttempt(ctx, GitApplyAttempt{SchemaVersion: GitApplyAttemptSchema, ApplyID: applyID, StartedAt: time.Unix(1, 0).UTC(), InvocationMode: InvocationModeDirect}); err != nil {
				t.Error(err)
			}
			if _, err := store.AppendAuditEvent(ctx, AuditEvent{Kind: AuditEventMutationIntent, ProjectIdentityDigest: project.Digest(), ApplyID: applyID, Sequence: 1, Record: []byte{byte(index)}}); err != nil {
				t.Error(err)
			}
			_, _ = filesystem.CreateExclusive(ctx, "/file-"+strconv.Itoa(index), []byte{byte(index)}, 0o600)
			buffer := make([]byte, 1)
			if n, err := random.Read(buffer); n != 1 || err != nil {
				t.Errorf("random read=%d err=%v", n, err)
			}
			results <- clock.Now()
		}()
	}
	wait.Wait()
	close(results)
	for name, got := range map[string]int{
		"runner": len(runner.callsSnapshot()), "store": len(store.eventsSnapshot()), "filesystem": len(filesystem.callsSnapshot()),
	} {
		if got != calls {
			t.Fatalf("%s calls=%d want=%d", name, got, calls)
		}
	}
	seen := make(map[time.Time]bool)
	for result := range results {
		if result.IsZero() || seen[result] {
			t.Fatalf("invalid or duplicate clock result %v", result)
		}
		seen[result] = true
	}
}

func TestFakeClockPanicsWhenExhausted(t *testing.T) {
	clock := newFakeClock()
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("fakeClock did not fail explicitly when exhausted")
		}
	}()
	_ = clock.Now()
}

func cloneGitInvocation(value GitInvocation) GitInvocation {
	value.Args = append([]string(nil), value.Args...)
	value.Environment = append([]string(nil), value.Environment...)
	value.Stdin = append([]byte(nil), value.Stdin...)
	return value
}

func cloneGitRunResult(value GitRunResult) GitRunResult {
	value.Stdout = append([]byte(nil), value.Stdout...)
	value.Stderr = append([]byte(nil), value.Stderr...)
	return value
}

var (
	_ GitRunner    = (*fakeGitRunner)(nil)
	_ GitStore     = (*fakeGitStore)(nil)
	_ ApplyLock    = (*fakeApplyLock)(nil)
	_ Filesystem   = (*fakeFilesystem)(nil)
	_ Clock        = (*fakeClock)(nil)
	_ RandomSource = (*fakeRandomSource)(nil)
)
