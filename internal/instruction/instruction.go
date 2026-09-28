// Package instruction validates and resolves the supported instruction sources.
package instruction

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

var (
	ErrInstructionRequired  = errors.New("instruction required")
	ErrAmbiguousInstruction = errors.New("ambiguous instruction source")
)

// Resolve returns the instruction from exactly one meaningful source. It uses
// trimming only for validation and otherwise preserves the selected input.
func Resolve(arg string, hasArg bool, stdin io.Reader, stdinIsInteractive bool) (string, error) {
	argPresent := hasArg && strings.TrimSpace(arg) != ""

	if stdinIsInteractive {
		if argPresent {
			return arg, nil
		}
		return "", ErrInstructionRequired
	}

	stdinBytes, err := io.ReadAll(stdin)
	if err != nil {
		return "", fmt.Errorf("read instruction from stdin: %w", err)
	}
	stdinInstruction := string(stdinBytes)
	stdinPresent := strings.TrimSpace(stdinInstruction) != ""

	if argPresent && stdinPresent {
		return "", ErrAmbiguousInstruction
	}
	if argPresent {
		return arg, nil
	}
	if stdinPresent {
		return stdinInstruction, nil
	}
	return "", ErrInstructionRequired
}
