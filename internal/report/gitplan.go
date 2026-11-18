package report

import (
	"fmt"
	"io"
	"strings"

	"ai-dev/internal/gitmanager"
)

// WriteGitPlanJSON writes exactly one GitPlanOutput object.
func WriteGitPlanJSON(w io.Writer, result gitmanager.PlanResult, planPath string) error {
	data, err := gitmanager.EncodePlanOutput(result, planPath)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

// WriteGitPlanText writes one stable block describing the plan; it claims
// no authorization.
func WriteGitPlanText(w io.Writer, result gitmanager.PlanResult, planPath string) error {
	plan := result.Plan
	if err := plan.Validate(); err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "plan:           %s\n", planPath)
	fmt.Fprintf(&b, "plan digest:    %s\n", plan.PlanDigest)
	fmt.Fprintf(&b, "strategy:       %s\n", plan.ImmutableIntent.Strategy)
	for _, operation := range plan.DerivedOperations() {
		fmt.Fprintf(&b, "operation %-4d %s %s\n", operation.Ordinal, operation.Kind, operation.TargetIdentity)
	}
	for _, tuple := range gitmanager.ApprovalTuples(plan.ImmutableIntent) {
		fmt.Fprintf(&b, "approval:       %s\n", tuple)
	}
	for _, pending := range result.PendingDependencies {
		fmt.Fprintf(&b, "pending:        %s\n", pending)
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(&b, "warning:        %s\n", warning)
	}
	b.WriteString("authorization:  none; apply needs a separate explicit authorization\n")
	_, err := io.WriteString(w, b.String())
	return err
}

// WriteGitPlanRejection writes the refusal as one GitPlanRejectionOutput
// object or one text line.
func WriteGitPlanRejection(w io.Writer, format string, rejection *gitmanager.PlanRejection, warnings []string) error {
	if format == "json" {
		data, err := gitmanager.EncodePlanRejection(rejection, warnings)
		if err != nil {
			return err
		}
		_, err = w.Write(append(data, '\n'))
		return err
	}
	_, err := fmt.Fprintf(w, "refused: %s: %s\n", rejection.Reason, rejection.Detail)
	return err
}
