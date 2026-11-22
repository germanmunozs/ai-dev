package checks

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/germanmunozs/ai-dev/internal/doctor"
)

const integrationRecordPath = ".specify/integration.json"

type integrationRecord struct {
	Installed []string `json:"installed_integrations"`
}

type ManifestIntegrationCheck struct {
	integration, id, name string
	root                  fs.FS
}

func NewManifestIntegrationCheck(integration, id, name string, root fs.FS) ManifestIntegrationCheck {
	return ManifestIntegrationCheck{integration: integration, id: id, name: name, root: root}
}

func (c ManifestIntegrationCheck) ID() string { return c.id }

func (c ManifestIntegrationCheck) Run(context.Context) (result doctor.CheckResult) {
	result = doctor.CheckResult{ID: c.id, Name: c.name, Area: "Repository"}
	defer func() {
		if recovered := recover(); recovered != nil {
			result.Status = doctor.Fail
			result.Explanation = fmt.Sprintf("The %s integration could not be inspected because of an unexpected filesystem failure: %v.", c.integration, recovered)
			result.Remediation = fmt.Sprintf("Verify or reinstall the %s Spec Kit integration.", c.integration)
		}
	}()
	issues := make([]string, 0)
	data, err := fs.ReadFile(c.root, integrationRecordPath)
	if err != nil {
		issues = append(issues, fmt.Sprintf("installation record %s is missing or unreadable", integrationRecordPath))
	} else {
		var record integrationRecord
		if err := json.Unmarshal(data, &record); err != nil {
			issues = append(issues, fmt.Sprintf("installation record %s is malformed", integrationRecordPath))
		} else if !slices.Contains(record.Installed, c.integration) {
			issues = append(issues, fmt.Sprintf("installation record does not list %s in installed_integrations", c.integration))
		}
	}
	manifestPath := fmt.Sprintf(".specify/integrations/%s.manifest.json", c.integration)
	issues = append(issues, ValidateManifestFiles(c.root, manifestPath)...)
	if len(issues) != 0 {
		result.Status = doctor.Fail
		result.Explanation = fmt.Sprintf("The %s integration is incomplete: %s.", c.integration, strings.Join(issues, "; "))
		result.Remediation = fmt.Sprintf("Reinstall or repair the %s Spec Kit integration.", c.integration)
		return result
	}
	result.Status = doctor.Pass
	return result
}
