//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"context"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"syscall"
)

// createConfigDirectory creates .ai-dev beneath the physical root
// (F008-FR-072–077). Mkdir never follows or replaces an entry; the mode is
// set through the new directory's descriptor so the umask cannot change
// it, and the root is synced so the entry is durable.
func createConfigDirectory(root string) error {
	path := root + "/.ai-dev"
	if err := os.Mkdir(path, 0o700); err != nil {
		return err
	}
	dir, err := os.OpenFile(path, os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := dir.Chmod(0o755); err != nil {
		return err
	}
	if err := dir.Sync(); err != nil {
		return err
	}
	return syncDirectory(root)
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// configTemporary is the exclusively created temporary of one attempt's
// configuration write; it is owned by its device and inode.
type configTemporary struct {
	path      string
	identity  syscall.Stat_t
	published bool
}

// release removes the temporary only while it is still the inode this
// attempt created; a published temporary is already gone.
func (c *configTemporary) release(context.Context) error {
	if c.published {
		return nil
	}
	var named syscall.Stat_t
	if err := syscall.Lstat(c.path, &named); err != nil || !sameInode(named, c.identity) {
		return ErrOwnedResourceReplaced
	}
	return os.Remove(c.path)
}

// writeProjectConfig is the atomic configuration write (F008-FR-076, 078):
// an exclusive temporary in the same directory, mode set by descriptor,
// the exact bytes, fsync, and a digest check of what was written; then a
// no-replace publication by hard link to the absent target, removal of the
// owned temporary and a directory sync. An existing target is never
// overwritten: the link fails instead. The temporary is registered before
// anything is written, so a failure leaves it to the audited release.
func writeProjectConfig(root string, data []byte, random RandomSource, own func(*configTemporary) error) error {
	dirPath := root + "/.ai-dev"
	before, err := lstatIdentity(dirPath)
	if err != nil || !before.Mode.IsDir() {
		return errLiteralFile
	}
	name := make([]byte, 16)
	if random == nil {
		return errors.New("no random source")
	}
	if n, err := random.Read(name); err != nil || n != len(name) {
		return errors.New("random source failed")
	}
	path := dirPath + "/.config.json." + hex.EncodeToString(name) + ".tmp"
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	identity, err := descriptorStat(file)
	if err != nil {
		return err
	}
	temporary := &configTemporary{path: path, identity: identity}
	if err := own(temporary); err != nil {
		os.Remove(path)
		return err
	}
	if err := file.Chmod(0o644); err != nil {
		return err
	}
	if n, err := file.Write(data); err != nil || n != len(data) {
		return errors.New("short configuration write")
	}
	if err := file.Sync(); err != nil {
		return err
	}
	written, err := os.ReadFile(path)
	if err != nil || contentDigest(written) != contentDigest(data) {
		return errors.New("the written configuration does not have the expected digest")
	}
	// The directory must still be the one that was checked: the
	// temporary was created inside it, not through a replacement.
	after, err := lstatIdentity(dirPath)
	if err != nil || after.DeviceDecimal != before.DeviceDecimal || after.InodeDecimal != before.InodeDecimal || !after.Mode.IsDir() {
		return ErrOwnedResourceReplaced
	}
	if err := os.Link(path, dirPath+"/config.json"); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return errConfigExists
		}
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	temporary.published = true
	return syncDirectory(dirPath)
}

// errConfigExists reports a configuration that appeared after
// revalidation; it is never overwritten.
var errConfigExists = errors.New(".ai-dev/config.json appeared and was not overwritten")

// createDirectory and writeConfig are the dispatcher's two project
// configuration operations (T093). A failure with the target proven absent
// is failed; any other failure leaves an effect that cannot be classified
// without a fresh inspection, so it is indeterminate.
func (d *OperationDispatcher) createDirectory(ctx context.Context, permit MutationPermit, plan GitInitializationPlan, operation DerivedOperation) OperationOutcome {
	root := string(plan.PreconditionSnapshot.ProjectIdentity.ReversiblePathBytes())
	if permit.authorizesPlanned(plan, operation) != nil {
		return OperationFailed
	}
	if err := createConfigDirectory(root); err != nil {
		return absentOrIndeterminate(root + "/.ai-dev")
	}
	return d.postconditions(ctx, plan, operation)
}

func (d *OperationDispatcher) writeConfig(ctx context.Context, permit MutationPermit, plan GitInitializationPlan, operation DerivedOperation) OperationOutcome {
	intent := plan.ImmutableIntent
	root := string(plan.PreconditionSnapshot.ProjectIdentity.ReversiblePathBytes())
	data, err := renderProjectConfig(intent.ProjectID, intent.Strategy, intent.GitFlowDialect, intent.BranchParameters(), intent.PrefixParameters())
	if err != nil || contentDigest(data) != intent.ProjectConfigBytesDigest || permit.authorizesPlanned(plan, operation) != nil {
		return OperationFailed
	}
	own := func(temporary *configTemporary) error {
		return permit.own(operation, ResourceRoleConfigTemporary, temporary.release)
	}
	if err := writeProjectConfig(root, data, d.deps.Random, own); err != nil {
		if errors.Is(err, errConfigExists) {
			return OperationFailed
		}
		return absentOrIndeterminate(root + "/" + ProjectConfigPath)
	}
	return d.postconditions(ctx, plan, operation)
}

func absentOrIndeterminate(path string) OperationOutcome {
	if present, ok := pathPresence(path); ok && !present {
		return OperationFailed
	}
	return OperationIndeterminate
}
