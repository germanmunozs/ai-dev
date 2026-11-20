// Package guide implements Feature 007's read-only guided mode. It composes
// Feature 005's next-step evaluation (sdd.Evaluate) and Feature 003's role
// selection (through an injected Selector) by reference, and builds the
// command templates a human may complete and run. It starts no process,
// writes nothing, reads no standard input, reads no environment or
// configuration, and never evaluates a transition or derives a Human Gate
// decision field.
package guide
