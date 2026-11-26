package watch

import (
	"bytes"
	"slices"
	"time"
)

// featureEvents compares two observations of one feature and returns its
// events in the contract's order: state, artifacts, next step
// (contracts/watch.md, F015-FR-007).
func featureEvents(id string, prev, cur featureView) []Event {
	var events []Event
	if !bytes.Equal(prev.state.JSON, cur.state.JSON) {
		events = append(events, Event{Type: WorkflowStateChanged, Data: StateChange{
			Feature: id, From: stateOf(prev), To: stateOf(cur), Outcome: cur.state.Outcome, State: cur.state.JSON,
			HumanGate: evidence(id, prev, cur),
		}})
	}
	if !slices.EqualFunc(prev.artifacts, cur.artifacts, sameArtifact) {
		events = append(events, Event{Type: ArtifactsChanged, Data: ArtifactsChange{Feature: id, Artifacts: cur.artifacts}})
	}
	if !bytes.Equal(prev.next, cur.next) {
		events = append(events, Event{Type: NextStepChanged, Data: NextChange{Feature: id, Next: cur.next}})
	}
	return events
}

func stateOf(v featureView) *string {
	if !v.state.Recognized {
		return nil
	}
	state := v.state.State
	return &state
}

// evidence attaches the last recorded Human Gate entry only when the
// previous observation read the history, the history only grew since (the
// old entries are unchanged), every new entry is this feature's own and
// they chain from the previous state to the new one. Anything else, such as
// a record copied from another feature or a rewritten history, has none
// (F015-FR-008, analysis U5, review F3). The entry is reported as recorded,
// never as an outcome.
func evidence(id string, prev, cur featureView) *HumanGate {
	if prev.historyLen < 0 || cur.historyLen <= prev.historyLen || !prev.state.Recognized || !cur.state.Recognized ||
		!slices.EqualFunc(prev.history, cur.history[:prev.historyLen], sameEntry) {
		return nil
	}
	from := prev.state.State
	for _, entry := range cur.history[prev.historyLen:] {
		if entry.FeatureID != id || string(entry.FromState) != from {
			return nil
		}
		from = string(entry.ToState)
	}
	if from != cur.state.State {
		return nil
	}
	e := cur.history[len(cur.history)-1]
	g := &HumanGate{Decision: string(e.Decision), Approver: e.Approver, FromState: string(e.FromState),
		ToState: string(e.ToState), DecidedAt: e.DecidedAt.UTC().Format(time.RFC3339)}
	if e.ReasonSet {
		reason := e.Reason
		g.Reason = &reason
	}
	return g
}

func sameArtifact(a, b Artifact) bool {
	return a.Name == b.Name && a.Status == b.Status && a.Modified == b.Modified &&
		(a.Size == nil) == (b.Size == nil) && (a.Size == nil || *a.Size == *b.Size)
}

// checkChanges lists the checks whose status changed, in id order; a check
// that appeared or disappeared has null on that side.
func checkChanges(prev, cur []CheckStatus) []CheckChange {
	before := map[string]CheckStatus{}
	for _, c := range prev {
		before[c.ID] = c
	}
	after := map[string]CheckStatus{}
	for _, c := range cur {
		after[c.ID] = c
	}
	var ids []string
	for id := range before {
		ids = append(ids, id)
	}
	for id := range after {
		if _, known := before[id]; !known {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	var changes []CheckChange
	for _, id := range ids {
		b, hadBefore := before[id]
		a, hasNow := after[id]
		if hadBefore && hasNow && b.Status == a.Status {
			continue
		}
		change := CheckChange{ID: id, Remediation: a.Remediation}
		if hadBefore {
			status := b.Status
			change.From = &status
		}
		if hasNow {
			status := a.Status
			change.To = &status
		}
		changes = append(changes, change)
	}
	return changes
}
