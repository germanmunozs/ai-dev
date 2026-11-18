package gitprocess

import (
	"context"
	"fmt"

	"ai-dev/internal/gitmanager"
)

// readRows maps the manager's closed read rows onto matrix rows. Only
// read-only rows appear, so a Reader can never run a mutation.
var readRows = map[gitmanager.GitReadRow]Operation{
	gitmanager.GitReadDiscover:        OpDiscoverRepository,
	gitmanager.GitReadHeadSymbolicRef: OpHeadSymbolicRef,
	gitmanager.GitReadHeadCommit:      OpHeadCommit,
	gitmanager.GitReadShowRefs:        OpShowRefs,
	gitmanager.GitReadStatus:          OpStatus,
}

// Reader implements gitmanager.GitReader over the operation matrix and a
// runner.
type Reader struct {
	Runner gitmanager.GitRunner
}

var _ gitmanager.GitReader = Reader{}

func (r Reader) Read(ctx context.Context, request gitmanager.GitReadRequest) (gitmanager.GitRunResult, error) {
	op, ok := readRows[request.Row]
	if !ok {
		return gitmanager.GitRunResult{}, refuse(Operation(request.Row), gitmanager.ReasonPlanInvalid, "not a read-only row")
	}
	replay := make([]ConfigReplay, len(request.Replay))
	for i, entry := range request.Replay {
		replay[i] = ConfigReplay{Key: entry.Key, Value: entry.Value}
	}
	invocation, err := Build(op, Context{
		Tool: request.Tool, Environment: request.Environment, Directory: request.Directory,
		Overrides: Overrides{FilterDrivers: request.FilterDrivers, Replay: replay},
	}, Parameters{})
	if err != nil {
		return gitmanager.GitRunResult{}, err
	}
	if r.Runner == nil {
		return gitmanager.GitRunResult{}, fmt.Errorf("git reader has no runner")
	}
	return r.Runner.Run(ctx, invocation)
}
