package app

import (
	"flag"
	"io"
)

// flagSetObserver, when set by a test, receives every flag set a command
// builds, so the command catalog can be proven equal to the options each
// command really defines (F010-FR-009). It is nil in production.
var flagSetObserver func(*flag.FlagSet)

// newFlagSet builds a command's flag set: parse errors are returned, never
// printed, because each command writes its own usage line.
func newFlagSet(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	if flagSetObserver != nil {
		flagSetObserver(flags)
	}
	return flags
}
