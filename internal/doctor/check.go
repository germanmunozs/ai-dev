// Package doctor defines the technology-agnostic readiness domain.
package doctor

import "context"

// Status is the binary outcome of a check or report.
type Status uint8

const (
	Fail Status = iota
	Pass
)

// CheckResult is the result of one readiness check.
type CheckResult struct {
	ID          string
	Name        string
	Area        string
	Status      Status
	Explanation string
	Remediation string
}

// Check is an independently executable readiness inspection.
type Check interface {
	ID() string
	Run(context.Context) CheckResult
}

// Report contains all check results in execution order and their aggregate status.
type Report struct {
	Checks  []CheckResult
	Overall Status
}

// RunAll executes every check in order without short-circuiting.
func RunAll(ctx context.Context, checks []Check) Report {
	report := Report{Overall: Pass}
	for _, check := range checks {
		result := check.Run(ctx)
		report.Checks = append(report.Checks, result)
		if result.Status == Fail {
			report.Overall = Fail
		}
	}
	return report
}
