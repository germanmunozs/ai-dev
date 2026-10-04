package workflow

import "testing"

func TestRecognizedAcceptsExactlySevenCanonicalStates(t *testing.T) {
	for _, s := range []State{Draft, Specified, Planned, Tasked, Implemented, Approved, Abandoned} {
		got, ok := Recognized(string(s))
		if !ok || got != s {
			t.Fatalf("Recognized(%q) = (%q, %v), want (%q, true)", s, got, ok, s)
		}
	}
}

func TestRecognizedRejectsNonExactValuesWithoutNormalization(t *testing.T) {
	for _, value := range []string{"Planned", " planned ", "planned ", "PLANNED", "", "   ", "reviewing", "plan"} {
		if got, ok := Recognized(value); ok || got != "" {
			t.Fatalf("Recognized(%q) = (%q, %v), want (\"\", false)", value, got, ok)
		}
	}
}

func TestTerminalOnlyForApprovedAndAbandoned(t *testing.T) {
	want := map[State]bool{
		Draft: false, Specified: false, Planned: false, Tasked: false, Implemented: false,
		Approved: true, Abandoned: true,
	}
	for s, terminal := range want {
		if got := Terminal(s); got != terminal {
			t.Fatalf("Terminal(%q) = %v, want %v", s, got, terminal)
		}
	}
}
