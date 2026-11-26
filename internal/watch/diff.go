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
			HumanGate: evidence(prev, cur),
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

// evidence attaches the recorded Human Gate entry only when the previous
// observation read the history, a new entry appeared since, and its
// to_state is the new state; a change without one has none (F015-FR-008,
// analysis U5). The entry is reported as recorded, never as an outcome.
func evidence(prev, cur featureView) *HumanGate {
	if prev.historyLen < 0 || cur.historyLen <= prev.historyLen || cur.last == nil || !cur.state.Recognized ||
		string(cur.last.ToState) != cur.state.State {
		return nil
	}
	e := cur.last
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
