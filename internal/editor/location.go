// Package editor opens project files in Neovim for ai-dev open (Feature
// 014): it validates the location, checks the parent instance's endpoint,
// and talks to it through a minimal msgpack-RPC client. It uses the
// standard library only (research R-02).
package editor

import (
	"os"
	"path/filepath"
	"strings"
)

// Refusal reasons for a path, in check order (data-model.md).
const (
	ReasonAbsolutePath   = "absolute_path"
	ReasonNotFound       = "not_found"
	ReasonOutsideProject = "outside_project"
	ReasonNotRegularFile = "not_regular_file"
)

// Location is a project file and a position in it. Rel is the path as
// given, for display and reports only; Abs is the resolved path handed to
// the editor.
type Location struct {
	Rel, Abs     string
	Line, Column int
}

// PathError refuses a path with one of the reasons above.
type PathError struct{ Reason string }

func (e *PathError) Error() string { return "path refused: " + e.Reason }

// Resolve validates path against the project root (F014-FR-002): it must
// be relative, and resolve, after every symbolic link, to an existing
// regular file under the resolved root. Every absolute path is refused,
// even one inside the project (owner decision, 2026-11-24). A line or
// column of 0 means 1.
func Resolve(root, path string, line, column int) (Location, error) {
	if filepath.IsAbs(path) {
		return Location{}, &PathError{ReasonAbsolutePath}
	}
	if path == "" {
		return Location{}, &PathError{ReasonNotFound}
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return Location{}, &PathError{ReasonNotFound}
	}
	target, err := filepath.EvalSymlinks(filepath.Join(resolvedRoot, path))
	if err != nil {
		return Location{}, &PathError{ReasonNotFound}
	}
	rel, err := filepath.Rel(resolvedRoot, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return Location{}, &PathError{ReasonOutsideProject}
	}
	info, err := os.Stat(target)
	if err != nil {
		return Location{}, &PathError{ReasonNotFound}
	}
	if !info.Mode().IsRegular() {
		return Location{}, &PathError{ReasonNotRegularFile}
	}
	return Location{Rel: path, Abs: target, Line: max(line, 1), Column: max(column, 1)}, nil
}
