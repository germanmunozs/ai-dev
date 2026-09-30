// Package role defines the vendor-neutral development-role model.
package role

// Role is a canonical development-role identifier, independent of any coding
// agent.
type Role string

const (
	Specification  Role = "specification"
	Planning       Role = "planning"
	Implementation Role = "implementation"
	Review         Role = "review"
	Remediation    Role = "remediation"
)

// Recognized reports whether value is exactly, case-sensitively, one of the
// five fixed canonical roles, with no normalization, aliasing, abbreviation,
// or fuzzy matching.
func Recognized(value string) (Role, bool) {
	switch Role(value) {
	case Specification, Planning, Implementation, Review, Remediation:
		return Role(value), true
	default:
		return "", false
	}
}
