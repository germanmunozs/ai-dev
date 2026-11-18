//go:build !darwin && !linux

package app

import "io"

// isTerminal fails closed where terminal detection is not implemented.
func isTerminal(io.Reader) bool { return false }
