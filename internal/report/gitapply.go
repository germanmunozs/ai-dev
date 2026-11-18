package report

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

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
	for _, effect := range result.VerifiedEffects() {
		line("effect", "%s", conditionText(effect))
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

// conditionText renders a verified effect with its decoded subject. Path
// and ref bytes that are not valid UTF-8 keep their reversible encoding.
func conditionText(c gitmanager.Condition) string {
	parts := []string{string(c.Kind)}
	if c.Subject != "" {
		subject := c.Subject
		if raw, err := gitmanager.DecodeReversiblePath(c.Subject); err == nil && utf8.Valid(raw) && c.Kind != gitmanager.ConditionConfigAbsent &&
			c.Kind != gitmanager.ConditionConfigValue && c.Kind != gitmanager.ConditionObjectType {
			subject = string(raw)
		}
		parts = append(parts, subject)
	}
	if c.Expected != "" {
		parts = append(parts, c.Expected)
	}
	return strings.Join(parts, " ")
}

// initializationLines render, for a plan that creates a repository, the
// frozen toolchain and the exact F008-FR-190 initialization it runs, so
// the operator sees both before authorizing the plan.
func initializationLines(plan gitmanager.GitInitializationPlan) []string {
	intent := plan.ImmutableIntent
	if !intent.InitializeRepository {
		return nil
	}
	tool := intent.ToolIdentity
	lines := []string{fmt.Sprintf("toolchain:      git %s (%s) %s, exec path %s", tool.Version, tool.InvocationMode, tool.InvocationPath, tool.ExecPath)}
	for _, operation := range plan.DerivedOperations() {
		if operation.Kind != gitmanager.OperationKindRepositoryInit {
			continue
		}
		argv := operation.ExactArgv()
		for i, arg := range argv {
			if arg == "init" {
				argv = argv[i:]
				break
			}
		}
		quoted := make([]string, len(argv))
		for i, arg := range argv {
			quoted[i] = arg
			if arg == "" || strings.ContainsAny(arg, " \t\n\"'\\") || !utf8.ValidString(arg) {
				quoted[i] = strconv.Quote(arg)
			}
		}
		lines = append(lines, fmt.Sprintf("initialize:     git %s", strings.Join(quoted, " ")),
			fmt.Sprintf("formats:        object %s, refs %s", intent.ObjectFormat, intent.RefFormat))
	}
	return lines
}
