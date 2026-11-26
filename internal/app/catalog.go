package app

import (
	"slices"
	"strings"

	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/projectconfig"
)

// The command catalog is the single source of the general help, each
// command's help and both completion scripts (F010-FR-009). A test proves
// that each command's options equal the options its flag set defines and
// that its usage line is the line it writes on a usage failure.

// argSpec is one positional argument.
type argSpec struct {
	name        string
	values      []string
	file        bool // a file name, completed after "--"
	description string
}

// optionSpec is one option, named as its flag set defines it. The help
// option is rendered by the help itself and is not listed here.
type optionSpec struct {
	name        string
	value       string // placeholder; empty for a boolean option
	values      []string
	valuesHelp  string // states values in help instead of listing them; completion keeps values
	file        bool   // the value is a file or directory
	repeatable  bool
	description string
}

// commandSpec is one command or group path. A group is a path that other
// paths extend; a command has a usage line. init is both.
type commandSpec struct {
	path        []string
	description string
	usage       string
	args        []argSpec
	options     []optionSpec
	changes     string
	exitCodes   string
}

const (
	changesNothing       = "nothing"
	changesWorkflowState = "workflow-state.json"
	changesRepository    = "the repository and its project configuration"
	changesPlanFile      = "the plan file only"
	changesAgent         = "whatever the agent does, under its own sandbox"
	changesConfiguration = "the project configuration file .ai-dev/agents.json"

	exitCodesDomain = "0 positive result, 1 negative domain result, 2 usage or internal failure."
	exitCodesPlain  = "0 success, 2 usage failure."
)

var (
	formatValues   = []string{"text", "json"}
	strategyValues = []string{"basic", "git-flow", "github-flow", "trunk-based"}
	// Agents and roles come from the catalog and the role list, so a new
	// catalog agent appears in help and completion unchanged (F011-FR-003).
	agentValues     = catalogAgentValues()
	roleValues      = roleNames()
	roleAgentValues = roleAgentPairs()
	shellValues     = []string{"bash", "zsh"}
)

func catalogAgentValues() []string {
	var values []string
	for _, a := range agent.Catalog() {
		values = append(values, string(a.ID))
	}
	return values
}

func roleNames() []string {
	var values []string
	for _, r := range projectconfig.Roles() {
		values = append(values, string(r))
	}
	return values
}

// roleAgentPairs are the ROLE=AGENT values of setup apply --role (R-07).
func roleAgentPairs() []string {
	var values []string
	for _, r := range roleNames() {
		for _, a := range catalogAgentValues() {
			values = append(values, r+"="+a)
		}
	}
	return values
}

// roleAgentHelp states the ROLE=AGENT values in help as the roles and the
// agents, once each, instead of every pair (Feature 010, Amendment 2).
func roleAgentHelp() string {
	return "ROLE: " + strings.Join(roleNames(), "|") + "; AGENT: " + strings.Join(catalogAgentValues(), "|")
}

func formatOption() optionSpec {
	return optionSpec{name: "format", value: "FORMAT", values: formatValues, description: "Output format (default text)"}
}

func textOption(name, value, description string) optionSpec {
	return optionSpec{name: name, value: value, description: description}
}

func fileOption(name, value, description string) optionSpec {
	return optionSpec{name: name, value: value, file: true, description: description}
}

func boolOption(name, description string) optionSpec {
	return optionSpec{name: name, description: description}
}

func featureArg() argSpec {
	return argSpec{name: "feature-id", description: "The feature, as named under specs/"}
}

func targetArg() argSpec {
	return argSpec{name: "target-state", description: "The Workflow State to move to"}
}

func initPlanOptions() []optionSpec {
	options := []optionSpec{
		fileOption("project-dir", "PATH", "Project directory (default .)"),
		{name: "strategy", value: "STRATEGY", values: strategyValues, description: "Git strategy"},
		boolOption("initialize-repository", "Create the repository"),
		{name: "git-flow-dialect", value: "DIALECT", values: []string{"avh", "git-flow-next"}, description: "git-flow configuration dialect"},
		textOption("development-base-oid", "OID", "Commit the development branch starts from"),
		fileOption("output", "FILE", "Where to write the plan; must be outside the project"),
		formatOption(),
	}
	for _, name := range []string{"initial-branch", "production-branch", "development-branch", "trunk-branch"} {
		options = append(options, textOption(name, "NAME", "Branch name: "+strings.TrimSuffix(name, "-branch")))
	}
	for _, name := range []string{"feature", "bugfix", "release", "hotfix", "support", "version-tag"} {
		options = append(options, textOption(name+"-prefix", "PREFIX", "git-flow "+name+" prefix"))
	}
	options = append(options,
		boolOption("initial-commit", "Plan the initial commit"),
		fileOption("initial-commit-message-file", "FILE", "Initial commit message"),
		textOption("author-name", "NAME", "Initial commit author name"),
		textOption("author-email", "EMAIL", "Initial commit author email"),
		textOption("committer-name", "NAME", "Initial commit committer name"),
		textOption("committer-email", "EMAIL", "Initial commit committer email"),
		textOption("author-date", "RFC3339", "Initial commit author date"),
		textOption("committer-date", "RFC3339", "Initial commit committer date"),
		optionSpec{name: "include", value: "PATH", file: true, repeatable: true, description: "File to include in the initial commit"},
	)
	return options
}

func workflowGateSpec(verb, usage, description, reason string) commandSpec {
	return commandSpec{
		path: []string{"workflow", verb}, description: description, usage: usage,
		args:      []argSpec{featureArg(), targetArg()},
		options:   []optionSpec{formatOption(), textOption("approver", "IDENTITY", "Who decides (a declared identity)"), textOption("reason", "TEXT", reason)},
		changes:   changesWorkflowState,
		exitCodes: exitCodesDomain,
	}
}

var catalog = []commandSpec{
	{path: []string{"doctor"}, description: "Check that the environment and the repository are ready", usage: doctorUsage,
		options: []optionSpec{formatOption()}, changes: changesNothing, exitCodes: exitCodesDomain},
	{path: []string{"agents"}, description: "List the supported coding agents and their local availability", usage: agentsUsage,
		options: []optionSpec{formatOption()}, changes: changesNothing, exitCodes: exitCodesDomain},
	{path: []string{"agent"}, description: "Select or run a coding agent"},
	{path: []string{"agent", "select"}, description: "Show which coding agent a development role maps to", usage: agentSelectUsage,
		args:    []argSpec{{name: "role", values: roleValues, description: "Development role"}},
		options: []optionSpec{formatOption()}, changes: changesNothing, exitCodes: exitCodesDomain},
	{path: []string{"agent", "run"}, description: "Run a coding agent with an instruction (argument or stdin)", usage: agentRunUsage,
		args: []argSpec{
			{name: "agent-id", values: agentValues, description: "Coding agent"},
			{name: "instruction", description: "The instruction; read from non-interactive stdin when omitted"},
		},
		options: []optionSpec{formatOption()}, changes: changesAgent, exitCodes: exitCodesDomain},
	{path: []string{"workflow"}, description: "Report, evaluate and decide a feature's Workflow State"},
	{path: []string{"workflow", "state"}, description: "Report a feature's Workflow State", usage: workflowStateUsage,
		args: []argSpec{featureArg()}, options: []optionSpec{formatOption()}, changes: changesNothing, exitCodes: exitCodesDomain},
	{path: []string{"workflow", "transition"}, description: "Check whether a Workflow State transition is valid", usage: workflowTransitionUsage,
		args: []argSpec{featureArg(), targetArg()}, options: []optionSpec{formatOption()}, changes: changesNothing, exitCodes: exitCodesDomain},
	{path: []string{"workflow", "next"}, description: "Recommend the next Spec-Driven Development step", usage: workflowNextUsage,
		args: []argSpec{featureArg()}, options: []optionSpec{formatOption()}, changes: changesNothing, exitCodes: exitCodesDomain},
	{path: []string{"workflow", "guide"}, description: "Guide the next human actions, with command templates (read-only)", usage: workflowGuideUsage,
		args: []argSpec{featureArg()}, options: []optionSpec{formatOption()}, changes: changesNothing, exitCodes: exitCodesDomain},
	workflowGateSpec("approve", workflowApproveUsage, "Record a human approval and apply its transition", "Why (optional)"),
	workflowGateSpec("abandon", workflowAbandonUsage, "Record a human abandonment", "Why (required)"),
	{path: []string{"init"}, description: "Initialize Git interactively (needs a terminal)", usage: gitWizardUsage,
		options: []optionSpec{fileOption("project-dir", "PATH", "Project directory (default .)")},
		changes: changesRepository + ", only after the plan digest is confirmed", exitCodes: exitCodesDomain},
	{path: []string{"init", "inspect"}, description: "Inspect the project and its repository", usage: gitInitUsage,
		options: []optionSpec{fileOption("project-dir", "PATH", "Project directory (default .)"), formatOption()},
		changes: changesNothing, exitCodes: exitCodesDomain},
	{path: []string{"init", "plan"}, description: "Write a deterministic Git initialization plan", usage: gitPlanUsage,
		options: initPlanOptions(), changes: changesPlanFile, exitCodes: exitCodesDomain},
	{path: []string{"init", "apply"}, description: "Apply a confirmed plan under a human authorization", usage: gitApplyUsage,
		options: []optionSpec{
			fileOption("plan", "FILE", "The plan file"),
			textOption("confirm-plan", "sha256:DIGEST", "The plan digest, as printed by init plan"),
			textOption("authorization-id", "UUID", "Authorization to issue or consume"),
			boolOption("issue-authorization", "Issue the authorization now"),
			textOption("authorized-by", "TEXT", "Who authorizes (a declared identity)"),
			textOption("authorization-expires-at", "RFC3339", "When the authorization expires"),
			boolOption("confirm-initial-commit", "Confirm the planned initial commit"),
			{name: "approve-file", value: "TUPLE", repeatable: true, description: "Approve one planned file, as printed by init plan"},
			{name: "approve-ignored-file", value: "TUPLE", repeatable: true, description: "Approve one planned ignored file"},
			textOption("reconciles-authorization", "UUID", "The interrupted authorization this one reconciles"),
			formatOption(),
		},
		changes: changesRepository, exitCodes: exitCodesDomain},
	{path: []string{"init", "reconcile"}, description: "Report the reconciliation of an interrupted apply", usage: gitReconcileUsage,
		options: []optionSpec{fileOption("plan", "FILE", "The plan file"), textOption("authorization-id", "UUID", "The interrupted authorization"), formatOption()},
		changes: changesNothing, exitCodes: exitCodesDomain},
	{path: []string{"git"}, description: "Git helpers"},
	{path: []string{"git", "commit"}, description: "Commit message helpers"},
	{path: []string{"git", "commit", "validate"}, description: "Validate a Conventional Commits message", usage: gitCommitValidateUsage,
		options: []optionSpec{textOption("message", "TEXT", "The message"), fileOption("file", "FILE", "A file holding the message"), formatOption()},
		changes: changesNothing, exitCodes: exitCodesDomain},
	{path: []string{"setup"}, description: "Choose the project's agents and assign one to each role (needs a terminal)", usage: setupWizardUsage,
		changes: changesConfiguration + ", only after confirmation", exitCodes: exitCodesDomain},
	{path: []string{"setup", "show"}, description: "Show the detected agents and the role assignment in effect", usage: setupShowUsage,
		options: []optionSpec{formatOption()}, changes: changesNothing, exitCodes: exitCodesDomain},
	{path: []string{"setup", "apply"}, description: "Write the project's agents and role assignment from options", usage: setupApplyUsage,
		options: []optionSpec{
			{name: "agent", value: "AGENT", values: agentValues, repeatable: true, description: "An agent the project works with"},
			{name: "role", value: "ROLE=AGENT", values: roleAgentValues, valuesHelp: roleAgentHelp(), repeatable: true, description: "The agent for one role; give all five"},
			boolOption("replace", "Replace an existing, different configuration"),
			formatOption(),
		},
		changes: changesConfiguration, exitCodes: exitCodesDomain},
	{path: []string{"tui"}, description: "Open the interactive terminal cockpit (needs a terminal)", usage: cockpitUsage,
		changes: "nothing by itself; actions run their own commands", exitCodes: "0 on q or Ctrl-C, 130 on an interrupt signal, 143 on termination, 2 usage failure or no terminal."},
	{path: []string{"open"}, description: "Open a project file at a line in Neovim", usage: openUsage,
		args: []argSpec{{name: "path", file: true, description: "A file of the project, relative to the current directory, after --"}},
		options: []optionSpec{
			formatOption(),
			textOption("line", "N", "Line to open, from 1 (default 1)"),
			textOption("column", "N", "Column to open, from 1 (default 1)"),
		},
		changes: "nothing in the project; opens the file in Neovim", exitCodes: exitCodesDomain},
	{path: []string{"watch"}, description: "Report project changes as they happen, read-only", usage: watchUsage,
		options: []optionSpec{
			formatOption(),
			{name: "feature", value: "ID", repeatable: true, description: "Report only this feature"},
			textOption("debounce", "DURATION", "Group changes within this time, 50ms to 5s (default 250ms)"),
			textOption("heartbeat", "DURATION", "Emit a heartbeat this often, 5s to 300s (default 15s)"),
			textOption("environment-interval", "DURATION", "Check the environment again this often, 5s to 3600s, or 0 for never (default 60s)"),
			boolOption("once", "Emit the snapshot only, then exit"),
		},
		changes: "nothing; reads the project and reports its changes", exitCodes: exitCodesDomain},
	{path: []string{"version"}, description: "Print which build of ai-dev is running", usage: versionUsage,
		options: []optionSpec{formatOption()}, changes: changesNothing, exitCodes: exitCodesPlain},
	{path: []string{"help"}, description: "Print this help, or one command's help", usage: helpUsage,
		args: []argSpec{{name: "command", description: "A command or group, for example: workflow state"}}, changes: changesNothing, exitCodes: exitCodesPlain},
	{path: []string{"completion"}, description: "Print a bash or zsh completion script", usage: completionUsage,
		args: []argSpec{{name: "shell", values: shellValues, description: "Target shell"}}, changes: changesNothing, exitCodes: exitCodesPlain},
}

// isGroup reports whether another catalog path extends path.
func isGroup(path []string) bool {
	return len(children(path)) > 0
}

// children lists the catalog entries that extend path by one word.
func children(path []string) []commandSpec {
	var out []commandSpec
	for _, spec := range catalog {
		if len(spec.path) == len(path)+1 && slices.Equal(spec.path[:len(path)], path) {
			out = append(out, spec)
		}
	}
	return out
}

// lookup finds the longest catalog path that prefixes args and returns it
// with the number of arguments it consumes.
func lookup(args []string) (commandSpec, int, bool) {
	var best commandSpec
	found := false
	for _, spec := range catalog {
		if len(spec.path) <= len(args) && slices.Equal(spec.path, args[:len(spec.path)]) && len(spec.path) > len(best.path) {
			best, found = spec, true
		}
	}
	return best, len(best.path), found
}
