//go:build (darwin && !ios) || (linux && !android)

package gitstore

import (
	"errors"
	"os"
	"path"
	"strings"

	"ai-dev/internal/gitmanager"
)

// Store is the private, durable Git state beneath one canonical state
// root. Its authorization, audit and lock operations arrive with later
// tasks; this file establishes the layout they share.
type Store struct {
	root    *os.Root
	records records
}

// Open creates or validates the private state layout: the root and its
// attempts, environment, locks and projects directories are mode 0700 and owned by
// this user, and both environment leaves are absent.
func Open(stateRoot gitmanager.ObservedPath) (*Store, error) {
	refuse := func(detail string) (*Store, error) {
		return nil, &PlatformError{Reason: gitmanager.ReasonAuditUnavailable, Detail: detail}
	}
	if err := stateRoot.Validate(); err != nil {
		return refuse("state root: " + err.Error())
	}
	rootPath := string(stateRoot.Bytes())
	if !strings.HasPrefix(rootPath, "/") {
		return refuse("state root is not absolute")
	}
	if err := os.MkdirAll(rootPath, 0o700); err != nil {
		return refuse("create state root: " + err.Error())
	}
	if err := requirePrivateDirectory(rootPath); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return refuse("open state root: " + err.Error())
	}
	for _, dir := range []string{"attempts", "environment", "locks", "projects"} {
		if err := ensurePrivateDirectory(root, dir); err != nil {
			root.Close()
			return nil, asAuditUnavailable(err)
		}
	}
	for _, leaf := range []string{gitmanager.EnvironmentHomeLeaf, gitmanager.EnvironmentXDGLeaf} {
		if _, err := root.Lstat(leaf); !errors.Is(err, os.ErrNotExist) {
			root.Close()
			return refuse("environment leaf " + leaf + " exists or cannot be proven absent")
		}
	}
	return &Store{root: root, records: records{root: root, ops: defaultRecordOps()}}, nil
}

// Close releases the state root.
func (s *Store) Close() error { return s.root.Close() }

// projectPath names the partition of one physical project identity.
func projectPath(project gitmanager.ProjectIdentityDigest) (string, error) {
	if err := project.Validate(); err != nil {
		return "", err
	}
	return path.Join("projects", strings.TrimPrefix(string(project), "sha256:")), nil
}

// projectDirectory returns, creating it when needed, the private partition
// of one physical project identity.
func (s *Store) projectDirectory(project gitmanager.ProjectIdentityDigest) (string, error) {
	dir, err := projectPath(project)
	if err != nil {
		return "", err
	}
	for _, name := range []string{dir, path.Join(dir, "authorizations"), path.Join(dir, "attempts")} {
		if err := ensurePrivateDirectory(s.root, name); err != nil {
			return "", asAuditUnavailable(err)
		}
	}
	return dir, nil
}

func asAuditUnavailable(err error) error {
	var platformErr *PlatformError
	if errors.As(err, &platformErr) {
		return err
	}
	return &PlatformError{Reason: gitmanager.ReasonAuditUnavailable, Detail: err.Error()}
}
