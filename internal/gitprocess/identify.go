package gitprocess

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/germanmunozs/ai-dev/internal/gitmanager"
)

// minimumGitVersion is the first release with `git init --ref-format`.
var minimumGitVersion = [3]int{2, 45, 0}

// identifiedCapabilities are the matrix capabilities every Git at or above
// minimumGitVersion provides. The config option separator comes from Git's
// shared option parser, which accepts "--" in every supported release.
var identifiedCapabilities = []string{CapabilityConfigEndOfOptions, CapabilityInitialBranch, CapabilityObjectFormat,
	CapabilityPathFormat, CapabilityRefFormat}

// developerDirSuffix is where a macOS developer directory keeps Git's exec
// path; the launcher's DEVELOPER_DIR is derived from it.
const developerDirSuffix = "/usr/libexec/git-core"

var gitVersionPattern = regexp.MustCompile(`^git version (\d+)\.(\d+)\.(\d+)`)

// Identify freezes the Git toolchain once: the invocation path and the
// executable it resolves to, their digests and file identities, the
// reported version and exec path, and the capabilities the operation
// matrix relies on. There is no fallback to another executable.
//
// Direct mode requires the invocation path to be the very Git binary of its
// own exec path, so a launcher can never pass as a direct executable. In
// launcher mode (the macOS /usr/bin/git shim) the effective executable is
// the git binary of the exec path, and DEVELOPER_DIR is frozen to the
// developer directory that exec path belongs to, so later invocations
// resolve the same toolchain whatever xcode-select says.
func Identify(ctx context.Context, mode gitmanager.InvocationMode, invocationPath string, selectors map[string]string,
	environment gitmanager.EnvironmentProfile, directory []byte) (gitmanager.GitToolIdentity, error) {
	refuse := func(reason gitmanager.Reason, detail string) (gitmanager.GitToolIdentity, error) {
		return gitmanager.GitToolIdentity{}, refuse(OpIdentifyVersion, reason, detail)
	}
	if !mode.Valid() {
		return refuse(gitmanager.ReasonPlanInvalid, "unknown invocation mode")
	}
	if !filepath.IsAbs(invocationPath) {
		return refuse(gitmanager.ReasonPlanInvalid, "Git invocation path is not absolute")
	}
	versionOutput, err := runIdentify(ctx, OpIdentifyVersion, invocationPath, selectors, environment, directory)
	if err != nil {
		return gitmanager.GitToolIdentity{}, err
	}
	version, err := parseGitVersion(versionOutput)
	if err != nil {
		return gitmanager.GitToolIdentity{}, err
	}
	execPathOutput, err := runIdentify(ctx, OpIdentifyExecPath, invocationPath, selectors, environment, directory)
	if err != nil {
		return gitmanager.GitToolIdentity{}, err
	}
	execPath := string(bytes.TrimSuffix(execPathOutput, []byte("\n")))
	if !filepath.IsAbs(execPath) || bytes.ContainsAny([]byte(execPath), "\n\x00") {
		return refuse(gitmanager.ReasonGitCapabilityMissing, "Git reported an unusable exec path")
	}

	invocation, err := executableIdentity(invocationPath)
	if err != nil {
		return refuse(gitmanager.ReasonGitCapabilityMissing, err.Error())
	}
	effectivePath, err := filepath.EvalSymlinks(filepath.Join(execPath, "git"))
	if err != nil {
		return refuse(gitmanager.ReasonGitCapabilityMissing, "Git binary of the exec path: "+err.Error())
	}
	effective, err := executableIdentity(effectivePath)
	if err != nil {
		return refuse(gitmanager.ReasonGitCapabilityMissing, err.Error())
	}
	sameFile := invocation.device == effective.device && invocation.inode == effective.inode
	// Some distributions (Debian, Ubuntu) install the Git binary twice, as
	// separate files with identical bytes. Identical content is the same
	// program, so it is not a launcher either; a shim always differs.
	sameContent := invocation.digest != "" && invocation.digest == effective.digest
	switch mode {
	case gitmanager.InvocationModeDirect:
		if !sameFile && !sameContent {
			return refuse(gitmanager.ReasonGitCapabilityMissing, invocationPath+" is a launcher, not the Git binary of its exec path")
		}
		effectivePath, effective = invocationPath, invocation
	case gitmanager.InvocationModeLauncher:
		if sameFile {
			return refuse(gitmanager.ReasonGitCapabilityMissing, invocationPath+" is the Git binary itself, not a launcher")
		}
		developerDir, found := strings.CutSuffix(execPath, developerDirSuffix)
		if !found || developerDir == "" {
			return refuse(gitmanager.ReasonGitCapabilityMissing, "exec path "+execPath+" is not inside a developer directory")
		}
		if supplied, ok := selectors["DEVELOPER_DIR"]; ok && supplied != developerDir {
			return refuse(gitmanager.ReasonGitCapabilityMissing, "DEVELOPER_DIR does not name the resolved developer directory")
		}
		selectors = map[string]string{"DEVELOPER_DIR": developerDir}
		// The frozen selector must resolve to the same exec path.
		frozen, err := runIdentify(ctx, OpIdentifyExecPath, invocationPath, selectors, environment, directory)
		if err != nil {
			return gitmanager.GitToolIdentity{}, err
		}
		if string(bytes.TrimSuffix(frozen, []byte("\n"))) != execPath {
			return refuse(gitmanager.ReasonGitCapabilityMissing, "frozen DEVELOPER_DIR resolves to another exec path")
		}
	}
	tool, err := gitmanager.NewGitToolIdentity(gitmanager.GitToolIdentityInput{
		InvocationMode: mode, InvocationPath: invocationPath, InvocationPathDigest: invocation.digest,
		InvocationDevice: invocation.device, InvocationInode: invocation.inode,
		EffectiveExecutablePath: effectivePath, EffectiveExecutableDigest: effective.digest,
		Version: version, ExecPath: execPath, Capabilities: identifiedCapabilities, EnvironmentSelectors: selectors,
	})
	if err != nil {
		return refuse(gitmanager.ReasonGitCapabilityMissing, err.Error())
	}
	return tool, nil
}

func runIdentify(ctx context.Context, op Operation, executable string, selectors map[string]string,
	environment gitmanager.EnvironmentProfile, directory []byte) ([]byte, error) {
	invocation, err := BuildIdentify(op, executable, selectors, environment, directory)
	if err != nil {
		return nil, err
	}
	result, err := Runner{}.Run(ctx, invocation)
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return nil, refuse(op, gitmanager.ReasonGitCapabilityMissing, fmt.Sprintf("git exited %d", result.ExitCode))
	}
	return result.Stdout, nil
}

// parseGitVersion returns the numeric version and refuses any Git older
// than minimumGitVersion.
func parseGitVersion(output []byte) (string, error) {
	match := gitVersionPattern.FindSubmatch(output)
	if match == nil {
		return "", refuse(OpIdentifyVersion, gitmanager.ReasonGitCapabilityMissing, fmt.Sprintf("unrecognized version output %q", output))
	}
	var parts [3]int
	for i := range parts {
		parts[i], _ = strconv.Atoi(string(match[i+1]))
	}
	for i := range parts {
		if parts[i] != minimumGitVersion[i] {
			if parts[i] < minimumGitVersion[i] {
				return "", refuse(OpIdentifyVersion, gitmanager.ReasonGitCapabilityMissing,
					fmt.Sprintf("Git %d.%d.%d is older than the required 2.45.0", parts[0], parts[1], parts[2]))
			}
			break
		}
	}
	return fmt.Sprintf("%d.%d.%d", parts[0], parts[1], parts[2]), nil
}

type fileIdentity struct {
	digest gitmanager.ContentDigest
	device string
	inode  string
}

// executableIdentity identifies the regular file a path resolves to and
// digests its content.
func executableIdentity(path string) (fileIdentity, error) {
	file, err := os.Open(path)
	if err != nil {
		return fileIdentity{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fileIdentity{}, err
	}
	if !info.Mode().IsRegular() {
		return fileIdentity{}, fmt.Errorf("%s is not a regular file", path)
	}
	device, inode, err := deviceInode(info)
	if err != nil {
		return fileIdentity{}, err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fileIdentity{}, err
	}
	return fileIdentity{digest: gitmanager.ContentDigest(fmt.Sprintf("sha256:%x", hash.Sum(nil))), device: device, inode: inode}, nil
}
