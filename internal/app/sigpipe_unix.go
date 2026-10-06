//go:build unix

package app

import (
	"os"
	"os/signal"
	"syscall"
)

// brokenPipeAsError makes broken-pipe writes return an error instead of
// terminating the process. Its effect is process-wide but is limited in time
// to one runWorkflowNext call, under the production precondition that
// runWorkflowNext runs at most once per process.
func brokenPipeAsError() (release func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGPIPE)
	return func() { signal.Stop(ch) }
}
