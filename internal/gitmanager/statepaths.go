package gitmanager

import (
	"bytes"
	"fmt"
)

// EnvironmentProfileVersion names the private Git subprocess environment
// layout that EnvironmentProfile freezes.
const EnvironmentProfileVersion = "ai-dev.git-environment/v1"

// StateRootError reports why the canonical Git state root cannot be
// resolved. Inspect turns it into a warning; plan and apply refuse with
// Reason before any mutation.
type StateRootError struct {
	Reason Reason
	Detail string
}

func (e *StateRootError) Error() string {
	return fmt.Sprintf("git state root unavailable (%s): %s", e.Reason, e.Detail)
}

// ResolveStateRoot resolves the canonical Git state root once, from the real
// user context supplied by lookupEnv:
//
//   - macOS: $HOME/Library/Application Support/ai-dev/state/git
//   - Linux: $XDG_STATE_HOME/ai-dev/git, or $HOME/.local/state/ai-dev/git
//     when XDG_STATE_HOME is unset or empty
//
// The root ends in exactly one "git" segment. A base must be absolute and
// is never resolved against the working directory; trailing slashes are
// dropped, but an empty, "." or ".." component is refused rather than
// cleaned, because cleaning could name a different directory.
func ResolveStateRoot(goos string, lookupEnv func(string) (string, bool)) (ObservedPath, error) {
	var base, suffix string
	switch goos {
	case "darwin":
		base, suffix = lookupHome(lookupEnv), "Library/Application Support/ai-dev/state/git"
	case "linux":
		if xdg, _ := lookupEnv("XDG_STATE_HOME"); xdg != "" {
			base, suffix = xdg, "ai-dev/git"
		} else {
			base, suffix = lookupHome(lookupEnv), ".local/state/ai-dev/git"
		}
	default:
		return ObservedPath{}, &StateRootError{Reason: ReasonPlatformUnsupported, Detail: "unsupported operating system " + goos}
	}
	if base == "" {
		return ObservedPath{}, &StateRootError{Reason: ReasonAuditUnavailable, Detail: "HOME is unset or empty"}
	}
	clean, err := absoluteBase([]byte(base))
	if err != nil {
		return ObservedPath{}, err
	}
	root := NewObservedPath(joinPath(clean, []byte(suffix)))
	if err := root.Validate(); err != nil {
		return ObservedPath{}, &StateRootError{Reason: ReasonAuditUnavailable, Detail: err.Error()}
	}
	return root, nil
}

func lookupHome(lookupEnv func(string) (string, bool)) string {
	home, _ := lookupEnv("HOME")
	return home
}

// absoluteBase validates an environment-provided base directory and drops
// its trailing slashes.
func absoluteBase(base []byte) ([]byte, error) {
	refuse := func(detail string) ([]byte, error) {
		return nil, &StateRootError{Reason: ReasonAuditUnavailable, Detail: detail}
	}
	if base[0] != '/' {
		return refuse("base directory is not absolute")
	}
	if bytes.IndexByte(base, 0) >= 0 {
		return refuse("base directory contains NUL")
	}
	trimmed := bytes.TrimRight(base, "/")
	if len(trimmed) == 0 {
		return []byte("/"), nil
	}
	for _, component := range bytes.Split(trimmed[1:], []byte("/")) {
		switch string(component) {
		case "", ".", "..":
			return refuse("base directory has an empty, '.' or '..' component")
		}
	}
	return trimmed, nil
}

func joinPath(base, relative []byte) []byte {
	joined := append([]byte(nil), base...)
	if !bytes.HasSuffix(joined, []byte("/")) {
		joined = append(joined, '/')
	}
	return append(joined, relative...)
}

// NewEnvironmentProfile builds the frozen private environment for a
// resolved state root: HOME and XDG_CONFIG_HOME are the two fixed leaves
// directly beneath it, which must never exist.
func NewEnvironmentProfile(stateRoot ObservedPath) (EnvironmentProfile, error) {
	if err := stateRoot.Validate(); err != nil {
		return EnvironmentProfile{}, err
	}
	root := stateRoot.Bytes()
	v := EnvironmentProfile{
		Version:       EnvironmentProfileVersion,
		StateRoot:     stateRoot,
		Home:          NewObservedPath(joinPath(root, []byte(EnvironmentHomeLeaf))),
		XDGConfigHome: NewObservedPath(joinPath(root, []byte(EnvironmentXDGLeaf))),
	}
	return v, v.Validate()
}

// GitEnvironmentEntries returns the HOME and XDG_CONFIG_HOME entries a Git
// subprocess receives, built from exactly the frozen bytes.
func (v EnvironmentProfile) GitEnvironmentEntries() []string {
	return []string{"HOME=" + string(v.Home.Bytes()), "XDG_CONFIG_HOME=" + string(v.XDGConfigHome.Bytes())}
}
