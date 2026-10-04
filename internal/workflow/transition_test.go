package workflow

import "testing"

var allStates = []State{Draft, Specified, Planned, Tasked, Implemented, Approved, Abandoned}

// validEdges is the complete FR-004 table, restated independently of Allowed.
var validEdges = map[[2]State]bool{
	{Draft, Specified}:       true,
	{Draft, Abandoned}:       true,
	{Specified, Planned}:     true,
	{Specified, Abandoned}:   true,
	{Planned, Tasked}:        true,
	{Planned, Abandoned}:     true,
	{Tasked, Implemented}:    true,
	{Tasked, Abandoned}:      true,
	{Implemented, Approved}:  true,
	{Implemented, Abandoned}: true,
}

func TestAllowedMatchesTransitionRuleSetExhaustively(t *testing.T) {
	if len(validEdges) != 10 {
		t.Fatalf("expected ten valid edges, got %d", len(validEdges))
	}
	for _, from := range allStates {
		for _, to := range allStates {
			if got, want := Allowed(from, to), validEdges[[2]State{from, to}]; got != want {
				t.Fatalf("Allowed(%q, %q) = %v, want %v", from, to, got, want)
			}
		}
	}
}

func TestAllowedRejectsRepresentativeInvalidMoves(t *testing.T) {
	cases := []struct {
		name     string
		from, to State
	}{
		{"skip-ahead", Specified, Tasked},
		{"backward", Implemented, Specified},
		{"same-state", Planned, Planned},
		{"terminal approved", Approved, Abandoned},
		{"terminal abandoned", Abandoned, Draft},
	}
	for _, tc := range cases {
		if Allowed(tc.from, tc.to) {
			t.Fatalf("%s: Allowed(%q, %q) = true, want false", tc.name, tc.from, tc.to)
		}
	}
	for _, to := range allStates {
		if Allowed(Approved, to) || Allowed(Abandoned, to) {
			t.Fatalf("terminal origin allowed a transition to %q", to)
		}
	}
}
