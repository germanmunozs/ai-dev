package gitmanager

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
)

// LimitKind names one F008-FR-051 input bound. Reaching a bound exactly is
// valid; one unit or byte over it rejects with the bound's mapped reason.
type LimitKind string

const (
	LimitPlanBytes                LimitKind = "plan_bytes"
	LimitPlanNesting              LimitKind = "plan_nesting"
	LimitContainerMembers         LimitKind = "container_members"
	LimitStringBytes              LimitKind = "string_bytes"
	LimitPathBytes                LimitKind = "path_bytes"
	LimitPathComponentBytes       LimitKind = "path_component_bytes"
	LimitAuthorizedFiles          LimitKind = "authorized_files"
	LimitDerivedOperations        LimitKind = "derived_operations"
	LimitPostconditions           LimitKind = "postconditions"
	LimitAuthorizedFileBytes      LimitKind = "authorized_file_bytes"
	LimitAggregateAuthorizedBytes LimitKind = "aggregate_authorized_bytes"
	LimitCommitMessageBytes       LimitKind = "commit_message_bytes"
	LimitSourceBytes              LimitKind = "source_bytes"
	LimitAggregateSourceBytes     LimitKind = "aggregate_source_bytes"
	LimitIncludeDepth             LimitKind = "include_depth"
	LimitCapturedStreamBytes      LimitKind = "captured_stream_bytes"
)

// limitCatalog is the closed F008-FR-051 table. The path component bound is
// NAME_MAX, 255 bytes on both supported platforms. A captured stream has no
// reason of its own: overflow keeps the reason of the evaluated source or
// operation.
var limitCatalog = map[LimitKind]struct {
	max    int64
	reason Reason
}{
	LimitPlanBytes:                {8 << 20, ReasonPlanInvalid},
	LimitPlanNesting:              {32, ReasonPlanInvalid},
	LimitContainerMembers:         {16384, ReasonPlanInvalid},
	LimitStringBytes:              {1 << 20, ReasonPlanInvalid},
	LimitPathBytes:                {4096, ReasonPlanInvalid},
	LimitPathComponentBytes:       {255, ReasonPlanInvalid},
	LimitAuthorizedFiles:          {4096, ReasonPlanInvalid},
	LimitDerivedOperations:        {8192, ReasonPlanInvalid},
	LimitPostconditions:           {16384, ReasonPlanInvalid},
	LimitAuthorizedFileBytes:      {64 << 20, ReasonRepositoryStateIncompatible},
	LimitAggregateAuthorizedBytes: {512 << 20, ReasonRepositoryStateIncompatible},
	LimitCommitMessageBytes:       {1 << 20, ReasonCommitMessageInvalid},
	LimitSourceBytes:              {1 << 20, ReasonAttributesIncompatible},
	LimitAggregateSourceBytes:     {16 << 20, ReasonAttributesIncompatible},
	LimitIncludeDepth:             {16, ReasonAttributesIncompatible},
	LimitCapturedStreamBytes:      {4 << 20, ""},
}

// Max returns the bound, or 0 for an unknown kind.
func (k LimitKind) Max() int64 { return limitCatalog[k].max }

// Reason returns the closed reason an overflow maps to; it is empty for the
// captured-stream bound, whose reason the caller supplies.
func (k LimitKind) Reason() Reason { return limitCatalog[k].reason }

var errLimitExceeded = errors.New("input limit exceeded")

// LimitError reports an exceeded bound together with its mapped reason.
type LimitError struct {
	Kind     LimitKind
	Reason   Reason
	Max      int64
	Observed int64
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("%s: %s %d above %d (%s)", errLimitExceeded, e.Kind, e.Observed, e.Max, e.Reason)
}

func (e *LimitError) Unwrap() error { return errLimitExceeded }

// CheckLimit reports whether observed is within the bound of kind.
func CheckLimit(kind LimitKind, observed int64) error {
	return checkLimit(kind, observed, kind.Reason())
}

// CheckCapturedStream bounds one captured stdout or stderr stream, mapping
// an overflow to the governing reason of the evaluated source or operation.
func CheckCapturedStream(observed int64, governing Reason) error {
	if !governing.Valid() {
		return invalid("captured stream governing reason")
	}
	return checkLimit(LimitCapturedStreamBytes, observed, governing)
}

func checkLimit(kind LimitKind, observed int64, reason Reason) error {
	limit, known := limitCatalog[kind]
	if !known || reason == "" || observed < 0 {
		return invalid("limit check")
	}
	if observed > limit.max {
		return &LimitError{Kind: kind, Reason: reason, Max: limit.max, Observed: observed}
	}
	return nil
}

// Budget accounts one bound incrementally, so streamed input and aggregate
// totals are rejected at the first unit over the bound. Once exceeded, a
// budget accepts nothing more.
type Budget struct {
	kind   LimitKind
	reason Reason
	used   int64
	err    error
}

func NewBudget(kind LimitKind) (*Budget, error) {
	if _, known := limitCatalog[kind]; !known || kind.Reason() == "" {
		return nil, invalid("budget kind")
	}
	return &Budget{kind: kind, reason: kind.Reason()}, nil
}

// NewCapturedStreamBudget bounds one captured stream under the governing
// reason of the source or operation being evaluated.
func NewCapturedStreamBudget(governing Reason) (*Budget, error) {
	if !governing.Valid() {
		return nil, invalid("captured stream governing reason")
	}
	return &Budget{kind: LimitCapturedStreamBytes, reason: governing}, nil
}

// Add accounts n more units.
func (b *Budget) Add(n int64) error {
	if n < 0 {
		return invalid("budget amount")
	}
	if b.err != nil {
		return b.err
	}
	max := b.kind.Max()
	if n > max-b.used {
		// Saturate rather than wrap when reporting an enormous total.
		observed := int64(math.MaxInt64)
		if n <= math.MaxInt64-b.used {
			observed = b.used + n
		}
		b.err = &LimitError{Kind: b.kind, Reason: b.reason, Max: max, Observed: observed}
		return b.err
	}
	b.used += n
	return nil
}

// Used returns the units accounted so far.
func (b *Budget) Used() int64 { return b.used }

// remaining is how many more units fit within the bound.
func (b *Budget) remaining() int64 { return b.kind.Max() - b.used }

// ReadBounded reads r to its end, charging every byte to budget. It never
// requests more than one byte beyond what the budget still allows, so an
// overlong or endless source is rejected without retaining it.
func ReadBounded(r io.Reader, budget *Budget) ([]byte, error) {
	if budget.err != nil {
		return nil, budget.err
	}
	var buffer bytes.Buffer
	n, err := buffer.ReadFrom(io.LimitReader(r, budget.remaining()+1))
	if err != nil {
		return nil, err
	}
	if err := budget.Add(n); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// CheckPathBytes applies the path bounds to exact path bytes: 4,096 bytes in
// total and NAME_MAX bytes per slash-separated component.
func CheckPathBytes(path []byte) error {
	if err := CheckLimit(LimitPathBytes, int64(len(path))); err != nil {
		return err
	}
	for _, component := range bytes.Split(path, []byte("/")) {
		if err := CheckLimit(LimitPathComponentBytes, int64(len(component))); err != nil {
			return err
		}
	}
	return nil
}
