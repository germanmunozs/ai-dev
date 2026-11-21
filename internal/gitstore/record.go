package gitstore

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/germanmunozs/ai-dev/internal/gitmanager"
)

var (
	// ErrRecordExists reports a no-replace publication over an existing
	// record; the existing record is left untouched.
	ErrRecordExists = errors.New("git store record already exists")
	// ErrInconsistentEvents reports an event directory that a single
	// append-only writer could not have produced. It is never partially
	// trusted; supervised reconciliation examines it.
	ErrInconsistentEvents = errors.New("git store event directory is inconsistent")
)

// temporaryPrefix marks same-directory temporaries. A temporary is never a
// record: one left behind by a crash is reconciliation evidence.
const temporaryPrefix = ".tmp-"

// maxRecordBytes bounds every record read back from the store.
const maxRecordBytes = 8 << 20

// validRecordName accepts a single visible path component of safe bytes.
func validRecordName(name string) bool {
	if name == "" || len(name) > 255 || name[0] == '.' {
		return false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// eventName is "<20-digit sequence>-<kind>.json": zero padding makes
// lexical order equal sequence order.
func eventName(sequence uint64, kind gitmanager.AuditEventKind) string {
	return fmt.Sprintf("%020d-%s.json", sequence, kind)
}

func parseEventName(name string) (uint64, gitmanager.AuditEventKind, error) {
	refuse := func() (uint64, gitmanager.AuditEventKind, error) {
		return 0, "", fmt.Errorf("%w: event name %q", ErrInconsistentEvents, name)
	}
	digits, rest, found := strings.Cut(name, "-")
	if !found || len(digits) != 20 || !strings.HasSuffix(rest, ".json") {
		return refuse()
	}
	sequence, err := strconv.ParseUint(digits, 10, 64)
	kind := gitmanager.AuditEventKind(strings.TrimSuffix(rest, ".json"))
	if err != nil || sequence == 0 || !kind.Valid() {
		return refuse()
	}
	return sequence, kind, nil
}

// event is one listed event file.
type event struct {
	sequence uint64
	kind     gitmanager.AuditEventKind
	name     string
}
