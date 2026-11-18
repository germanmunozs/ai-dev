package gitmanager

import (
	"fmt"
	"io/fs"
	"strings"
)

// conditionResult is one condition evaluated against fresh state.
type conditionResult uint8

const (
	conditionHolds conditionResult = iota
	conditionFails
	conditionUnverifiable
)

// evaluateConditions evaluates every condition and returns the first that
// does not hold, preferring an unverifiable one: uncertainty is never
// reported as a plain failure.
func evaluateConditions(conditions []Condition, inspection GitInspection) (conditionResult, Condition) {
	worst, at := conditionHolds, Condition{}
	for _, condition := range conditions {
		if result := evaluateCondition(condition, inspection); result > worst {
			worst, at = result, condition
		}
	}
	return worst, at
}

// evaluateCondition evaluates one condition of the closed catalogue against
// a fresh inspection; path conditions and reflogs are observed directly
// beneath the physical project root. A condition the inspection does not
// observe is unverifiable, never assumed (F008-FR-166).
func evaluateCondition(c Condition, inspection GitInspection) conditionResult {
	o := inspection.Observation
	absent := o.RepositoryKind == RepositoryKindAbsent
	normal := o.RepositoryKind == RepositoryKindNormal
	known := func(holds bool) conditionResult {
		if holds {
			return conditionHolds
		}
		return conditionFails
	}
	switch c.Kind {
	case ConditionRepositoryAbsent:
		return known(absent)
	case ConditionRepositoryKind:
		return known(string(o.RepositoryKind) == c.Expected)
	case ConditionHeadState:
		return known(normal && string(o.Head.State) == c.Expected)
	case ConditionHeadSymbolicRef:
		ref, ok := decodedSubject(c)
		return orUnverifiable(ok, known(normal && string(o.Head.SymbolicRef.Bytes()) == ref))
	case ConditionRefAbsent, ConditionRefTarget:
		ref, ok := decodedSubject(c)
		if !ok {
			return conditionUnverifiable
		}
		if absent {
			return known(c.Kind == ConditionRefAbsent)
		}
		if o.RefsObservationState != ObservationObserved && o.RefsObservationState != ObservationObservedEmpty {
			return conditionUnverifiable
		}
		for _, observed := range o.Refs() {
			if string(observed.Name.Bytes()) == ref {
				return known(c.Kind == ConditionRefTarget && string(observed.TargetOID) == c.Expected)
			}
		}
		return known(c.Kind == ConditionRefAbsent)
	case ConditionReflogAbsent:
		ref, ok := decodedSubject(c)
		common := string(o.CommonDir.Bytes())
		if !ok || !normal || !strings.HasPrefix(common, "/") {
			return conditionUnverifiable
		}
		return presence(strings.TrimSuffix(common, "/")+"/logs/"+ref, false)
	case ConditionPathAbsent:
		path, ok := projectPath(inspection, c)
		return orUnverifiable(ok, presence(path, false))
	case ConditionPathIdentity, ConditionPathContent, ConditionPathMode:
		path, ok := projectPath(inspection, c)
		if !ok {
			return conditionUnverifiable
		}
		if presence(path, true) != conditionHolds {
			// A proven absence fails; anything else is unverifiable.
			return presence(path, true)
		}
		observed, ok := observePathIdentity(path)
		if !ok {
			return conditionUnverifiable
		}
		switch c.Kind {
		case ConditionPathIdentity:
			return known(observed.Identity.DeviceDecimal+":"+observed.Identity.InodeDecimal == c.Expected)
		case ConditionPathContent:
			return known(observed.Identity.Mode.IsRegular() && string(observed.ContentDigest) == c.Expected)
		default:
			return known(octalMode(observed.Identity.Mode) == c.Expected)
		}
	case ConditionIndexAbsent, ConditionIndexEmpty:
		if !normal || o.Index.Presence == IndexUnreadable {
			return conditionUnverifiable
		}
		if c.Kind == ConditionIndexAbsent {
			return known(o.Index.Presence == IndexAbsent)
		}
		return known(o.Index.Presence == IndexPresent && o.Index.EntryCountDecimal == "0")
	case ConditionLockFileAbsent:
		if !normal || o.Index.LockState == LockFileUnobservable {
			return conditionUnverifiable
		}
		return known(o.Index.LockState == LockFileAbsent)
	case ConditionConfigAbsent, ConditionConfigValue:
		if absent {
			return known(c.Kind == ConditionConfigAbsent)
		}
		if !normal || (o.ConfigurationObservationState != ObservationObserved && o.ConfigurationObservationState != ObservationObservedEmpty) {
			return conditionUnverifiable
		}
		var last *ConfigEntry
		for _, entry := range o.EffectiveConfiguration() {
			if entry.Scope == ConfigScopeLocal && entry.Key == c.Subject {
				entry := entry
				last = &entry
			}
		}
		switch {
		case last == nil:
			return known(c.Kind == ConditionConfigAbsent)
		case c.Kind == ConditionConfigAbsent:
			return conditionFails
		case !last.ValuePresent || last.ValueRedaction != RedactionNotNeeded:
			return conditionUnverifiable
		}
		return known(last.ValueRedacted == c.Expected)
	}
	// Object types, index trees and per-path status are not observed by
	// the inspection; the stories that plan them add their evidence.
	return conditionUnverifiable
}

func orUnverifiable(ok bool, result conditionResult) conditionResult {
	if !ok {
		return conditionUnverifiable
	}
	return result
}

// presence evaluates the existence of path against want.
func presence(path string, want bool) conditionResult {
	present, ok := pathPresence(path)
	switch {
	case !ok:
		return conditionUnverifiable
	case present == want:
		return conditionHolds
	}
	return conditionFails
}

func decodedSubject(c Condition) (string, bool) {
	raw, err := DecodeReversiblePath(c.Subject)
	return string(raw), err == nil
}

// projectPath resolves a path subject beneath the physical project root.
// A subject that could escape the root is refused.
func projectPath(inspection GitInspection, c Condition) (string, bool) {
	rel, ok := decodedSubject(c)
	if !ok || rel == "" || strings.HasPrefix(rel, "/") || rel == ".." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") {
		return "", false
	}
	root := string(inspection.ProjectIdentity.ReversiblePathBytes())
	if !strings.HasPrefix(root, "/") {
		return "", false
	}
	return strings.TrimSuffix(root, "/") + "/" + strings.TrimSuffix(rel, "/"), true
}

// octalMode renders a mode in the Git object-mode form the plan uses.
func octalMode(mode fs.FileMode) string {
	switch {
	case mode.IsDir():
		return fmt.Sprintf("040%03o", mode.Perm())
	case mode&fs.ModeSymlink != 0:
		return "120000"
	case mode.IsRegular():
		return fmt.Sprintf("100%03o", mode.Perm())
	}
	return "0"
}
