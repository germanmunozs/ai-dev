package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/doctor"
)

func TestWriteTextReadyReport(t *testing.T) {
	report := doctor.Report{Overall: doctor.Pass, Checks: []doctor.CheckResult{
		{ID: "git-tool", Name: "Git", Area: "Environment", Status: doctor.Pass},
		{ID: "speckit-tool", Name: "Spec Kit", Area: "Environment", Status: doctor.Pass},
		{ID: "claude-code-tool", Name: "Claude Code", Area: "Environment", Status: doctor.Pass},
		{ID: "codex-cli-tool", Name: "Codex CLI", Area: "Environment", Status: doctor.Pass},
		{ID: "git-repository", Name: "Current Git Repository", Area: "Repository", Status: doctor.Pass},
		{ID: "speckit-initialized", Name: "Spec Kit Initialization", Area: "Repository", Status: doctor.Pass},
		{ID: "constitution", Name: "Project Constitution", Area: "Repository", Status: doctor.Pass},
		{ID: "claude-integration", Name: "Claude Spec Kit Integration", Area: "Repository", Status: doctor.Pass},
		{ID: "codex-integration", Name: "Codex Spec Kit Integration", Area: "Repository", Status: doctor.Pass},
	}}
	var out bytes.Buffer
	if err := WriteText(&out, report); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"Environment", "Repository", "Overall: Ready"} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q:\n%s", want, text)
		}
	}
	for _, check := range report.Checks {
		if !strings.Contains(text, check.Name) || !strings.Contains(text, "PASS") {
			t.Errorf("output missing PASS/name for %s:\n%s", check.ID, text)
		}
	}
}

func TestWriteTextFailureIncludesActionableDiagnostics(t *testing.T) {
	report := doctor.Report{Overall: doctor.Fail, Checks: []doctor.CheckResult{
		{ID: "codex-cli-tool", Name: "Codex CLI", Area: "Environment", Status: doctor.Fail, Explanation: "Codex was not found.", Remediation: "Install Codex."},
	}}
	var out bytes.Buffer
	if err := WriteText(&out, report); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"FAIL", "Codex CLI", "Explanation:", "Codex was not found.", "Remediation:", "Install Codex.", "Overall: Not Ready"} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q:\n%s", want, text)
		}
	}
}

func TestWriteTextMixedStatusesKeepCheckRowsCompact(t *testing.T) {
	report := doctor.Report{Overall: doctor.Fail, Checks: []doctor.CheckResult{
		{ID: "git-tool", Name: "Git", Area: "Environment", Status: doctor.Pass},
		{ID: "codex-cli-tool", Name: "Codex CLI", Area: "Environment", Status: doctor.Fail, Explanation: "Codex was not found.", Remediation: "Install Codex."},
		{ID: "claude-code-tool", Name: "Claude Code", Area: "Environment", Status: doctor.Pass},
	}}
	var out bytes.Buffer
	if err := WriteText(&out, report); err != nil {
		t.Fatal(err)
	}
	want := "Environment\n" +
		"  [PASS] Git\n" +
		"  [FAIL] Codex CLI\n" +
		"        Explanation: Codex was not found.\n" +
		"        Remediation: Install Codex.\n" +
		"  [PASS] Claude Code\n" +
		"\nOverall: Not Ready\n"
	if got := out.String(); got != want {
		t.Fatalf("output =\n%q\nwant =\n%q", got, want)
	}
}
