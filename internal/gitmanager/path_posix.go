//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// ResolveProjectRoot resolves the requested project directory to its
// physical path and proves it through an open descriptor (F008-FR-020,
// 021, 150, 151). A relative request is resolved against workingDir. Symlinks
// and "." or ".." are resolved physically, never lexically; the result is
// opened without following a final symlink, and the descriptor's device and
// inode must equal those of the requested path both before and after, or
// the identity is unavailable.
func ResolveProjectRoot(requested, workingDir string) (ProjectRoot, error) {
	unavailable := func(format string, args ...any) (ProjectRoot, error) {
		return ProjectRoot{}, inspectFailure(ReasonProjectIdentityUnavailable, format, args...)
	}
	if requested == "" || strings.ContainsRune(requested, 0) {
		return unavailable("project path is empty or contains NUL")
	}
	path := requested
	if !filepath.IsAbs(path) {
		if !filepath.IsAbs(workingDir) || strings.ContainsRune(workingDir, 0) {
			return unavailable("working directory is not absolute")
		}
		path = workingDir + "/" + path
	}
	if err := CheckPathBytes([]byte(path)); err != nil {
		return unavailable("project path exceeds the path bounds")
	}
	physical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return unavailable("project path cannot be resolved: %v", err)
	}
	if err := CheckPathBytes([]byte(physical)); err != nil {
		return unavailable("physical project path exceeds the path bounds")
	}
	dir, err := os.OpenFile(physical, os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return unavailable("project directory cannot be opened: %v", err)
	}
	defer dir.Close()
	info, err := dir.Stat()
	if err != nil {
		return unavailable("project directory cannot be identified: %v", err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() {
		return unavailable("project path is not a directory")
	}
	device, inode := strconv.FormatUint(deviceNumber(st), 10), strconv.FormatUint(uint64(st.Ino), 10)
	requestedInfo, err := os.Stat(path)
	again, againErr := filepath.EvalSymlinks(path)
	if err != nil || againErr != nil || again != physical || !os.SameFile(info, requestedInfo) {
		return unavailable("project path changed while it was resolved")
	}
	display := ""
	if physicalUTF8(physical) {
		display = physical
	}
	identity, err := NewPhysicalProjectIdentity(display, []byte(physical), device, inode)
	if err != nil {
		return unavailable("project identity: %v", err)
	}
	return ProjectRoot{
		Identity: identity, Aliased: filepath.Clean(path) != physical,
		OwnerUID: st.Uid, OwnedByCurrentUser: int(st.Uid) == os.Geteuid(),
	}, nil
}

func physicalUTF8(path string) bool { return NewObservedPath([]byte(path)).DisplayPathUTF8 != "" }

// fileOwner returns the owner of path without following a final symlink.
func fileOwner(path string) (uint32, error) {
	var st syscall.Stat_t
	if err := syscall.Lstat(path, &st); err != nil {
		return 0, err
	}
	return st.Uid, nil
}

// gitDirState is how complete a candidate Git directory is.
type gitDirState int

const (
	gitDirAbsent gitDirState = iota
	gitDirPartial
	gitDirComplete
	gitDirInaccessible
)

// examineGitDir reports whether dir holds the HEAD file and the objects and
// refs directories that identify a Git directory.
func examineGitDir(dir string) gitDirState {
	found := 0
	for _, entry := range []struct {
		name string
		dir  bool
	}{{"HEAD", false}, {"objects", true}, {"refs", true}} {
		info, err := os.Lstat(dir + "/" + entry.name)
		switch {
		case errors.Is(err, fs.ErrPermission):
			return gitDirInaccessible
		case err != nil:
			continue
		case entry.dir && info.IsDir(), !entry.dir && info.Mode().IsRegular():
			found++
		}
	}
	switch found {
	case 0:
		return gitDirAbsent
	case 3:
		return gitDirComplete
	default:
		return gitDirPartial
	}
}

// locateRepository classifies the repository governing root from the
// filesystem alone, so no Git process runs under a configuration that has
// not been vetted (F008-FR-022, 023). Discovery stops at a filesystem
// boundary, as Git's does by default.
func locateRepository(root []byte) repositoryLocation {
	r := string(root)
	dotgit := r + "/.git"
	info, err := os.Lstat(dotgit)
	switch {
	case err == nil:
		location := ownRepository(r, dotgit, info)
		if location.Kind == RepositoryKindNormal {
			if _, found := ancestorRepository(r); found {
				location.Kind = RepositoryKindNested
			}
		}
		return location
	case errors.Is(err, fs.ErrNotExist):
	default:
		return repositoryLocation{Kind: RepositoryKindInaccessible, Detail: "project .git cannot be inspected"}
	}
	switch examineGitDir(r) {
	case gitDirComplete:
		return repositoryLocation{Kind: RepositoryKindBare, GitDir: root, CommonDir: root}
	case gitDirInaccessible:
		return repositoryLocation{Kind: RepositoryKindInaccessible, Detail: "project directory cannot be inspected"}
	}
	if gitDir, found := ancestorRepository(r); found {
		return repositoryLocation{Kind: RepositoryKindAncestor, GitDir: []byte(gitDir), CommonDir: []byte(gitDir)}
	}
	return repositoryLocation{Kind: RepositoryKindAbsent}
}

// ownRepository classifies the project's own .git entry.
func ownRepository(root, dotgit string, info os.FileInfo) repositoryLocation {
	ambiguous := func(detail string) repositoryLocation {
		return repositoryLocation{Kind: RepositoryKindAmbiguous, Detail: detail}
	}
	switch {
	case info.IsDir():
		switch examineGitDir(dotgit) {
		case gitDirInaccessible:
			return repositoryLocation{Kind: RepositoryKindInaccessible, Detail: ".git directory cannot be inspected"}
		case gitDirComplete:
		default:
			return ambiguous("incomplete .git directory")
		}
		if _, err := os.Lstat(dotgit + "/commondir"); err == nil {
			return ambiguous(".git directory has a commondir")
		}
		return repositoryLocation{Kind: RepositoryKindNormal, GitDir: []byte(dotgit), CommonDir: []byte(dotgit)}
	case info.Mode().IsRegular():
		target, err := readGitFile(dotgit)
		if err != nil {
			return ambiguous("malformed .git file")
		}
		if !filepath.IsAbs(target) {
			target = root + "/" + target
		}
		gitDir, err := filepath.EvalSymlinks(target)
		if err != nil {
			return ambiguous(".git file target cannot be resolved")
		}
		if common, err := readSmallFile(gitDir + "/commondir"); err == nil {
			commonPath := strings.TrimSuffix(string(common), "\n")
			if !filepath.IsAbs(commonPath) {
				commonPath = gitDir + "/" + commonPath
			}
			commonDir, err := filepath.EvalSymlinks(commonPath)
			if err != nil || examineGitDir(commonDir) != gitDirComplete {
				return ambiguous("linked worktree common directory is not a Git directory")
			}
			return repositoryLocation{Kind: RepositoryKindLinkedWorktree, GitDir: []byte(gitDir), CommonDir: []byte(commonDir)}
		}
		if examineGitDir(gitDir) == gitDirComplete && strings.Contains(gitDir, "/.git/modules/") {
			return repositoryLocation{Kind: RepositoryKindSubmodule, GitDir: []byte(gitDir), CommonDir: []byte(gitDir)}
		}
		return ambiguous(".git file names an unrecognized layout")
	default:
		return ambiguous(".git is neither a directory nor a regular file")
	}
}

// ancestorRepository finds the nearest ancestor that is, or contains, a Git
// directory on the same filesystem.
func ancestorRepository(root string) (string, bool) {
	var rootStat syscall.Stat_t
	if syscall.Stat(root, &rootStat) != nil {
		return "", false
	}
	for dir := filepath.Dir(root); ; dir = filepath.Dir(dir) {
		var st syscall.Stat_t
		if syscall.Stat(dir, &st) != nil || deviceNumber(&st) != deviceNumber(&rootStat) {
			return "", false
		}
		if _, err := os.Lstat(dir + "/.git"); err == nil {
			return dir + "/.git", true
		}
		if examineGitDir(dir) == gitDirComplete {
			return dir, true
		}
		if dir == "/" {
			return "", false
		}
	}
}

// readGitFile parses a gitfile: exactly "gitdir: <path>" and a newline.
func readGitFile(path string) (string, error) {
	data, err := readSmallFile(path)
	if err != nil {
		return "", err
	}
	line, ok := bytes.CutPrefix(data, []byte("gitdir: "))
	line = bytes.TrimSuffix(line, []byte("\n"))
	if !ok || len(line) == 0 || bytes.ContainsAny(line, "\x00\n\r") {
		return "", errors.New("not a gitfile")
	}
	return string(line), nil
}

// readSmallFile reads a regular file of at most 4 KiB without following a
// final symlink.
func readSmallFile(path string) ([]byte, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || len(data) > 4096 {
		return nil, errors.New("file too large")
	}
	return data, nil
}
