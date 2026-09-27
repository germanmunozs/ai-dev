package checks

import (
	"context"
	"fmt"
	"io/fs"
	"regexp"
	"strings"

	"ai-dev/internal/doctor"
)

const constitutionPath = ".specify/memory/constitution.md"

var (
	placeholderPattern = regexp.MustCompile(`\[[A-Z][A-Z0-9_]*\]`)
	versionPattern     = regexp.MustCompile(`(?m)\*\*Version\*\*:[ \t]*\S+`)
	ratifiedPattern    = regexp.MustCompile(`(?m)\*\*Ratified\*\*:[ \t]*\S+`)
)

type ConstitutionCheck struct{ root fs.FS }

func NewConstitutionCheck(root fs.FS) ConstitutionCheck { return ConstitutionCheck{root: root} }

func (ConstitutionCheck) ID() string { return "constitution" }

func (c ConstitutionCheck) Run(context.Context) (result doctor.CheckResult) {
	result = doctor.CheckResult{ID: c.ID(), Name: "Project Constitution", Area: "Repository"}
	defer func() {
		if recovered := recover(); recovered != nil {
			result.Status = doctor.Fail
			result.Explanation = fmt.Sprintf("Project constitution could not be inspected because of an unexpected filesystem failure: %v.", recovered)
			result.Remediation = "Verify .specify/memory/constitution.md is readable and complete."
		}
	}()
	data, err := fs.ReadFile(c.root, constitutionPath)
	reason := ""
	switch {
	case err != nil:
		reason = "the constitution file is missing or unreadable"
	case strings.TrimSpace(string(data)) == "":
		reason = "the constitution file is empty"
	case placeholderPattern.Match(data):
		reason = "the constitution contains unresolved template placeholders"
	case !versionPattern.Match(data):
		reason = "the constitution has no non-blank Version value"
	case !ratifiedPattern.Match(data):
		reason = "the constitution has no non-blank Ratified value"
	}
	if reason != "" {
		result.Status = doctor.Fail
		result.Explanation = "Project constitution is incomplete: " + reason + "."
		result.Remediation = "Complete .specify/memory/constitution.md, including Version and Ratified metadata."
		return result
	}
	result.Status = doctor.Pass
	return result
}
