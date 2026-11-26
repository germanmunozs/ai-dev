//go:build !darwin && !linux

package fswatch

import (
	"errors"
	"testing"
)

func TestNewIsUnavailable(t *testing.T) {
	w, err := New()
	var u Unavailable
	if w != nil || !errors.As(err, &u) || u.Reason != "platform_unsupported" {
		t.Fatalf("New() = %v, %v; want platform_unsupported", w, err)
	}
}
