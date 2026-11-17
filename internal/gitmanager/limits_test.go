package gitmanager

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
)

// The bounds of F008-FR-051 and the reason each one maps to when exceeded.
var limitCatalogue = []struct {
	kind   LimitKind
	max    int64
	reason Reason
}{
	{LimitPlanBytes, 8 << 20, ReasonPlanInvalid},
	{LimitPlanNesting, 32, ReasonPlanInvalid},
	{LimitContainerMembers, 16384, ReasonPlanInvalid},
	{LimitStringBytes, 1 << 20, ReasonPlanInvalid},
	{LimitPathBytes, 4096, ReasonPlanInvalid},
	{LimitPathComponentBytes, 255, ReasonPlanInvalid},
	{LimitAuthorizedFiles, 4096, ReasonPlanInvalid},
	{LimitDerivedOperations, 8192, ReasonPlanInvalid},
	{LimitPostconditions, 16384, ReasonPlanInvalid},
	{LimitAuthorizedFileBytes, 64 << 20, ReasonRepositoryStateIncompatible},
	{LimitAggregateAuthorizedBytes, 512 << 20, ReasonRepositoryStateIncompatible},
	{LimitCommitMessageBytes, 1 << 20, ReasonCommitMessageInvalid},
	{LimitSourceBytes, 1 << 20, ReasonAttributesIncompatible},
	{LimitAggregateSourceBytes, 16 << 20, ReasonAttributesIncompatible},
	{LimitIncludeDepth, 16, ReasonAttributesIncompatible},
}

func TestLimitCatalogueBoundaries(t *testing.T) {
	for _, limit := range limitCatalogue {
		t.Run(string(limit.kind), func(t *testing.T) {
			if got := limit.kind.Max(); got != limit.max {
				t.Fatalf("max=%d want %d", got, limit.max)
			}
			if got := limit.kind.Reason(); got != limit.reason {
				t.Fatalf("reason=%q want %q", got, limit.reason)
			}
			for _, observed := range []int64{0, limit.max - 1, limit.max} {
				if err := CheckLimit(limit.kind, observed); err != nil {
					t.Errorf("%d rejected: %v", observed, err)
				}
			}
			err := CheckLimit(limit.kind, limit.max+1)
			var limitErr *LimitError
			if !errors.As(err, &limitErr) || limitErr.Kind != limit.kind || limitErr.Reason != limit.reason ||
				limitErr.Max != limit.max || limitErr.Observed != limit.max+1 || !errors.Is(err, errLimitExceeded) {
				t.Fatalf("one over: %#v", err)
			}
			if message := err.Error(); !strings.Contains(message, string(limit.kind)) || !strings.Contains(message, string(limit.reason)) {
				t.Fatalf("message %q lacks the bound or its reason", message)
			}
		})
	}
	if err := CheckLimit("bogus", 0); err == nil || errors.Is(err, errLimitExceeded) {
		t.Fatalf("unknown limit kind: %v", err)
	}
	if err := CheckLimit(LimitPlanBytes, -1); err == nil || errors.Is(err, errLimitExceeded) {
		t.Fatalf("negative observation: %v", err)
	}
}

// Captured stdout/stderr has no reason of its own: overflow carries the
// closed reason of the source or operation being evaluated.
func TestCapturedStreamUsesGoverningReason(t *testing.T) {
	if LimitCapturedStreamBytes.Max() != 4<<20 || LimitCapturedStreamBytes.Reason() != "" {
		t.Fatal("captured stream bound or reason")
	}
	if err := CheckCapturedStream(4<<20, ReasonOperationFailed); err != nil {
		t.Fatalf("exact boundary rejected: %v", err)
	}
	var limitErr *LimitError
	err := CheckCapturedStream(4<<20+1, ReasonAttributesIncompatible)
	if !errors.As(err, &limitErr) || limitErr.Reason != ReasonAttributesIncompatible || limitErr.Kind != LimitCapturedStreamBytes {
		t.Fatalf("one over: %#v", err)
	}
	for _, governing := range []Reason{"", "bogus"} {
		if err := CheckCapturedStream(1, governing); err == nil || errors.Is(err, errLimitExceeded) {
			t.Errorf("governing reason %q accepted: %v", governing, err)
		}
	}
	if err := CheckLimit(LimitCapturedStreamBytes, 1); err == nil {
		t.Error("captured stream checked without a governing reason")
	}
}

// Aggregate bounds are accounted while streaming: the exact total passes,
// the first unit over fails, and the failure is permanent.
func TestBudgetAccountsAggregates(t *testing.T) {
	budget, err := NewBudget(LimitAggregateSourceBytes)
	if err != nil {
		t.Fatal(err)
	}
	chunk := int64(1 << 20)
	for i := 0; i < 16; i++ {
		if err := budget.Add(chunk); err != nil {
			t.Fatalf("chunk %d rejected: %v", i, err)
		}
	}
	if budget.Used() != 16<<20 {
		t.Fatalf("used=%d", budget.Used())
	}
	var limitErr *LimitError
	if err := budget.Add(1); !errors.As(err, &limitErr) || limitErr.Observed != 16<<20+1 || limitErr.Reason != ReasonAttributesIncompatible {
		t.Fatalf("one over: %#v", err)
	}
	if err := budget.Add(0); !errors.Is(err, errLimitExceeded) {
		t.Fatalf("exceeded budget accepted more input: %v", err)
	}
	if err := budget.Add(-1); err == nil {
		t.Fatal("negative amount accepted")
	}
	if _, err := NewBudget(LimitCapturedStreamBytes); err == nil {
		t.Fatal("captured stream budget built without a governing reason")
	}
	stream, err := NewCapturedStreamBudget(ReasonOperationFailed)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Add(4<<20 + 1); !errors.As(err, &limitErr) || limitErr.Reason != ReasonOperationFailed {
		t.Fatalf("captured stream budget: %#v", err)
	}
	if _, err := NewCapturedStreamBudget("bogus"); err == nil {
		t.Fatal("unknown governing reason accepted")
	}
	// An overflow sum that would wrap is still an overflow.
	huge, _ := NewBudget(LimitAggregateAuthorizedBytes)
	if err := huge.Add(1 << 62); !errors.Is(err, errLimitExceeded) {
		t.Fatalf("huge amount: %v", err)
	}
	if err := huge.Add(1 << 62); !errors.Is(err, errLimitExceeded) {
		t.Fatalf("wrapped amount: %v", err)
	}
}

// countingReader records how many bytes were requested from an endless
// source, proving that a bounded read never retains more than max+1 bytes.
type countingReader struct{ read int64 }

func (r *countingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	r.read += int64(len(p))
	return len(p), nil
}

func TestBoundedReadNeverOverAllocates(t *testing.T) {
	exact, err := ReadBounded(strings.NewReader(strings.Repeat("a", 1<<20)), mustBudget(t, LimitCommitMessageBytes))
	if err != nil || len(exact) != 1<<20 {
		t.Fatalf("exact boundary: %d bytes, %v", len(exact), err)
	}
	over := bytes.Repeat([]byte("a"), 1<<20+1)
	var limitErr *LimitError
	if _, err := ReadBounded(bytes.NewReader(over), mustBudget(t, LimitCommitMessageBytes)); !errors.As(err, &limitErr) || limitErr.Reason != ReasonCommitMessageInvalid {
		t.Fatalf("one over: %#v", err)
	}
	endless := &countingReader{}
	if _, err := ReadBounded(endless, mustBudget(t, LimitSourceBytes)); !errors.Is(err, errLimitExceeded) {
		t.Fatalf("endless source: %v", err)
	}
	if endless.read > 1<<20+1 {
		t.Fatalf("read %d bytes from an endless source, bound is %d", endless.read, 1<<20)
	}
	// A shared budget makes per-source reads count toward the aggregate.
	aggregate := mustBudget(t, LimitAggregateSourceBytes)
	for i := 0; i < 16; i++ {
		if _, err := ReadBounded(io.LimitReader(&countingReader{}, 1<<20), aggregate); err != nil {
			t.Fatalf("source %d: %v", i, err)
		}
	}
	if _, err := ReadBounded(strings.NewReader("a"), aggregate); !errors.Is(err, errLimitExceeded) {
		t.Fatalf("aggregate overflow: %v", err)
	}
	failing := io.MultiReader(strings.NewReader("ab"), errReader{})
	if _, err := ReadBounded(failing, mustBudget(t, LimitSourceBytes)); !errors.Is(err, errInjected) {
		t.Fatalf("read error not returned: %v", err)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errInjected }

func TestPathByteLimits(t *testing.T) {
	component := strings.Repeat("c", 255)
	exact := strings.Repeat(component+"/", 16) // 4096 bytes, every component at 255
	if len(exact) != 4096 {
		t.Fatalf("fixture length %d", len(exact))
	}
	for name, path := range map[string]string{"exact total": exact, "exact component": component, "short": "a/b"} {
		if err := CheckPathBytes([]byte(path)); err != nil {
			t.Errorf("%s rejected: %v", name, err)
		}
	}
	var limitErr *LimitError
	if err := CheckPathBytes([]byte(exact + "e")); !errors.As(err, &limitErr) || limitErr.Kind != LimitPathBytes || limitErr.Reason != ReasonPlanInvalid {
		t.Fatalf("total one over: %#v", err)
	}
	if err := CheckPathBytes([]byte("a/" + component + "c")); !errors.As(err, &limitErr) || limitErr.Kind != LimitPathComponentBytes {
		t.Fatalf("component one over: %#v", err)
	}
	// Bytes, not characters: a 128-rune two-byte component is 256 bytes.
	if err := CheckPathBytes([]byte(strings.Repeat("é", 128))); !errors.As(err, &limitErr) || limitErr.Kind != LimitPathComponentBytes {
		t.Fatalf("multibyte component: %#v", err)
	}
}

func TestPlanDecodeLimitsMatchCatalogue(t *testing.T) {
	if PlanDecodeLimits.MaxBytes != int(LimitPlanBytes.Max()) || PlanDecodeLimits.MaxDepth != int(LimitPlanNesting.Max()) ||
		PlanDecodeLimits.MaxContainerMembers != int(LimitContainerMembers.Max()) || PlanDecodeLimits.MaxStringBytes != int(LimitStringBytes.Max()) {
		t.Fatalf("plan decode limits %+v diverge from the catalogue", PlanDecodeLimits)
	}
	if maxCanonicalDepth != LimitPlanNesting.Max() {
		t.Fatal("writer nesting bound diverges from the catalogue")
	}
	var limitErr *LimitError
	deep := strings.Repeat("[", 33) + strings.Repeat("]", 33)
	if _, err := UnmarshalCanonical([]byte(deep), PlanDecodeLimits); !errors.As(err, &limitErr) || limitErr.Kind != LimitPlanNesting || limitErr.Reason != ReasonPlanInvalid {
		t.Fatalf("nesting overflow: %#v", err)
	}
	long := `"` + strings.Repeat("a", 1<<20+1) + `"`
	if _, err := UnmarshalCanonical([]byte(long), PlanDecodeLimits); !errors.As(err, &limitErr) || limitErr.Kind != LimitStringBytes {
		t.Fatalf("string overflow: %#v", err)
	}
	exactString := `"` + strings.Repeat("a", 1<<20) + `"`
	if _, err := UnmarshalCanonical([]byte(exactString), PlanDecodeLimits); err != nil {
		t.Fatalf("exact string boundary rejected: %v", err)
	}
	items := "[" + strings.TrimSuffix(strings.Repeat("0,", 16385), ",") + "]"
	if _, err := UnmarshalCanonical([]byte(items), PlanDecodeLimits); !errors.As(err, &limitErr) || limitErr.Kind != LimitContainerMembers {
		t.Fatalf("item overflow: %#v", err)
	}
	exactItems := "[" + strings.TrimSuffix(strings.Repeat("0,", 16384), ",") + "]"
	if _, err := UnmarshalCanonical([]byte(exactItems), PlanDecodeLimits); err != nil {
		t.Fatalf("exact item boundary rejected: %v", err)
	}
	big := bytes.Repeat([]byte(" "), 8<<20+1)
	if _, err := UnmarshalCanonical(big, PlanDecodeLimits); !errors.As(err, &limitErr) || limitErr.Kind != LimitPlanBytes {
		t.Fatalf("byte overflow: %#v", err)
	}
}

// The model enforces the bounds it can see, so a value over a bound cannot
// be constructed and the caller receives the mapped reason.
func TestModelEnforcesBounds(t *testing.T) {
	var limitErr *LimitError
	expectLimit := func(t *testing.T, err error, kind LimitKind) {
		t.Helper()
		if !errors.As(err, &limitErr) || limitErr.Kind != kind {
			t.Fatalf("got %v, want %s limit", err, kind)
		}
	}

	t.Run("authorized files", func(t *testing.T) {
		in := validIntentInput(t)
		for i := len(in.AuthorizedFiles); i < 4096; i++ {
			in.AuthorizedFiles = append(in.AuthorizedFiles, validAuthorizedFile("f"+strconv.Itoa(i)))
		}
		if _, err := NewImmutableIntent(in); err != nil {
			t.Fatalf("exact file count rejected: %v", err)
		}
		in.AuthorizedFiles = append(in.AuthorizedFiles, validAuthorizedFile("extra"))
		_, err := NewImmutableIntent(in)
		expectLimit(t, err, LimitAuthorizedFiles)
	})
	t.Run("authorized file bytes", func(t *testing.T) {
		in := validIntentInput(t)
		in.AuthorizedFiles[0].ObservedIdentity.Size = 64 << 20
		if _, err := NewImmutableIntent(in); err != nil {
			t.Fatalf("exact file size rejected: %v", err)
		}
		in.AuthorizedFiles[0].ObservedIdentity.Size++
		_, err := NewImmutableIntent(in)
		expectLimit(t, err, LimitAuthorizedFileBytes)
	})
	t.Run("aggregate authorized bytes", func(t *testing.T) {
		in := validIntentInput(t)
		in.AuthorizedFiles = nil
		for i := 0; i < 8; i++ {
			file := validAuthorizedFile("f" + strconv.Itoa(i))
			file.ObservedIdentity.Size = 64 << 20
			in.AuthorizedFiles = append(in.AuthorizedFiles, file)
		}
		in.AuthorizedFiles[0] = validAuthorizedFile(ProjectConfigPath)
		in.AuthorizedFiles[0].ObservedIdentity.Size = 64 << 20
		if _, err := NewImmutableIntent(in); err != nil {
			t.Fatalf("exact aggregate rejected: %v", err)
		}
		in.AuthorizedFiles = append(in.AuthorizedFiles, validAuthorizedFile("one-more"))
		_, err := NewImmutableIntent(in)
		expectLimit(t, err, LimitAggregateAuthorizedBytes)
	})
	t.Run("commit message", func(t *testing.T) {
		in := validIntentInput(t)
		in.CommitMessageBytes = bytes.Repeat([]byte("m"), 1<<20)
		if _, err := NewImmutableIntent(in); err != nil {
			t.Fatalf("exact message rejected: %v", err)
		}
		in.CommitMessageBytes = append(in.CommitMessageBytes, '\n')
		_, err := NewImmutableIntent(in)
		expectLimit(t, err, LimitCommitMessageBytes)
	})
	t.Run("authorized path", func(t *testing.T) {
		in := validIntentInput(t)
		in.AuthorizedFiles = append(in.AuthorizedFiles, validAuthorizedFile(strings.Repeat("p", 256)))
		_, err := NewImmutableIntent(in)
		expectLimit(t, err, LimitPathComponentBytes)
	})
	t.Run("postconditions", func(t *testing.T) {
		in := validPlanInput(t)
		planLevel := 16384 - len(in.DerivedOperations[0].ExpectedPostconditions())
		in.ExpectedPostconditions = make([]Condition, planLevel)
		for i := range in.ExpectedPostconditions {
			in.ExpectedPostconditions[i] = Condition{Kind: ConditionHeadState, Expected: "unborn"}
		}
		sealPlanInput(t, &in)
		if _, err := NewGitInitializationPlan(in); err != nil {
			t.Fatalf("exact plan-wide postcondition total rejected: %v", err)
		}
		in.ExpectedPostconditions = append(in.ExpectedPostconditions, Condition{Kind: ConditionIndexEmpty})
		sealPlanInput(t, &in)
		_, err := NewGitInitializationPlan(in)
		expectLimit(t, err, LimitPostconditions)
	})
	t.Run("derived operations", func(t *testing.T) {
		in := validPlanInput(t)
		operation := in.DerivedOperations[0]
		in.DerivedOperations = make([]DerivedOperation, 8193)
		for i := range in.DerivedOperations {
			in.DerivedOperations[i] = operation
		}
		_, err := NewGitInitializationPlan(in)
		expectLimit(t, err, LimitDerivedOperations)
	})
}

func mustBudget(t testing.TB, kind LimitKind) *Budget {
	t.Helper()
	budget, err := NewBudget(kind)
	if err != nil {
		t.Fatal(err)
	}
	return budget
}
