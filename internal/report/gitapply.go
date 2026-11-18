package report

import (
	"fmt"
	"io"
	"strings"

	"ai-dev/internal/gitmanager"
)

// WriteGitApplyJSON writes exactly one GitApplyOutput object.
func WriteGitApplyJSON(w io.Writer, result gitmanager.GitApplyResult) error {
	data, err := gitmanager.EncodeApplyOutput(result)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

// WriteGitApplyText writes one stable block rendered from the validated
// result. Secondary failures and findings carry only their structured,
// redacted fields.
func WriteGitApplyText(w io.Writer, result gitmanager.GitApplyResult) error {
	if err := result.Validate(); err != nil {
		return err
	}
	var b strings.Builder
	line := func(label, format string, args ...any) {
		fmt.Fprintf(&b, "%-15s %s\n", label+":", fmt.Sprintf(format, args...))
	}
	line("status", "%s", result.Status)
	if result.Reason != "" {
		line("reason", "%s", result.Reason)
	}
	line("apply", "%s", result.ApplyID)
	if result.AuthorizationID != "" {
		line("authorization", "%s (%s)", result.AuthorizationID, result.DurableAuthorizationState)
	}
	if result.PlanDigest != "" {
		line("plan digest", "%s", result.PlanDigest)
	}
	for _, id := range result.CompletedOperationIDs() {
		line("completed", "%s", id)
	}
	if result.FailedOperationID != "" {
		line("failed", "%s", result.FailedOperationID)
	}
	for _, id := range result.IndeterminateOperationIDs() {
		line("indeterminate", "%s", id)
	}
	for _, failure := range result.SecondaryFailures() {
		line("release failure", "%s %s (ownership verified: %t)", failure.ResourceRole, failure.FailureClass, failure.OwnershipVerified)
	}
	for _, finding := range result.Findings() {
		line("finding", "%s %s", finding.Category, finding.Path.DisplayPathUTF8)
	}
	if result.RecoveryRequired {
		line("recovery", "supervised reconciliation required")
	}
	line("exit", "%d", result.ExitCode)
	_, err := io.WriteString(w, b.String())
	return err
}
