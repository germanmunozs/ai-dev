package gitmanager

import (
	"crypto/sha256"
	"fmt"
)

// sourceRead is one bounded configuration, attribute or ignore source. Data
// is retained only in memory for evaluation; the identity is what the
// inspection records.
type sourceRead struct {
	identity InspectedSourceIdentity
	data     []byte
}

func (s sourceRead) failed() bool {
	switch s.identity.ReadState {
	case SourceReadUnreadable, SourceReadAmbiguous, SourceReadUnsupported:
		return true
	}
	return false
}

func contentDigest(data []byte) ContentDigest {
	return ContentDigest(fmt.Sprintf("sha256:%x", sha256.Sum256(data)))
}

// sourceIdentityDigest binds a source to its physical device and inode.
func sourceIdentityDigest(device, inode string) ContentDigest {
	return contentDigest([]byte("ai-dev-source-v1\x00" + device + "\x00" + inode))
}

// failedSource records a source that could not be read safely.
func failedSource(scope string, path []byte, precedence uint64, state SourceReadState) sourceRead {
	identity := InspectedSourceIdentity{Scope: scope, ReadState: state, Precedence: precedence}
	if CheckPathBytes(path) == nil && len(path) > 0 {
		identity.Path = NewObservedPath(path)
	}
	return sourceRead{identity: identity}
}
