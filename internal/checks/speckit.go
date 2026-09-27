package checks

import (
	"context"
	"io/fs"
	"strings"

	"ai-dev/internal/doctor"
)

const specKitManifestPath = ".specify/integrations/speckit.manifest.json"

type SpecKitCheck struct{ root fs.FS }

func NewSpecKitCheck(root fs.FS) SpecKitCheck { return SpecKitCheck{root: root} }

func (SpecKitCheck) ID() string { return "speckit-initialized" }

func (c SpecKitCheck) Run(context.Context) doctor.CheckResult {
	result := doctor.CheckResult{ID: c.ID(), Name: "Spec Kit Initialization", Area: "Repository"}
	if issues := ValidateManifestFiles(c.root, specKitManifestPath); len(issues) != 0 {
		result.Status = doctor.Fail
		result.Explanation = "The core Spec Kit scaffold is incomplete: " + strings.Join(issues, "; ") + "."
		result.Remediation = "Initialize or repair the core Spec Kit scaffold in this repository."
		return result
	}
	result.Status = doctor.Pass
	return result
}
