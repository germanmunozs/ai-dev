//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// planOutputWrite writes the plan bytes; tests replace it to inject a
// failure after creation.
var planOutputWrite = func(file *os.File, data []byte) error {
	_, err := file.Write(data)
	return err
}

// WritePlanOutput exports a plan as an explicit external write (T057,
// inspection contract §5). The destination parent is resolved physically
// and must lie outside every prohibited area on a supported production
// filesystem; an existing destination is output_exists and anything unsafe
// or unverifiable is unsafe_output_path, both before anything is created.
// The file is created exclusively with mode 0600, synced, and its parent
// directory synced. A failure after creation is output_failed, and the
// partial file this call created is removed only if it is still the same
// file. It returns the physical path written.
func WritePlanOutput(req PlanOutputRequest, data []byte) (string, *PlanRejection) {
	unsafe := func(format string, args ...any) (string, *PlanRejection) {
		return "", reject(ReasonUnsafeOutputPath, format, args...)
	}
	path := req.Path
	if path == "" || strings.ContainsRune(path, 0) {
		return unsafe("the output path is empty or contains NUL")
	}
	if !filepath.IsAbs(path) {
		if !filepath.IsAbs(req.WorkingDir) {
			return unsafe("the working directory is not absolute")
		}
		path = req.WorkingDir + "/" + path
	}
	name := filepath.Base(path)
	if strings.HasSuffix(path, "/") || name == "." || name == ".." || name == "/" {
		return unsafe("the output path does not name a file")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return unsafe("the output directory cannot be resolved")
	}
	final := parent + "/" + name
	if parent == "/" {
		final = "/" + name
	}
	if CheckPathBytes([]byte(final)) != nil {
		return unsafe("the output path exceeds the path bounds")
	}
	if _, err := os.Lstat(final); err == nil {
		return "", reject(ReasonOutputExists, "the output file already exists")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return unsafe("the output path cannot be inspected")
	}
	for _, area := range prohibitedAreas(req.Inspection) {
		if insideArea(parent, area) {
			return unsafe("the output path is inside the project or a Git area")
		}
	}
	if req.ObserveFilesystem == nil {
		return unsafe("the output filesystem cannot be observed")
	}
	observation, err := req.ObserveFilesystem(parent)
	if err != nil {
		return unsafe("the output filesystem cannot be observed")
	}
	profile, err := ClassifyFilesystem(FilesystemRolePlanOutputParent, observation, CertifiedPrimitives(observation))
	if err != nil || RequireProductionFilesystem(profile) != nil {
		return unsafe("the output directory is not on a supported production filesystem")
	}
	directory, err := os.OpenFile(parent, os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return unsafe("the output directory cannot be opened")
	}
	defer directory.Close()
	file, err := os.OpenFile(final, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return "", reject(ReasonOutputExists, "the output file already exists")
	}
	if err != nil {
		return unsafe("the output file cannot be created exclusively")
	}
	created, statErr := file.Stat()
	fail := func() (string, *PlanRejection) {
		file.Close()
		if current, err := os.Lstat(final); err == nil && statErr == nil && os.SameFile(created, current) {
			_ = os.Remove(final)
		}
		return "", reject(ReasonOutputFailed, "the plan could not be written durably")
	}
	if statErr != nil || file.Chmod(0o600) != nil || planOutputWrite(file, data) != nil || file.Sync() != nil {
		return fail()
	}
	if err := file.Close(); err != nil {
		return fail()
	}
	if err := directory.Sync(); err != nil {
		return "", reject(ReasonOutputFailed, "the output directory could not be synced")
	}
	return final, nil
}
