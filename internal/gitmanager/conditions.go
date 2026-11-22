package gitmanager

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
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
		// Read directly: once HEAD has a commit, status reports an absent
		// or empty index as staged deletions, which the observation does
		// not reconcile.
		if !normal {
			return conditionUnverifiable
		}
		data, present, ok := readVerificationFile(string(o.GitDir.Bytes()) + "/index")
		if !ok {
			return conditionUnverifiable
		}
		return known(normalIndexUnchanged(data, present, c.Kind == ConditionIndexAbsent))
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
	case ConditionObjectType:
		if absent {
			return conditionFails
		}
		if !normal {
			return conditionUnverifiable
		}
		return looseObjectType(string(o.CommonDir.Bytes()), ObjectOID(c.Subject), o.ObjectFormat, c.Expected)
	case ConditionIndexTree:
		if absent {
			return conditionFails
		}
		if !normal {
			return conditionUnverifiable
		}
		data, present, ok := readVerificationFile(string(o.GitDir.Bytes()) + "/index")
		if !ok {
			return conditionUnverifiable
		}
		return known(present && indexHoldsTree(data, o.ObjectFormat, ObjectOID(c.Expected)))
	case ConditionPathStatusClean:
		path, ok := decodedSubject(c)
		if !ok || !normal || (o.WorktreeObservationState != ObservationObserved && o.WorktreeObservationState != ObservationObservedEmpty) ||
			o.Index.Presence == IndexUnreadable {
			return conditionUnverifiable
		}
		for _, change := range o.Index.Changes() {
			if string(change.Path.Bytes()) == path {
				return conditionFails
			}
		}
		for _, change := range o.WorktreeChanges() {
			if strings.TrimSuffix(string(change.Path.Bytes()), "/") == path {
				return conditionFails
			}
		}
		return known(o.Index.Presence == IndexPresent)
	}
	return conditionUnverifiable
}

// normalIndexUnchanged is step 5: the normal index still matches the
// authorized empty state, absent or an index without entries.
func normalIndexUnchanged(data []byte, present, absent bool) bool {
	if absent {
		return !present
	}
	return present && len(data) >= 12 && string(data[:4]) == "DIRC" && binary.BigEndian.Uint32(data[8:12]) == 0
}

// maxObjectBytes bounds a verified loose object: the largest authorized
// file plus its header.
const maxObjectBytes = 64<<20 + 64

// looseObjectType verifies one object in process from its loose file: the
// inflated bytes must hash to the OID and carry the expected type. An
// object that is not loose is unverifiable when packs exist, since only a
// loose object can have been written by this attempt's plumbing.
func looseObjectType(commonDir string, oid ObjectOID, format ObjectFormat, want string) conditionResult {
	if !strings.HasPrefix(commonDir, "/") || oid.ValidateFor(format) != nil {
		return conditionUnverifiable
	}
	data, present, ok := readVerificationFile(commonDir + "/objects/" + string(oid[:2]) + "/" + string(oid[2:]))
	switch {
	case !ok:
		return conditionUnverifiable
	case !present:
		if packs, err := os.ReadDir(commonDir + "/objects/pack"); err == nil && len(packs) == 0 || errors.Is(err, fs.ErrNotExist) {
			return conditionFails
		}
		return conditionUnverifiable
	}
	reader, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return conditionUnverifiable
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxObjectBytes+1))
	if err != nil || len(raw) > maxObjectBytes {
		return conditionUnverifiable
	}
	header, content, found := bytes.Cut(raw, []byte{0})
	kind, size, _ := strings.Cut(string(header), " ")
	if !found || size != strconv.Itoa(len(content)) || objectOID(format, kind, content) != oid {
		return conditionUnverifiable
	}
	if kind == want {
		return conditionHolds
	}
	return conditionFails
}

// readVerificationFile reads one regular file inside the Git directory, without
// following a final symlink. present is false only for a proven absence.
func readVerificationFile(path string) (data []byte, present, ok bool) {
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, false, true
	case err != nil || !info.Mode().IsRegular() || info.Size() > maxIndexReadBytes:
		return nil, false, false
	}
	data, err = os.ReadFile(path)
	if err != nil || int64(len(data)) != info.Size() {
		return nil, false, false
	}
	return data, true, true
}

// maxIndexReadBytes bounds a file read for verification.
const maxIndexReadBytes = 128 << 20

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
