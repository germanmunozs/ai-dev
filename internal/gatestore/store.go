// Package gatestore is ai-dev's sole Workflow State write path. It persists a
// state transition and its Human Gate evidence as one atomic record replace.
package gatestore

import "github.com/germanmunozs/ai-dev/internal/gate"

const tempPrefix = ".workflow-state."

// New returns the platform implementation without performing I/O.
func New(dir string) gate.Store { return newPlatformStore(dir) }
