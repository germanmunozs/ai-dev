package gitmanager

import (
	"bytes"
	"fmt"
)

// InspectError reports why inspection could not establish a required fact,
// with the closed reason the caller returns.
type InspectError struct {
	Reason Reason
	Detail string
}

func (e *InspectError) Error() string { return fmt.Sprintf("inspect: %s (%s)", e.Detail, e.Reason) }

func inspectFailure(reason Reason, format string, args ...any) error {
	return &InspectError{Reason: reason, Detail: fmt.Sprintf(format, args...)}
}

// ProjectRoot is a physical project root resolved and proven through an
// open directory descriptor.
type ProjectRoot struct {
	Identity PhysicalProjectIdentity
	// Aliased reports that the requested path named the root through a
	// symlink or a non-canonical spelling.
	Aliased            bool
	OwnerUID           uint32
	OwnedByCurrentUser bool
}

// repositoryLocation is the in-process classification of the repository
// that governs a project root. Paths are physical.
type repositoryLocation struct {
	Kind      RepositoryKind
	GitDir    []byte
	CommonDir []byte
	// Detail is a fixed label explaining an ambiguous or inaccessible kind.
	Detail string
}

// ownershipIncompatibilities reports dubious ownership of the project root
// or its Git directory. It is reported, never repaired, and never resolved
// through safe.directory (F008-FR-019).
func ownershipIncompatibilities(rootUID, gitDirUID, euid uint32, hasGitDir bool) []Incompatibility {
	var found []Incompatibility
	if rootUID != euid {
		found = append(found, Incompatibility{Reason: ReasonDubiousOwnership, Subject: "project root is not owned by the current user"})
	}
	if hasGitDir && gitDirUID != euid {
		found = append(found, Incompatibility{Reason: ReasonDubiousOwnership, Subject: "Git directory is not owned by the current user"})
	}
	return found
}

// pathEquivalenceIncompatibility proves that a set of repository paths stay
// distinct and byte-exact under the effective case and Unicode behaviour
// (F008-FR-088, F008-FR-183). Exact duplicates collide. Where case folding
// or Unicode precomposition may apply, a non-ASCII path cannot be proven
// equivalent without a Unicode normalization table, which the standard
// library lacks, so it is path_normalization_incompatible; ASCII paths and
// directory prefixes that fold together collide.
func pathEquivalenceIncompatibility(paths [][]byte, ignoreCase, precompose bool) Reason {
	exact := make(map[string]bool, len(paths))
	for _, path := range paths {
		if exact[string(path)] {
			return ReasonPathCollision
		}
		exact[string(path)] = true
	}
	if !ignoreCase && !precompose {
		return ""
	}
	for _, path := range paths {
		for _, b := range path {
			if b >= 0x80 {
				return ReasonPathNormalizationIncompatible
			}
		}
	}
	if !ignoreCase {
		return ""
	}
	folded := make(map[string]string)
	for _, path := range paths {
		for end := 0; end <= len(path); end++ {
			if end < len(path) && path[end] != '/' {
				continue
			}
			prefix := path[:end]
			key := string(bytes.ToLower(prefix))
			if original, ok := folded[key]; ok && original != string(prefix) {
				return ReasonPathCollision
			}
			folded[key] = string(prefix)
		}
	}
	return ""
}
