package doctor

import (
	"context"
	"reflect"
	"testing"
)

type stubCheck struct {
	id     string
	result CheckResult
	runs   *int
}

func (c stubCheck) ID() string { return c.id }

func (c stubCheck) Run(context.Context) CheckResult {
	*c.runs++
	return c.result
}

func TestStatusHasExactlyPassAndFailValues(t *testing.T) {
	if Pass == Fail {
		t.Fatal("Pass and Fail must be distinct")
	}
	if got := []Status{Fail, Pass}; !reflect.DeepEqual(got, []Status{0, 1}) {
		t.Fatalf("Status values = %v, want exactly Pass and Fail", got)
	}
	var zero Status
	if zero != Fail {
		t.Fatalf("zero Status = %v, want Fail", zero)
	}
}

func TestRunAllPreservesOrderRunsEveryCheckAndDerivesOverall(t *testing.T) {
	tests := []struct {
		name string
		in   []CheckResult
		want Status
	}{
		{name: "all pass", in: []CheckResult{{ID: "first-check", Status: Pass}, {ID: "second-check", Status: Pass}}, want: Pass},
		{name: "one fail", in: []CheckResult{{ID: "first-check", Status: Fail, Explanation: "broken", Remediation: "fix it"}, {ID: "second-check", Status: Pass}}, want: Fail},
		{name: "empty", in: nil, want: Pass},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runs := 0
			checks := make([]Check, 0, len(tt.in))
			for _, result := range tt.in {
				checks = append(checks, stubCheck{id: result.ID, result: result, runs: &runs})
			}
			report := RunAll(context.Background(), checks)
			if runs != len(checks) {
				t.Fatalf("ran %d checks, want %d", runs, len(checks))
			}
			if !reflect.DeepEqual(report.Checks, tt.in) {
				t.Fatalf("results = %#v, want %#v", report.Checks, tt.in)
			}
			if report.Overall != tt.want {
				t.Fatalf("overall = %v, want %v", report.Overall, tt.want)
			}
		})
	}
}

func TestCheckResultDiagnosticContract(t *testing.T) {
	pass := CheckResult{ID: "stable-id", Name: "Name", Area: "Area", Status: Pass}
	if pass.Explanation != "" || pass.Remediation != "" {
		t.Fatal("Pass diagnostics must be empty")
	}
	fail := CheckResult{ID: "stable-id", Name: "Name", Area: "Area", Status: Fail, Explanation: "specific problem", Remediation: "specific fix"}
	if fail.Explanation == "" || fail.Remediation == "" {
		t.Fatal("Fail diagnostics must be non-empty")
	}
}

func TestRunAllKeepsExactlyOneOrderedResultAfterFailure(t *testing.T) {
	runs := 0
	checks := []Check{
		stubCheck{id: "first-check", runs: &runs, result: CheckResult{ID: "first-check", Status: Fail, Explanation: "bad", Remediation: "fix"}},
		stubCheck{id: "second-check", runs: &runs, result: CheckResult{ID: "second-check", Status: Pass}},
		stubCheck{id: "third-check", runs: &runs, result: CheckResult{ID: "third-check", Status: Pass}},
	}
	report := RunAll(context.Background(), checks)
	if runs != 3 || len(report.Checks) != 3 || report.Overall != Fail {
		t.Fatalf("unexpected report/runs: %#v, %d", report, runs)
	}
	for i, check := range checks {
		if report.Checks[i].ID != check.ID() {
			t.Fatalf("result %d = %q, want %q", i, report.Checks[i].ID, check.ID())
		}
	}
}
