//go:build !((darwin && !ios) || (linux && !android))

package gatestore

import "ai-dev/internal/gate"

func newPlatformStore(string) gate.Store { return unsupportedStore{} }
