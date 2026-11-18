package report

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"ai-dev/internal/gitmanager"
)

// WriteGitStrategyReportJSON writes exactly one GitStrategyReport object.
func WriteGitStrategyReportJSON(w io.Writer, r gitmanager.GitStrategyReport) error {
	data, err := gitmanager.EncodeStrategyReport(r)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

// WriteGitStrategyReportText writes one stable block. It states that the
// report grants nothing.
func WriteGitStrategyReportText(w io.Writer, r gitmanager.GitStrategyReport) error {
	if _, err := gitmanager.EncodeStrategyReport(r); err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "strategy:       %s (from %s)\n", r.SelectedStrategy, r.Source)
	if r.GitFlowDialect != "" {
		fmt.Fprintf(&b, "dialect:        %s\n", r.GitFlowDialect)
	}
	names := make([]string, 0, len(r.BranchParameters))
	for name := range r.BranchParameters {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(&b, "branch:         %s = %s\n", name, r.BranchParameters[name])
	}
	for _, evidence := range r.DetectedEvidence {
		fmt.Fprintf(&b, "detected:       gitflow %s\n", evidence.Classification)
	}
	fmt.Fprintf(&b, "compatibility:  %s\n", r.Compatibility)
	for _, pending := range r.PendingDependencies {
		fmt.Fprintf(&b, "pending:        %s\n", pending)
	}
	for _, warning := range r.Warnings {
		fmt.Fprintf(&b, "warning:        %s\n", warning)
	}
	b.WriteString("authority:      none; this report grants no mutation authority\n")
	_, err := io.WriteString(w, b.String())
	return err
}
