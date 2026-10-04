package workflow

import "testing"

var invalidStructuralIDs = []string{"..", "../x", "a/../../x", "/etc/passwd", "a/b", "."}

func validStateReports() []StateReport {
	var reports []StateReport
	for _, s := range allStates {
		reports = append(reports, StateReport{FeatureID: testFeature, State: s, Terminal: Terminal(s), Outcome: StateReported})
	}
	return append(reports,
		StateReport{FeatureID: testFeature, Outcome: FeatureUnknown},
		StateReport{FeatureID: testFeature, Outcome: StateInvalid},
	)
}

func validTransitionReports() []TransitionReport {
	var reports []TransitionReport
	for _, from := range allStates {
		for _, to := range allStates {
			outcome := TransitionInvalid
			if Allowed(from, to) {
				outcome = TransitionValid
			}
			reports = append(reports, TransitionReport{FeatureID: testFeature, CurrentState: from, SuppliedTarget: string(to), Target: to, Outcome: outcome})
		}
	}
	return append(reports,
		TransitionReport{FeatureID: testFeature, SuppliedTarget: "reviewing", Outcome: TargetUnsupported},
		TransitionReport{FeatureID: testFeature, SuppliedTarget: "Planned", Outcome: TargetUnsupported},
		TransitionReport{FeatureID: testFeature, Outcome: FeatureUnknown},
		TransitionReport{FeatureID: testFeature, Outcome: StateInvalid},
	)
}

func TestStateReportValidAcceptsEveryApprovedCombination(t *testing.T) {
	for _, r := range validStateReports() {
		if !r.Valid() {
			t.Fatalf("Valid() = false for approved report %+v", r)
		}
	}
}

func TestTransitionReportValidAcceptsEveryApprovedCombination(t *testing.T) {
	for _, r := range validTransitionReports() {
		if !r.Valid() {
			t.Fatalf("Valid() = false for approved report %+v", r)
		}
	}
}

func TestStateReportValidRejectsMutations(t *testing.T) {
	reported := StateReport{FeatureID: testFeature, State: Planned, Terminal: false, Outcome: StateReported}
	mutations := map[string]StateReport{
		"noncanonical state":               {FeatureID: testFeature, State: "Planned", Outcome: StateReported},
		"empty state":                      {FeatureID: testFeature, Outcome: StateReported},
		"terminal mismatch true":           {FeatureID: testFeature, State: Planned, Terminal: true, Outcome: StateReported},
		"terminal mismatch false":          {FeatureID: testFeature, State: Approved, Terminal: false, Outcome: StateReported},
		"blank feature id":                 {FeatureID: "", State: Planned, Outcome: StateReported},
		"whitespace feature id":            {FeatureID: "   ", State: Planned, Outcome: StateReported},
		"feature_unknown with state":       {FeatureID: testFeature, State: Planned, Outcome: FeatureUnknown},
		"feature_unknown with terminal":    {FeatureID: testFeature, Terminal: true, Outcome: FeatureUnknown},
		"state_invalid with state":         {FeatureID: testFeature, State: Draft, Outcome: StateInvalid},
		"state_invalid with terminal":      {FeatureID: testFeature, Terminal: true, Outcome: StateInvalid},
		"feature_unknown blank feature id": {FeatureID: " ", Outcome: FeatureUnknown},
		"transition outcome":               {FeatureID: testFeature, State: Planned, Outcome: TransitionValid},
		"target_unsupported outcome":       {FeatureID: testFeature, Outcome: TargetUnsupported},
		"empty outcome":                    {FeatureID: testFeature, State: Planned},
		"unknown outcome":                  {FeatureID: testFeature, State: Planned, Outcome: "state_reported "},
	}
	if !reported.Valid() {
		t.Fatal("baseline report must be valid")
	}
	for name, r := range mutations {
		if r.Valid() {
			t.Fatalf("%s: Valid() = true for %+v", name, r)
		}
	}
}

func TestTransitionReportValidRejectsMutations(t *testing.T) {
	mutations := map[string]TransitionReport{
		"noncanonical current":                 {FeatureID: testFeature, CurrentState: "Planned", SuppliedTarget: "tasked", Target: Tasked, Outcome: TransitionValid},
		"empty current":                        {FeatureID: testFeature, SuppliedTarget: "tasked", Target: Tasked, Outcome: TransitionValid},
		"noncanonical target":                  {FeatureID: testFeature, CurrentState: Planned, SuppliedTarget: "Tasked", Target: "Tasked", Outcome: TransitionInvalid},
		"supplied/target mismatch":             {FeatureID: testFeature, CurrentState: Planned, SuppliedTarget: "tasked ", Target: Tasked, Outcome: TransitionValid},
		"empty supplied target":                {FeatureID: testFeature, CurrentState: Planned, Target: Tasked, Outcome: TransitionValid},
		"valid outcome for disallowed edge":    {FeatureID: testFeature, CurrentState: Planned, SuppliedTarget: "approved", Target: Approved, Outcome: TransitionValid},
		"invalid outcome for allowed edge":     {FeatureID: testFeature, CurrentState: Planned, SuppliedTarget: "tasked", Target: Tasked, Outcome: TransitionInvalid},
		"blank feature id":                     {FeatureID: "", CurrentState: Planned, SuppliedTarget: "tasked", Target: Tasked, Outcome: TransitionValid},
		"target_unsupported canonical target":  {FeatureID: testFeature, SuppliedTarget: "planned", Outcome: TargetUnsupported},
		"target_unsupported empty target":      {FeatureID: testFeature, Outcome: TargetUnsupported},
		"target_unsupported whitespace target": {FeatureID: testFeature, SuppliedTarget: "  ", Outcome: TargetUnsupported},
		"target_unsupported with current":      {FeatureID: testFeature, CurrentState: Planned, SuppliedTarget: "reviewing", Outcome: TargetUnsupported},
		"target_unsupported with target":       {FeatureID: testFeature, SuppliedTarget: "reviewing", Target: Planned, Outcome: TargetUnsupported},
		"target_unsupported blank feature id":  {FeatureID: "", SuppliedTarget: "reviewing", Outcome: TargetUnsupported},
		"feature_unknown with supplied target": {FeatureID: testFeature, SuppliedTarget: "reviewing", Outcome: FeatureUnknown},
		"feature_unknown with current":         {FeatureID: testFeature, CurrentState: Planned, Outcome: FeatureUnknown},
		"feature_unknown with target":          {FeatureID: testFeature, Target: Planned, Outcome: FeatureUnknown},
		"state_invalid with supplied target":   {FeatureID: testFeature, SuppliedTarget: "planned", Outcome: StateInvalid},
		"state_invalid with current":           {FeatureID: testFeature, CurrentState: Planned, Outcome: StateInvalid},
		"state_invalid with target":            {FeatureID: testFeature, Target: Planned, Outcome: StateInvalid},
		"state_reported outcome":               {FeatureID: testFeature, Outcome: StateReported},
		"empty outcome":                        {FeatureID: testFeature},
	}
	for name, r := range mutations {
		if r.Valid() {
			t.Fatalf("%s: Valid() = true for %+v", name, r)
		}
	}
}

func TestReportValidStructuralFeatureIDRules(t *testing.T) {
	for _, id := range invalidStructuralIDs {
		strictState := []StateReport{
			{FeatureID: id, State: Planned, Terminal: false, Outcome: StateReported},
			{FeatureID: id, Outcome: StateInvalid},
		}
		for _, r := range strictState {
			if r.Valid() {
				t.Fatalf("Valid() = true for %+v with structurally invalid feature ID", r)
			}
		}
		strictTransition := []TransitionReport{
			{FeatureID: id, CurrentState: Planned, SuppliedTarget: "tasked", Target: Tasked, Outcome: TransitionValid},
			{FeatureID: id, CurrentState: Planned, SuppliedTarget: "approved", Target: Approved, Outcome: TransitionInvalid},
			{FeatureID: id, Outcome: StateInvalid},
		}
		for _, r := range strictTransition {
			if r.Valid() {
				t.Fatalf("Valid() = true for %+v with structurally invalid feature ID", r)
			}
		}
		if r := (StateReport{FeatureID: id, Outcome: FeatureUnknown}); !r.Valid() {
			t.Fatalf("Valid() = false for %+v; feature_unknown may carry a structurally invalid ID", r)
		}
		if r := (TransitionReport{FeatureID: id, Outcome: FeatureUnknown}); !r.Valid() {
			t.Fatalf("Valid() = false for %+v; feature_unknown may carry a structurally invalid ID", r)
		}
		if r := (TransitionReport{FeatureID: id, SuppliedTarget: "reviewing", Outcome: TargetUnsupported}); !r.Valid() {
			t.Fatalf("Valid() = false for %+v; target_unsupported precedes feature-ID validation", r)
		}
	}
}
