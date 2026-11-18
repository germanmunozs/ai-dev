//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"fmt"
	"testing"
)

// T110/AC-006/AC-009/AC-018/AC-021/AC-026/AC-032: a failure is injected
// before and after every durable store step of a complete issue-and-apply
// attempt: attempt record, issuance, every authorization CAS, every audit
// publication (mutation intent, operation start and result, release,
// prepared result, terminal CAS), the apply lock and its release, and the
// result record. Each case must keep these invariants:
//
//   - the result is valid and its exit agrees with its status;
//   - operations ran in plan order, each at most once, and never after
//     the session stopped (no retry, no rollback: the executor has no
//     other entrypoint);
//   - the durable authorization agrees with the returned state, unless the
//     result is state_indeterminate because the store outcome is unknown;
//   - a later consumption with the same ID runs nothing again once intent
//     may be durable, and the audit trail never violates its order (the
//     fake store refuses every out-of-order publication).
func TestFaultMatrix(t *testing.T) {
	baseline := newApplyFixture(t, basicNewRepo())
	if result := baseline.issue(operator()); result.Status != ApplyStatusApplied {
		t.Fatalf("baseline %+v", result)
	}
	calls := baseline.store.callsSnapshot()
	operations := len(baseline.plan.DerivedOperations())
	faults := []struct {
		name  string
		err   error
		after bool
	}{
		{"fails", errInjected, false},
		{"unverifiable", ErrStoreOutcomeUnverifiable, false},
		{"unverifiable after its effect", ErrStoreOutcomeUnverifiable, true},
	}
	cases := 0
	for index, call := range calls {
		for _, fault := range faults {
			name := fmt.Sprintf("%02d %s %s", index, call, fault.name)
			t.Run(name, func(t *testing.T) {
				f := newApplyFixture(t, basicNewRepo())
				if fault.after {
					f.store.faults.failAfter(index, fault.err)
				} else {
					f.store.faults.failBefore(index, fault.err)
				}
				result := f.issue(operator())
				checkFaultResult(t, f, result, operations)
				executed := len(f.executor.executed)
				durable, issued := f.durable(t)
				// Recovery is never a retry: a later consumption runs the plan
				// again only when the authorization is still cleanly issued.
				f.store.faults = faultPlan{}
				later := f.consume()
				switch {
				case issued && durable.State == AuthorizationStateIssued:
					if executed != 0 && later.Status == ApplyStatusApplied {
						t.Fatalf("an authorization left issued after executing %d operations was consumed again", executed)
					}
				default:
					if len(f.executor.executed) != executed {
						t.Fatalf("a later attempt re-executed operations: %v", f.executor.executed)
					}
					if later.Status != ApplyStatusRefused && later.Status != ApplyStatusAlreadyApplied {
						t.Fatalf("later attempt %+v", later)
					}
				}
			})
			cases++
		}
	}
	if cases < 3*20 {
		t.Fatalf("the matrix covers only %d cases over %d store calls", cases, len(calls))
	}
}

func checkFaultResult(t *testing.T, f *applyFixture, result GitApplyResult, operations int) {
	t.Helper()
	if err := result.Validate(); err != nil {
		t.Fatalf("invalid result %+v: %v", result, err)
	}
	if (result.ExitCode == 0) != ((result.Status == ApplyStatusApplied || result.Status == ApplyStatusAlreadyApplied) &&
		len(result.SecondaryFailures()) == 0 && result.Reason != ReasonOutputFailed) {
		t.Fatalf("exit %d for %s/%s", result.ExitCode, result.Status, result.Reason)
	}
	var want []OperationKind
	for _, operation := range f.plan.DerivedOperations() {
		want = append(want, operation.Kind)
	}
	executed := f.executor.executed
	if len(executed) > operations || !equalLists(executed, want[:len(executed)]) {
		t.Fatalf("operations ran out of plan order: %v", executed)
	}
	if result.Status == ApplyStatusRefused && len(executed) > 0 && result.Reason != ReasonAuditUnavailable {
		t.Fatalf("a refusal after %d operations: %+v", len(executed), result)
	}
	durable, ok := f.durable(t)
	if !ok {
		if result.DurableAuthorizationState != "" {
			t.Fatalf("result claims state %s for a missing authorization", result.DurableAuthorizationState)
		}
		return
	}
	if result.DurableAuthorizationState != "" && result.DurableAuthorizationState != durable.State &&
		result.Status != ApplyStatusIndeterminate {
		t.Fatalf("result state %s, durable %s, status %s/%s", result.DurableAuthorizationState, durable.State, result.Status, result.Reason)
	}
	if durable.State == AuthorizationStateApplied && result.Status != ApplyStatusApplied && result.Status != ApplyStatusIndeterminate {
		t.Fatalf("durable applied but result %s", result.Status)
	}
}

// T110/AC-009/AC-028: every operation can end failed or indeterminate.
// Execution stops there, later operations are reported not started, the
// status follows only the verified evidence, and nothing is retried.
func TestOperationFaultMatrix(t *testing.T) {
	for _, outcome := range []OperationOutcome{OperationFailed, OperationIndeterminate} {
		probe := newApplyFixture(t, basicNewRepo())
		for ordinal := range probe.plan.DerivedOperations() {
			for name, evidence := range map[string]struct {
				evidence ApplyEvidence
				status   ApplyStatus
			}{
				"zero effects": {ApplyEvidence{ZeroEffectsProven: true}, ApplyStatusFailed},
				"some effects": {ApplyEvidence{VerifiedEffects: []Condition{{Kind: ConditionRepositoryKind, Expected: "normal"}}}, ApplyStatusPartiallyApplied},
				"unverifiable": {ApplyEvidence{EffectsUnverifiable: true}, ApplyStatusIndeterminate},
			} {
				t.Run(fmt.Sprintf("%s at %d with %s", outcome, ordinal, name), func(t *testing.T) {
					f := newApplyFixture(t, basicNewRepo())
					f.executor.outcomes = map[int]OperationOutcome{ordinal: outcome}
					f.executor.evidence = evidence.evidence
					result := f.issue(operator())
					want := evidence.status
					// An indeterminate operation's own effect is unknown, and a
					// completed earlier operation counts as an effect (§10), so
					// neither can prove zero effects.
					if outcome == OperationIndeterminate || (want == ApplyStatusFailed && ordinal > 0) {
						want = ApplyStatusIndeterminate
					}
					if result.Status != want || result.Reason != ReasonOperationFailed || result.ExitCode != 1 {
						t.Fatalf("result %s/%s exit %d, want %s", result.Status, result.Reason, result.ExitCode, want)
					}
					if len(f.executor.executed) != ordinal+1 || len(result.NotStartedOperationIDs()) != len(f.plan.DerivedOperations())-ordinal-1 {
						t.Fatalf("executed %v, not started %v", f.executor.executed, result.NotStartedOperationIDs())
					}
					if later := f.consume(); later.Status != ApplyStatusRefused || len(f.executor.executed) != ordinal+1 {
						t.Fatalf("later attempt %+v after %v", later, f.executor.executed)
					}
				})
			}
		}
	}
}
