//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// maxIndexBytes bounds the index read in process.
const maxIndexBytes = 64 << 20

func currentEUID() uint32 { return uint32(os.Geteuid()) }

// pathPresence reports whether path exists, without following a final
// symlink. ok is false when neither presence nor absence is proven.
func pathPresence(path string) (present, ok bool) {
	_, err := os.Lstat(path)
	switch {
	case err == nil:
		return true, true
	case errors.Is(err, fs.ErrNotExist):
		return false, true
	}
	return false, false
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// lstatIdentity is the identity of path without following a final symlink.
func lstatIdentity(path string) (FileIdentity, error) {
	var st syscall.Stat_t
	if err := syscall.Lstat(path, &st); err != nil {
		return FileIdentity{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return FileIdentity{}, err
	}
	return FileIdentity{
		DeviceDecimal: strconv.FormatUint(deviceNumber(&st), 10), InodeDecimal: strconv.FormatUint(uint64(st.Ino), 10),
		Mode: info.Mode(), Size: info.Size(),
	}, nil
}

// directoryIdentityDigest binds a directory to its device and inode.
func directoryIdentityDigest(path string) (ContentDigest, bool) {
	identity, err := lstatIdentity(path)
	if err != nil || !identity.Mode.IsDir() {
		return "", false
	}
	return sourceIdentityDigest(identity.DeviceDecimal, identity.InodeDecimal), true
}

// observePathIdentity freezes one worktree path: its identity and, for a
// regular file of at most the authorized-file bound, its content digest
// read through a descriptor whose identity is checked before and after.
func observePathIdentity(path string) (WorktreePathIdentity, bool) {
	identity, err := lstatIdentity(path)
	if err != nil {
		return WorktreePathIdentity{}, false
	}
	observed := WorktreePathIdentity{Path: NewObservedPath([]byte(path)), Identity: identity}
	if !identity.Mode.IsRegular() {
		return observed, observed.Validate() == nil
	}
	if identity.Size > LimitAuthorizedFileBytes.Max() {
		return WorktreePathIdentity{}, false
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return WorktreePathIdentity{}, false
	}
	defer file.Close()
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(file, identity.Size+1))
	after, statErr := file.Stat()
	if err != nil || statErr != nil || n != identity.Size || after.Size() != identity.Size {
		return WorktreePathIdentity{}, false
	}
	observed.ContentDigest = ContentDigest(fmt.Sprintf("sha256:%x", hash.Sum(nil)))
	return observed, observed.Validate() == nil
}

func lockState(path string) LockFileState {
	_, err := os.Lstat(path)
	switch {
	case err == nil:
		return LockFilePresent
	case errors.Is(err, fs.ErrNotExist):
		return LockFileAbsent
	default:
		return LockFileUnobservable
	}
}

// sequencerState reports an interrupted multi-step operation from the Git
// directory's state files.
func sequencerState(gitDir string) SequencerState {
	switch {
	case pathExists(gitDir + "/rebase-apply/applying"):
		return SequencerAm
	case pathExists(gitDir+"/rebase-merge") || pathExists(gitDir+"/rebase-apply"):
		return SequencerRebase
	case pathExists(gitDir + "/MERGE_HEAD"):
		return SequencerMerge
	case pathExists(gitDir + "/CHERRY_PICK_HEAD"):
		return SequencerCherryPick
	case pathExists(gitDir + "/REVERT_HEAD"):
		return SequencerRevert
	case pathExists(gitDir + "/BISECT_LOG"):
		return SequencerBisect
	case pathExists(gitDir + "/sequencer"):
		return SequencerUnknown
	}
	return SequencerNone
}

// readHead returns HEAD's symbolic target, or its detached OID.
func readHead(gitDir string) (symbolic []byte, detached string, ok bool) {
	data, err := readSmallFile(gitDir + "/HEAD")
	if err != nil {
		return nil, "", false
	}
	line := bytes.TrimSuffix(data, []byte("\n"))
	if target, found := bytes.CutPrefix(line, []byte("ref: ")); found {
		return target, "", len(target) > 0
	}
	if validateOID("HEAD", string(line)) == nil {
		return nil, string(line), true
	}
	return nil, "", false
}

// headBranch is HEAD's branch name, for includeIf onbranch; nil when HEAD
// is detached or unreadable.
func headBranch(gitDir string) []byte {
	symbolic, _, ok := readHead(gitDir)
	if !ok {
		return nil
	}
	branch, found := bytes.CutPrefix(symbolic, []byte("refs/heads/"))
	if !found {
		return nil
	}
	return branch
}

// resolveRef resolves a ref from a loose file or packed-refs without Git.
func resolveRef(gitDir, commonDir string, ref []byte) (string, bool) {
	for _, dir := range []string{gitDir, commonDir} {
		if data, err := readSmallFile(dir + "/" + string(ref)); err == nil {
			oid := string(bytes.TrimSuffix(data, []byte("\n")))
			return oid, validateOID("ref", oid) == nil
		}
	}
	file, err := os.Open(commonDir + "/packed-refs")
	if err != nil {
		return "", false
	}
	defer file.Close()
	scanner := bufio.NewScanner(io.LimitReader(file, maxIndexBytes))
	for scanner.Scan() {
		oid, name, found := strings.Cut(scanner.Text(), " ")
		if found && name == string(ref) && validateOID("ref", oid) == nil {
			return oid, true
		}
	}
	return "", false
}

// readIndex observes the normal index in process. Versions 2 and 3 are
// walked to find their extensions; version 4 and anything malformed are
// unreadable rather than guessed.
func readIndex(gitDir string, hashSize int) (IndexObservationInput, string) {
	in := IndexObservationInput{Presence: IndexAbsent, LockState: lockState(gitDir + "/index.lock")}
	file, err := os.OpenFile(gitDir+"/index", os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return in, ""
	}
	unreadable := func(why string) (IndexObservationInput, string) {
		return IndexObservationInput{Presence: IndexUnreadable, LockState: in.LockState}, why
	}
	if err != nil {
		return unreadable("index cannot be opened")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxIndexBytes+1))
	if err != nil || len(data) > maxIndexBytes {
		return unreadable("index cannot be read within its bound")
	}
	if len(data) < 12+hashSize || string(data[:4]) != "DIRC" {
		return unreadable("index header is malformed")
	}
	version, count := binary.BigEndian.Uint32(data[4:8]), binary.BigEndian.Uint32(data[8:12])
	if version != 2 && version != 3 {
		return unreadable(fmt.Sprintf("index version %d is not parsed", version))
	}
	end := len(data) - hashSize
	offset := 12
	for i := uint32(0); i < count; i++ {
		fixed := 40 + hashSize + 2
		if offset+fixed > end {
			return unreadable("index entries are truncated")
		}
		flags := binary.BigEndian.Uint16(data[offset+40+hashSize:])
		if flags&0x4000 != 0 {
			if version < 3 {
				return unreadable("extended index entry in version 2")
			}
			fixed += 2
		}
		nul := bytes.IndexByte(data[offset+fixed:end], 0)
		if nul < 0 {
			return unreadable("index entry name is unterminated")
		}
		offset += (fixed + nul + 8) &^ 7
		if offset > end {
			return unreadable("index entries are truncated")
		}
	}
	var extensions []string
	for offset < end {
		if offset+8 > end {
			return unreadable("index extension is truncated")
		}
		size := int(binary.BigEndian.Uint32(data[offset+4 : offset+8]))
		extensions = append(extensions, string(data[offset:offset+4]))
		offset += 8 + size
		if offset > end {
			return unreadable("index extension is truncated")
		}
	}
	in.Presence, in.Version, in.EntryCountDecimal = IndexPresent, version, strconv.FormatUint(uint64(count), 10)
	in.ContentDigest, in.Extensions = contentDigest(data), extensions
	return in, ""
}

// linkedWorktrees observes the worktrees registered in a common directory
// without running Git and without granting any mutation.
func linkedWorktrees(commonDir string) ([]LinkedWorktree, bool) {
	entries, err := os.ReadDir(commonDir + "/worktrees")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, true
	}
	if err != nil {
		return nil, false
	}
	common, commonOK := directoryIdentityDigest(commonDir)
	var worktrees []LinkedWorktree
	for _, entry := range entries {
		admin := commonDir + "/worktrees/" + entry.Name()
		pointer, err := readSmallFile(admin + "/gitdir")
		if err != nil || !entry.IsDir() {
			return nil, false
		}
		worktreePath := strings.TrimSuffix(strings.TrimSuffix(string(pointer), "\n"), "/.git")
		if CheckPathBytes([]byte(worktreePath)) != nil || !strings.HasPrefix(worktreePath, "/") {
			return nil, false
		}
		worktree := LinkedWorktree{
			ObservationState: ObservationObserved, WorktreePath: NewObservedPath([]byte(worktreePath)),
			GitDirPath: NewObservedPath([]byte(admin)), Availability: WorktreeAvailable, Head: worktreeHead(admin, commonDir),
		}
		if commonOK {
			worktree.CommonDirIdentity = common
		}
		if reason, err := readSmallFile(admin + "/locked"); err == nil {
			worktree.Locked, worktree.Availability = true, WorktreeLocked
			if len(bytes.TrimSpace(reason)) > 0 {
				worktree.LockReasonRedacted = OpaqueMarker
			}
		}
		if !worktree.Locked {
			switch _, err := os.Lstat(worktreePath); {
			case errors.Is(err, fs.ErrNotExist):
				worktree.Availability = WorktreeMissing
			case err != nil:
				worktree.Availability = WorktreeInaccessible
			case !commonOK:
				worktree.Availability = WorktreeAmbiguous
			}
		}
		worktrees = append(worktrees, worktree)
	}
	sort.Slice(worktrees, func(i, j int) bool {
		return bytes.Compare(worktrees[i].WorktreePath.Bytes(), worktrees[j].WorktreePath.Bytes()) < 0
	})
	return worktrees, true
}

func worktreeHead(gitDir, commonDir string) HeadObservation {
	symbolic, detached, ok := readHead(gitDir)
	switch {
	case !ok:
		return HeadObservation{State: HeadStateInvalid}
	case detached != "":
		return HeadObservation{State: HeadStateDetached, CommitOID: ObjectOID(detached), HistoryPresent: true}
	}
	ref := NewObservedRef(symbolic)
	if ref.Validate() != nil {
		return HeadObservation{State: HeadStateInvalid}
	}
	if oid, resolved := resolveRef(gitDir, commonDir, symbolic); resolved {
		return HeadObservation{State: HeadStateAttached, SymbolicRef: ref, CommitOID: ObjectOID(oid), HistoryPresent: true}
	}
	return HeadObservation{State: HeadStateUnborn, SymbolicRef: ref}
}
