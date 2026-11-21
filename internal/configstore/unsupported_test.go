package configstore

import (
	"strings"
	"testing"
)

// Outside macOS and Linux, writing always fails closed (F011-FR-007).
func TestUnsupportedStoreAlwaysFails(t *testing.T) {
	written, warnings := unsupportedStore{}.Write([]byte("x"))
	if written || len(warnings) != 1 || !strings.Contains(warnings[0], "macOS and Linux") {
		t.Fatalf("written=%v warnings=%v", written, warnings)
	}
}
