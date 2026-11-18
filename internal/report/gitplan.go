package report

import (
	"fmt"
	"io"
	"strings"
	"time"

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
	for _, line := range initializationLines(plan) {
		b.WriteString(line + "\n")
	}
	for _, operation := range plan.DerivedOperations() {
		fmt.Fprintf(&b, "operation %-4d %s %s\n", operation.Ordinal, operation.Kind, operation.TargetIdentity)
	}
	for _, line := range commitLines(plan) {
		b.WriteString(line + "\n")
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
	var b strings.Builder
	fmt.Fprintf(&b, "refused: %s: %s\n", rejection.Reason, rejection.Detail)
	// A finding names only the path and the category (F008-FR-085).
	for _, finding := range rejection.Findings {
		fmt.Fprintf(&b, "finding:        %s %s\n", displayPath(finding.Path), finding.Category)
	}
	if len(rejection.Findings) > 0 {
		fmt.Fprintf(&b, "screening:      %s\n", gitmanager.SecretScreeningLimitation)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// commitLines render the planned initial commit: the exact objects and
// ref, the frozen identities and dates, every message line as approved,
// and one exact approval argument per file, ignored files expressly in
// their reinforced form (F008-FR-090, 097, 103).
func commitLines(plan gitmanager.GitInitializationPlan) []string {
	intent := plan.ImmutableIntent
	if !intent.InitialCommit {
		return nil
	}
	var lines []string
	for _, operation := range plan.DerivedOperations() {
		switch operation.Kind {
		case gitmanager.OperationKindInitialCommit:
			lines = append(lines, "commit:         "+strings.TrimPrefix(operation.TargetIdentity, "commit:")+" (unsigned)")
		case gitmanager.OperationKindInitialRefCAS:
			lines = append(lines, "initial ref:    "+strings.TrimPrefix(operation.TargetIdentity, "ref:")+" (created only if absent)")
		}
	}
	lines = append(lines,
		fmt.Sprintf("author:         %s <%s> %s", intent.Author.Name, intent.Author.Email, intent.AuthorTimestamp.Format(time.RFC3339)),
		fmt.Sprintf("committer:      %s <%s> %s", intent.Committer.Name, intent.Committer.Email, intent.CommitterTimestamp.Format(time.RFC3339)))
	for _, line := range strings.Split(strings.TrimSuffix(string(intent.CommitMessageBytes()), "\n"), "\n") {
		lines = append(lines, "message:        | "+line)
	}
	for _, file := range intent.AuthorizedFiles() {
		tuple := file.PathUTF8 + "|" + file.Mode + "|" + string(file.ContentSHA256) + "|blob:" + string(file.BlobOID)
		if file.Ignored {
			lines = append(lines, "approve:        --approve-ignored-file "+shellQuote(tuple)+"  (ignored file)")
		} else {
			lines = append(lines, "approve:        --approve-file "+shellQuote(tuple))
		}
	}
	return append(lines, "screening:      "+gitmanager.SecretScreeningLimitation)
}

// shellQuote renders one argument for a POSIX shell.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func displayPath(path gitmanager.ObservedPath) string {
	if path.DisplayPathUTF8 != "" {
		return path.DisplayPathUTF8
	}
	return "base64url:" + path.ReversiblePathBytesBase64URL
}
