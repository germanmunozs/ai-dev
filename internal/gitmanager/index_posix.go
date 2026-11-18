//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"syscall"
)

// indexLock is the descriptor-owned .git/index.lock of one attempt
// (F008-FR-184). Ownership is the successful exclusive creation, the
// retained descriptor and its device and inode; no token enters the index.
type indexLock struct {
	files     fileOps
	path      string
	file      *os.File
	identity  syscall.Stat_t
	installed bool
}

// acquireIndexLock is steps 1–4: the planned lock must be absent, is
// created exclusively without following a symlink, its descriptor
// identity is recorded and the descriptor is kept open. A lock created
// but not identified is errCreatedUnverified and stays where it is.
func acquireIndexLock(files fileOps, gitDir string) (*indexLock, error) {
	path := gitDir + "/index.lock"
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		return nil, errors.New("index.lock exists")
	}
	file, err := files.openFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0o644)
	if err != nil {
		return nil, err
	}
	identity, err := files.fstat(file)
	if err != nil {
		file.Close()
		return nil, errCreatedUnverified
	}
	return &indexLock{files: files, path: path, file: file, identity: identity}, nil
}

// owned reports whether the lock path still names the inode this attempt
// created and holds open.
func (l *indexLock) owned() bool {
	held, err := l.files.fstat(l.file)
	var named syscall.Stat_t
	return err == nil && syscall.Lstat(l.path, &named) == nil && sameInode(held, l.identity) && sameInode(named, l.identity)
}

func sameInode(a, b syscall.Stat_t) bool {
	return deviceNumber(&a) == deviceNumber(&b) && a.Ino == b.Ino
}

// install is steps 7–11: copy the verified bytes through the retained
// descriptor, sync, reverify ownership, rename the owned lock to index and
// sync the Git directory.
func (l *indexLock) install(gitDir string, data []byte) error {
	if n, err := l.files.write(l.file, data); err != nil || n != len(data) {
		return errors.Join(errors.New("short index write"), err)
	}
	if err := l.files.sync(l.file); err != nil {
		return err
	}
	if !l.owned() {
		return ErrOwnedResourceReplaced
	}
	if err := l.files.rename(l.path, gitDir+"/index"); err != nil {
		// A rename that reports failure but no longer leaves the owned lock
		// in place may have installed the index.
		if !l.owned() {
			return errors.Join(errCreatedUnverified, err)
		}
		return err
	}
	l.installed = true
	l.file.Close()
	return l.files.syncDir(gitDir)
}

// installOutcome classifies a failed lock acquisition or installation: an
// installed index, or a lock that was created but cannot be identified, is
// an effect only a fresh inspection can classify.
func installOutcome(lock *indexLock, err error) OperationOutcome {
	if errors.Is(err, errCreatedUnverified) || lock != nil && lock.installed {
		return OperationIndeterminate
	}
	return OperationFailed
}

// release is step 13, run by the audited release protocol: an installed
// lock is gone by rename; otherwise only the still-matching owned inode is
// unlinked, and a missing, replaced or foreign lock is left untouched.
func (l *indexLock) release(context.Context) error {
	if l.installed {
		return nil
	}
	defer l.file.Close()
	if !l.owned() {
		return ErrOwnedResourceReplaced
	}
	return l.files.remove(l.path)
}

// preparedIndex is the private external prepared index of one attempt:
// a fresh 0700 directory below the state root that only Git's read-tree
// writes into.
type preparedIndex struct {
	files     fileOps
	dir, path string
	identity  *syscall.Stat_t
}

func newPreparedIndex(files fileOps, stateRoot []byte, random RandomSource) (*preparedIndex, error) {
	name := make([]byte, 16)
	if random == nil {
		return nil, errors.New("no random source")
	}
	if n, err := random.Read(name); err != nil || n != len(name) {
		return nil, errors.New("random source failed")
	}
	dir := string(stateRoot) + "/prepared/" + hex.EncodeToString(name)
	if err := files.mkdir(dir, 0o700); err != nil {
		// A mkdir that reports failure may still have created the fresh,
		// randomly named directory; rmdir removes it only while empty.
		files.remove(dir)
		return nil, err
	}
	return &preparedIndex{files: files, dir: dir, path: dir + "/index"}, nil
}

// read reads the index Git wrote, without following a symlink, and
// records its identity for release.
func (p *preparedIndex) read() ([]byte, error) {
	file, err := p.files.openFile(p.path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	identity, err := p.files.fstat(file)
	if err != nil || identity.Mode&syscall.S_IFMT != syscall.S_IFREG || identity.Size > maxIndexBytes {
		return nil, errors.New("prepared index is not a bounded regular file")
	}
	p.identity = &identity
	data, err := io.ReadAll(io.LimitReader(file, identity.Size+1))
	if err != nil || int64(len(data)) != identity.Size {
		return nil, errors.New("prepared index changed while read")
	}
	return data, nil
}

// release removes only the prepared index this attempt read and its own
// directory; anything else in it is left and reported.
func (p *preparedIndex) release(context.Context) error {
	if p.identity != nil {
		var named syscall.Stat_t
		if err := syscall.Lstat(p.path, &named); err != nil || !sameInode(named, *p.identity) {
			return ErrOwnedResourceReplaced
		}
		if err := p.files.remove(p.path); err != nil {
			return err
		}
	}
	if err := p.files.remove(p.dir); err != nil {
		return ErrOwnedResourceReplaced
	}
	return nil
}

// installIndex is the prepared-index operation: the sole 13-step
// index.lock protocol (F008-FR-115–124, 157–158, 184). The lock is taken
// first and registered for the audited release, the normal index is
// revalidated, the autonomous version-2 index is built outside the
// repository and parsed independently, and only then is it installed
// through the retained descriptor. Step 12 is the fresh postcondition
// inspection; step 13 is the release.
func (d *OperationDispatcher) installIndex(ctx context.Context, permit MutationPermit, plan GitInitializationPlan, operation DerivedOperation) OperationOutcome {
	intent, snapshot := plan.ImmutableIntent, plan.PreconditionSnapshot
	objects, err := planCommit(intent)
	if err != nil {
		return OperationFailed
	}
	gitDir := string(snapshot.ProjectIdentity.ReversiblePathBytes()) + "/.git"
	absent := snapshot.Observation.Index.Presence != IndexPresent
	if permit.authorizesPlanned(plan, operation) != nil {
		return OperationFailed
	}
	lock, err := acquireIndexLock(d.files(), gitDir)
	if err != nil {
		return installOutcome(nil, err)
	}
	if err := permit.own(operation, ResourceRoleIndexLock, lock.release); err != nil {
		lock.release(ctx)
		return OperationFailed
	}
	if data, present, ok := readVerificationFile(gitDir + "/index"); !ok || !normalIndexUnchanged(data, present, absent) {
		return OperationFailed
	}
	prepared, err := newPreparedIndex(d.files(), snapshot.EnvironmentProfile.StateRoot.Bytes(), d.deps.Random)
	if err != nil {
		return OperationFailed
	}
	if err := permit.own(operation, ResourceRolePreparedIndex, prepared.release); err != nil {
		prepared.release(ctx)
		return OperationFailed
	}
	invocation, rejection := buildCommand(d.deps.Commands, intent, snapshot,
		GitCommand{Kind: GitCommandPreparedIndex, TreeOID: objects.rootTree(), IndexFile: []byte(prepared.path)})
	if rejection != nil || !equalLists(invocation.Args, operation.ExactArgv()) {
		return OperationFailed
	}
	if outcome, ran := d.run(ctx, permit, plan, operation, invocation, ""); !ran {
		return outcome
	}
	data, err := prepared.read()
	if err != nil || !indexHoldsTree(data, intent.ObjectFormat, objects.rootTree()) {
		return OperationFailed
	}
	if permit.authorizesPlanned(plan, operation) != nil {
		return OperationFailed
	}
	if err := lock.install(gitDir, data); err != nil {
		return installOutcome(lock, err)
	}
	return d.postconditions(ctx, plan, operation)
}
