package gitmanager

import (
	"context"
	"errors"
	"io"
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

type fakeGitStore struct {
	mu      sync.Mutex
	Calls   []GitStoreRequest
	Results []GitStoreResult
	Errors  map[int]error
}

func newFakeGitStore(results []GitStoreResult, configuredErrors map[int]error) *fakeGitStore {
	clonedResults := make([]GitStoreResult, len(results))
	for index, result := range results {
		clonedResults[index] = cloneGitStoreResult(result)
	}
	clonedErrors := make(map[int]error, len(configuredErrors))
	for index, err := range configuredErrors {
		clonedErrors[index] = err
	}
	return &fakeGitStore{Results: clonedResults, Errors: clonedErrors}
}

func (f *fakeGitStore) Execute(_ context.Context, request GitStoreRequest) (GitStoreResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, cloneGitStoreRequest(request))
	index := len(f.Calls) - 1
	var result GitStoreResult
	if index < len(f.Results) {
		result = cloneGitStoreResult(f.Results[index])
	}
	return result, f.Errors[index]
}

func (f *fakeGitStore) callsSnapshot() []GitStoreRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	calls := make([]GitStoreRequest, len(f.Calls))
	for index, call := range f.Calls {
		calls[index] = cloneGitStoreRequest(call)
	}
	return calls
}

type fakeFilesystem struct {
	mu      sync.Mutex
	Calls   []FilesystemRequest
	Results []FilesystemResult
	Errors  map[int]error
}

func (f *fakeFilesystem) Execute(_ context.Context, request FilesystemRequest) (FilesystemResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, cloneFilesystemRequest(request))
	index := len(f.Calls) - 1
	var result FilesystemResult
	if index < len(f.Results) {
		result = cloneFilesystemResult(f.Results[index])
	}
	return result, f.Errors[index]
}

func (f *fakeFilesystem) callsSnapshot() []FilesystemRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	calls := make([]FilesystemRequest, len(f.Calls))
	for index, call := range f.Calls {
		calls[index] = cloneFilesystemRequest(call)
	}
	return calls
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

type fakeReporter struct {
	mu     sync.Mutex
	Calls  []ReportRequest
	Errors map[int]error
}

func (f *fakeReporter) Report(_ context.Context, request ReportRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, cloneReportRequest(request))
	return f.Errors[len(f.Calls)-1]
}

func (f *fakeReporter) callsSnapshot() []ReportRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	calls := make([]ReportRequest, len(f.Calls))
	for index, call := range f.Calls {
		calls[index] = cloneReportRequest(call)
	}
	return calls
}

func TestPortFakesInjectFaultsDeterministically(t *testing.T) {
	ctx := context.Background()
	runner := &fakeGitRunner{Errors: map[int]error{1: errInjected}}
	_, firstErr := runner.Run(ctx, GitInvocation{Args: []string{"version"}})
	_, secondErr := runner.Run(ctx, GitInvocation{Args: []string{"status"}})
	if firstErr != nil || !errors.Is(secondErr, errInjected) {
		t.Fatalf("runner errors = %v, %v", firstErr, secondErr)
	}

	store := &fakeGitStore{Errors: map[int]error{0: errInjected}}
	if _, err := store.Execute(ctx, GitStoreRequest{Operation: "test"}); !errors.Is(err, errInjected) {
		t.Fatalf("store error = %v", err)
	}

	filesystem := &fakeFilesystem{Errors: map[int]error{0: errInjected}}
	if _, err := filesystem.Execute(ctx, FilesystemRequest{Operation: "test"}); !errors.Is(err, errInjected) {
		t.Fatalf("filesystem error = %v", err)
	}

	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	clock := newFakeClock(now)
	if got := clock.Now(); !got.Equal(now) {
		t.Fatalf("clock = %v", got)
	}

	random := newFakeRandomSource([]byte{1, 2})
	random.injectErrorAt(2, errInjected)
	buffer := make([]byte, 2)
	if n, err := random.Read(buffer); n != 2 || !errors.Is(err, errInjected) {
		t.Fatalf("random read = %d, %v", n, err)
	}

	reporter := &fakeReporter{Errors: map[int]error{0: errInjected}}
	if err := reporter.Report(ctx, ReportRequest{Kind: "test"}); !errors.Is(err, errInjected) {
		t.Fatalf("reporter error = %v", err)
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

func TestPortFakesReturnPartialResultsWithErrors(t *testing.T) {
	ctx := context.Background()
	runner := &fakeGitRunner{Results: []GitRunResult{{Stdout: []byte("partial"), ExitCode: 1}}, Errors: map[int]error{0: errInjected}}
	if result, err := runner.Run(ctx, GitInvocation{}); string(result.Stdout) != "partial" || result.ExitCode != 1 || !errors.Is(err, errInjected) {
		t.Fatalf("runner result=%+v err=%v", result, err)
	}
	store := &fakeGitStore{Results: []GitStoreResult{{Applied: true, Payload: []byte("persisted effect")}}, Errors: map[int]error{0: errInjected}}
	if result, err := store.Execute(ctx, GitStoreRequest{}); !result.Applied || string(result.Payload) != "persisted effect" || !errors.Is(err, errInjected) {
		t.Fatalf("store result=%+v err=%v", result, err)
	}
	filesystem := &fakeFilesystem{Results: []FilesystemResult{{Applied: true, Data: []byte("written")}}, Errors: map[int]error{0: errInjected}}
	if result, err := filesystem.Execute(ctx, FilesystemRequest{}); !result.Applied || string(result.Data) != "written" || !errors.Is(err, errInjected) {
		t.Fatalf("filesystem result=%+v err=%v", result, err)
	}
}

func TestPortFakesAreRaceSafe(t *testing.T) {
	const calls = 64
	ctx := context.Background()
	runner := &fakeGitRunner{}
	store := &fakeGitStore{}
	filesystem := &fakeFilesystem{}
	reporter := &fakeReporter{}
	random := newFakeRandomSource(make([]byte, calls))
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
			_, _ = store.Execute(ctx, GitStoreRequest{Operation: "call", Payload: []byte{byte(index)}})
			_, _ = filesystem.Execute(ctx, FilesystemRequest{Operation: "call", Data: []byte{byte(index)}})
			_ = reporter.Report(ctx, ReportRequest{Kind: "call", Payload: []byte{byte(index)}})
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
		"runner": len(runner.callsSnapshot()), "store": len(store.callsSnapshot()),
		"filesystem": len(filesystem.callsSnapshot()), "reporter": len(reporter.callsSnapshot()),
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

func TestPortFakesDefensivelyCopyInputsAndResults(t *testing.T) {
	ctx := context.Background()
	input := []byte("request")
	configuredResult := []byte("result")
	configuredErrors := map[int]error{}
	store := newFakeGitStore([]GitStoreResult{{Payload: configuredResult}}, configuredErrors)
	result, err := store.Execute(ctx, GitStoreRequest{Payload: input})
	if err != nil {
		t.Fatal(err)
	}
	input[0] = 'X'
	result.Payload[0] = 'X'
	configuredResult[0] = 'X'
	configuredErrors[0] = errInjected
	calls := store.callsSnapshot()
	if string(calls[0].Payload) != "request" {
		t.Fatalf("recorded input changed: %q", calls[0].Payload)
	}
	if string(store.Results[0].Payload) != "result" {
		t.Fatalf("configured result changed through returned slice: %q", store.Results[0].Payload)
	}
	mapConfiguration := map[int]error{}
	mapStore := newFakeGitStore(nil, mapConfiguration)
	mapConfiguration[0] = errInjected
	if _, err := mapStore.Execute(ctx, GitStoreRequest{}); err != nil {
		t.Fatalf("fake retained caller-owned error map: %v", err)
	}

	request := GitInvocation{Args: []string{"status"}, Environment: []string{"A=B"}, Stdin: []byte("stdin")}
	runner := &fakeGitRunner{}
	_, _ = runner.Run(ctx, request)
	request.Args[0], request.Environment[0], request.Stdin[0] = "changed", "changed", 'X'
	recorded := runner.callsSnapshot()[0]
	if recorded.Args[0] != "status" || recorded.Environment[0] != "A=B" || string(recorded.Stdin) != "stdin" {
		t.Fatalf("recorded invocation changed: %+v", recorded)
	}
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

func cloneGitStoreRequest(value GitStoreRequest) GitStoreRequest {
	value.Payload = append([]byte(nil), value.Payload...)
	return value
}

func cloneGitStoreResult(value GitStoreResult) GitStoreResult {
	value.Payload = append([]byte(nil), value.Payload...)
	return value
}

func cloneFilesystemRequest(value FilesystemRequest) FilesystemRequest {
	value.Data = append([]byte(nil), value.Data...)
	return value
}

func cloneFilesystemResult(value FilesystemResult) FilesystemResult {
	value.Data = append([]byte(nil), value.Data...)
	return value
}

func cloneReportRequest(value ReportRequest) ReportRequest {
	value.Payload = append([]byte(nil), value.Payload...)
	return value
}

var (
	_ GitRunner    = (*fakeGitRunner)(nil)
	_ GitStore     = (*fakeGitStore)(nil)
	_ Filesystem   = (*fakeFilesystem)(nil)
	_ Clock        = (*fakeClock)(nil)
	_ RandomSource = (*fakeRandomSource)(nil)
	_ Reporter     = (*fakeReporter)(nil)
)
