package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/germanmunozs/ai-dev/internal/conventionalcommit"
)

// ConventionalCommitValidationSchema is the schema of the validator output
// (data-model §16).
const ConventionalCommitValidationSchema = "ai-dev.conventional-commit-validation/v1"

type conventionalCommitFooter struct {
	Token     string `json:"token"`
	Separator string `json:"separator"`
	Value     string `json:"value"`
}

type conventionalCommitOutput struct {
	SchemaVersion            string                     `json:"schema_version"`
	Valid                    bool                       `json:"valid"`
	Type                     string                     `json:"type,omitempty"`
	Scope                    string                     `json:"scope,omitempty"`
	Description              string                     `json:"description,omitempty"`
	BodyLines                []string                   `json:"body_lines"`
	Footers                  []conventionalCommitFooter `json:"footers"`
	BreakingChangeRecognized bool                       `json:"breaking_change_recognized"`
	Diagnostics              []string                   `json:"diagnostics"`
}

func conventionalCommitModel(v conventionalcommit.Validation) (conventionalCommitOutput, error) {
	if v.Valid == (len(v.Diagnostics) > 0) || (!v.Valid && v.Breaking) {
		return conventionalCommitOutput{}, fmt.Errorf("inconsistent Conventional Commit validation")
	}
	out := conventionalCommitOutput{
		SchemaVersion: ConventionalCommitValidationSchema, Valid: v.Valid, Type: v.Type, Scope: v.Scope, Description: v.Description,
		BodyLines: []string{}, Footers: []conventionalCommitFooter{}, BreakingChangeRecognized: v.Breaking,
		Diagnostics: append([]string{}, v.Diagnostics...),
	}
	if v.Body != "" {
		out.BodyLines = strings.Split(v.Body, "\n")
	}
	for _, footer := range v.Footers {
		out.Footers = append(out.Footers, conventionalCommitFooter{Token: footer.Token, Separator: footer.Separator, Value: footer.Value})
	}
	return out, nil
}

// WriteConventionalCommitJSON writes exactly one validation object.
func WriteConventionalCommitJSON(w io.Writer, v conventionalcommit.Validation) error {
	out, err := conventionalCommitModel(v)
	if err != nil {
		return err
	}
	data, err := json.Marshal(out)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

// WriteConventionalCommitText writes one stable block. Validity and
// breaking-change recognition are reported separately.
func WriteConventionalCommitText(w io.Writer, v conventionalcommit.Validation) error {
	out, err := conventionalCommitModel(v)
	if err != nil {
		return err
	}
	var b strings.Builder
	if !out.Valid {
		b.WriteString("conventional commit: invalid\n")
		for _, diagnostic := range out.Diagnostics {
			fmt.Fprintf(&b, "diagnostic:     %s\n", diagnostic)
		}
	} else {
		b.WriteString("conventional commit: valid\n")
		fmt.Fprintf(&b, "type:           %s\n", out.Type)
		if out.Scope != "" {
			fmt.Fprintf(&b, "scope:          %s\n", out.Scope)
		}
		fmt.Fprintf(&b, "description:    %s\n", out.Description)
		fmt.Fprintf(&b, "body lines:     %d\n", len(out.BodyLines))
		for _, footer := range out.Footers {
			fmt.Fprintf(&b, "footer:         %s\n", footer.Token)
		}
		fmt.Fprintf(&b, "breaking:       %t\n", out.BreakingChangeRecognized)
	}
	_, err = io.WriteString(w, b.String())
	return err
}
