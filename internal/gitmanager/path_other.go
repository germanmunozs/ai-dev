//go:build !((darwin && !ios) || (linux && !android))

package gitmanager

// ResolveProjectRoot fails closed on platforms without the descriptor and
// identity guarantees inspection relies on.
func ResolveProjectRoot(string, string) (ProjectRoot, error) {
	return ProjectRoot{}, inspectFailure(ReasonPlatformUnsupported, "project inspection requires macOS or Linux")
}

func locateRepository([]byte) repositoryLocation {
	return repositoryLocation{Kind: RepositoryKindInaccessible, Detail: "platform unsupported"}
}

func readSource(scope string, path []byte, precedence uint64, _ *Budget) sourceRead {
	return failedSource(scope, path, precedence, SourceReadUnsupported)
}

func readTreeSource(scope string, path []byte, precedence uint64, _ *Budget) sourceRead {
	return failedSource(scope, path, precedence, SourceReadUnsupported)
}

func currentEUID() uint32                             { return 0 }
func pathExists(string) bool                          { return false }
func fileOwner(string) (uint32, error)                { return 0, errUnsupportedPlatform }
func headBranch(string) []byte                        { return nil }
func sequencerState(string) SequencerState            { return SequencerUnknown }
func lockState(string) LockFileState                  { return LockFileUnobservable }
func linkedWorktrees(string) ([]LinkedWorktree, bool) { return nil, false }
func observePathIdentity(string) (WorktreePathIdentity, bool) {
	return WorktreePathIdentity{}, false
}
func readIndex(string, int) (IndexObservationInput, string) {
	return IndexObservationInput{Presence: IndexUnreadable, LockState: LockFileUnobservable}, "platform unsupported"
}

var errUnsupportedPlatform = inspectFailure(ReasonPlatformUnsupported, "platform unsupported")
