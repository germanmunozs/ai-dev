package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/germanmunozs/ai-dev/internal/version"
)

// VersionSchemaVersion identifies the machine-readable version report.
const VersionSchemaVersion = "ai-dev.version/v1"

// WriteVersionText writes the one-line version report (F009-FR-002).
func WriteVersionText(w io.Writer, info version.Info) error {
	details := make([]string, 0, 4)
	if info.Commit != "" {
		commit := info.Commit
		if len(commit) > 12 {
			commit = commit[:12]
		}
		details = append(details, "commit "+commit)
	}
	if info.Modified {
		details = append(details, "modified")
	}
	details = append(details, info.GoVersion, info.Platform)
	_, err := fmt.Fprintf(w, "ai-dev %s (%s)\n", info.Version, strings.Join(details, ", "))
	return err
}

// WriteVersionJSON writes exactly one machine-readable version report.
func WriteVersionJSON(w io.Writer, info version.Info) error {
	return json.NewEncoder(w).Encode(struct {
		SchemaVersion string `json:"schema_version"`
		Version       string `json:"version"`
		Commit        string `json:"commit,omitempty"`
		CommitTime    string `json:"commit_time,omitempty"`
		Modified      bool   `json:"modified,omitempty"`
		GoVersion     string `json:"go_version"`
		Platform      string `json:"platform"`
	}{VersionSchemaVersion, info.Version, info.Commit, info.CommitTime, info.Modified, info.GoVersion, info.Platform})
}
