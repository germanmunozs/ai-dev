package gitmanager

import "strings"

// initCapabilities are the frozen toolchain capabilities without which
// deterministic initialization cannot be guaranteed (F008-FR-181, 190).
var initCapabilities = []string{"initial-branch", "object-format", "ref-format"}

// initArgv is the exact F008-FR-190 subcommand vector of a new SHA-1,
// files-ref repository: no template, no ambient default branch, object
// format or ref format, and the physical project path after "--".
func initArgv(branch string, root []byte) []string {
	return []string{"init", "--template=", "--object-format=sha1", "--ref-format=files", "--initial-branch=" + branch, "--", string(root)}
}

// checkInitArgv proves that an initialization argv is exactly the
// F008-FR-190 form for the intent's initial branch and the physical root
// under a toolchain with every mandatory capability. Only "-c" overrides
// of the controlled environment may precede the subcommand. It is checked
// when the plan is derived and again before the dispatcher runs Git.
func checkInitArgv(intent ImmutableIntent, root []byte, argv []string) *PlanRejection {
	for _, capability := range initCapabilities {
		if !containsValue(intent.ToolIdentity.Capabilities(), capability) {
			return reject(ReasonGitCapabilityMissing, "the Git toolchain lacks %s", capability)
		}
	}
	if !intent.InitializeRepository || intent.ObjectFormat != ObjectFormatSHA1 || intent.RefFormat != RefFormatFiles {
		return reject(ReasonPlanInvalid, "the intent does not initialize a SHA-1 files-ref repository")
	}
	branch := initialBranchOf(intent.Strategy, intent.BranchParameters())
	if branch == "" || branch == "HEAD" {
		return reject(ReasonInvalidStrategy, "the initial branch is not an explicit name")
	}
	i := 0
	for i+1 < len(argv) && argv[i] == "-c" && !initOverride(argv[i+1]) {
		i += 2
	}
	if !equalLists(argv[i:], initArgv(branch, root)) {
		return reject(ReasonPlanInvalid, "the initialization argv is not the exact deterministic form")
	}
	return nil
}

// initOverride reports a "-c" value that would change what git init
// creates: any init.* default or a shared-repository permission mode.
func initOverride(value string) bool {
	key := strings.ToLower(value)
	return strings.HasPrefix(key, "init.") || strings.HasPrefix(key, "core.sharedrepository")
}
