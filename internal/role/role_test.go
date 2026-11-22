package role

import "testing"

func TestRecognized(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  Role
		ok    bool
	}{
		{"specification", "specification", Specification, true},
		{"planning", "planning", Planning, true},
		{"implementation", "implementation", Implementation, true},
		{"review", "review", Review, true},
		{"remediation", "remediation", Remediation, true},

		{"empty string", "", "", false},
		{"whitespace only", "   ", "", false},
		{"tab and newline whitespace", "\t\n", "", false},

		{"wrong case leading capital", "Specification", "", false},
		{"wrong case all caps", "IMPLEMENTATION", "", false},
		{"wrong case mixed", "Review", "", false},

		{"alias developer", "developer", "", false},
		{"alias coder", "coding", "", false},
		{"alias fix", "fix", "", false},
		{"alias qa", "qa", "", false},

		{"abbreviation spec", "spec", "", false},
		{"abbreviation impl", "impl", "", false},
		{"abbreviation plan", "plan", "", false},
		{"abbreviation rev", "rev", "", false},
		{"abbreviation remediate", "remediate", "", false},

		{"arbitrary value", "nonsense", "", false},
		{"leading whitespace", " review", "", false},
		{"trailing whitespace", "review ", "", false},
		{"plural", "specifications", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Recognized(tt.value)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("Recognized(%q) = (%q, %v), want (%q, %v)", tt.value, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestExactlyFiveCanonicalRolesEachRecognizeThemselves(t *testing.T) {
	roles := []Role{Specification, Planning, Implementation, Review, Remediation}
	if len(roles) != 5 {
		t.Fatalf("len(roles) = %d, want 5", len(roles))
	}
	seen := make(map[Role]bool, len(roles))
	for _, r := range roles {
		if seen[r] {
			t.Fatalf("duplicate canonical role constant: %v", r)
		}
		seen[r] = true
		got, ok := Recognized(string(r))
		if !ok || got != r {
			t.Fatalf("Recognized(%q) = (%q, %v), want (%q, true)", r, got, ok, r)
		}
	}
}
