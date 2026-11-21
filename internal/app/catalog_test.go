package app

import (
	"bytes"
	"context"
	"flag"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/germanmunozs/ai-dev/internal/gate"
	"github.com/germanmunozs/ai-dev/internal/workflow"
)

// unusedGateStore lets workflow approve and abandon reach their flag
// parsing; a parse failure never commits.
type unusedGateStore struct{ t *testing.T }

func (s unusedGateStore) Commit(string, workflow.State, gate.Evidence) (gate.CommitStatus, []string) {
	s.t.Fatal("a usage failure reached the gate store")
	return 0, nil
}

func catalogTestDependencies(t *testing.T, stdout, stderr *bytes.Buffer) Dependencies {
	return Dependencies{
		Executor: &fakeExecutor{}, Launcher: &appRunLauncher{}, Root: fstest.MapFS{}, GateStore: unusedGateStore{t},
		Git: unavailableGitManagementDependencies(), Stdin: bytes.NewReader(nil), Stdout: stdout, Stderr: stderr,
	}
}

// catalogCommands lists the catalog's command paths, in order.
func catalogCommands() []string {
	var paths []string
	for _, spec := range catalog {
		if spec.usage != "" {
			paths = append(paths, strings.Join(spec.path, " "))
		}
	}
	return paths
}

// The catalog holds exactly the routed commands and groups (F010-FR-001).
func TestCatalogCoversRoutedPaths(t *testing.T) {
	commands := []string{
		"doctor", "agents", "agent select", "agent run",
		"workflow state", "workflow transition", "workflow next", "workflow guide", "workflow approve", "workflow abandon",
		"init", "init inspect", "init plan", "init apply", "init reconcile",
		"git commit validate", "setup", "setup show", "setup apply", "version", "help", "completion",
	}
	if got := catalogCommands(); !slices.Equal(got, commands) {
		t.Fatalf("catalog commands:\n%v\nwant:\n%v", got, commands)
	}
	var groups []string
	for _, spec := range catalog {
		if isGroup(spec.path) {
			groups = append(groups, strings.Join(spec.path, " "))
		}
	}
	slices.Sort(groups)
	if want := []string{"agent", "git", "git commit", "init", "setup", "workflow"}; !slices.Equal(groups, want) {
		t.Fatalf("catalog groups %v, want %v", groups, want)
	}
	seen := map[string]bool{}
	for _, spec := range catalog {
		key := strings.Join(spec.path, " ")
		if seen[key] {
			t.Fatalf("duplicate catalog path %q", key)
		}
		seen[key] = true
		if spec.description == "" || spec.usage == "" && !isGroup(spec.path) {
			t.Fatalf("incomplete catalog entry %q", key)
		}
	}
}

// A group's usage line names exactly the subcommands the catalog routes
// under it (F010-FR-010, Amendment 1). The list is in <…>, or in […] for a
// group that is also a command, like setup (F011 T026).
func TestGroupUsageNamesEverySubcommand(t *testing.T) {
	for group, usage := range map[string]string{"agent": agentGroupUsage, "workflow": workflowGroupUsage, "setup": setupGroupUsage} {
		var names []string
		for _, spec := range children([]string{group}) {
			names = append(names, spec.path[1])
		}
		start, end := strings.Index(usage, "<"), strings.Index(usage, ">")
		if start < 0 {
			start, end = strings.Index(usage, "["), strings.Index(usage, "]")
		}
		if start < 0 || end < start {
			t.Fatalf("%s group usage %q has no subcommand list", group, usage)
		}
		listed := strings.Split(usage[start+1:end], "|")
		slices.Sort(names)
		slices.Sort(listed)
		if !slices.Equal(listed, names) {
			t.Errorf("%s group usage %q lists %v, catalog routes %v", group, usage, listed, names)
		}
	}
}

// For every command, the catalog's options are the options its flag set
// defines (F010-FR-009, SC-004), and its usage line is the line it writes
// on a usage failure (SC-002).
func TestCatalogMatchesEachCommand(t *testing.T) {
	t.Cleanup(func() { flagSetObserver = nil })
	for _, spec := range catalog {
		if spec.usage == "" {
			continue
		}
		path := strings.Join(spec.path, " ")
		var sets []*flag.FlagSet
		flagSetObserver = func(set *flag.FlagSet) { sets = append(sets, set) }
		var stdout, stderr bytes.Buffer
		code := RunWithDependencies(context.Background(), append(slices.Clone(spec.path), "--zz-undefined"), catalogTestDependencies(t, &stdout, &stderr))
		flagSetObserver = nil
		if code != 2 || stdout.Len() != 0 || stderr.String() != spec.usage+"\n" {
			t.Errorf("%s --zz-undefined: exit %d, stdout %q, stderr %q; want usage %q", path, code, stdout.String(), stderr.String(), spec.usage)
		}
		var defined []string
		if len(sets) > 0 {
			sets[len(sets)-1].VisitAll(func(f *flag.Flag) { defined = append(defined, f.Name) })
		}
		var listed []string
		for _, option := range spec.options {
			listed = append(listed, option.name)
			// Completion counts positionals by whether an option takes a
			// value, so the catalog's kind must match the flag's.
			if len(sets) > 0 {
				if f := sets[len(sets)-1].Lookup(option.name); f != nil {
					boolean, _ := f.Value.(interface{ IsBoolFlag() bool })
					if (boolean != nil && boolean.IsBoolFlag()) != (option.value == "") {
						t.Errorf("%s --%s: catalog value %q disagrees with the flag kind", path, option.name, option.value)
					}
				}
			}
		}
		slices.Sort(defined)
		slices.Sort(listed)
		if len(sets) > 1 || !slices.Equal(defined, listed) {
			t.Errorf("%s: %d flag sets; defined options %v, catalog options %v", path, len(sets), defined, listed)
		}
	}
}

func TestCatalogDescriptionsMatchTheGeneralHelp(t *testing.T) {
	_, help, _, _, _ := runTopLevel(t, "help")
	for _, spec := range catalog {
		if spec.usage == "" {
			continue
		}
		line := regexp.MustCompile(`(?m)^  ` + regexp.QuoteMeta(strings.Join(spec.path, " ")) + ` +` + regexp.QuoteMeta(spec.description) + `$`)
		if !line.MatchString(help) {
			t.Errorf("general help lacks %q with %q:\n%s", strings.Join(spec.path, " "), spec.description, help)
		}
	}
}

// Every word the completion scripts embed needs no quoting (F010-FR-008).
// Option values may also hold "=" (setup apply --role ROLE=AGENT, F011
// R-07): they are only embedded inside double quotes, where "=" is inert.
func TestCatalogWordsAreShellSafe(t *testing.T) {
	safe := regexp.MustCompile(`^[A-Za-z0-9._|-]+$`)
	safeValue := regexp.MustCompile(`^[A-Za-z0-9._|=-]+$`)
	check := func(word, where string) {
		if !safe.MatchString(word) {
			t.Errorf("%s: unsafe word %q", where, word)
		}
	}
	for _, spec := range catalog {
		where := strings.Join(spec.path, " ")
		for _, word := range spec.path {
			check(word, where)
		}
		for _, option := range spec.options {
			check(option.name, where)
			for _, value := range option.values {
				if !safeValue.MatchString(value) {
					t.Errorf("%s: unsafe option value %q", where, value)
				}
			}
		}
		for _, arg := range spec.args {
			for _, value := range arg.values {
				check(value, where)
			}
		}
	}
}

func TestLookupFindsTheLongestPath(t *testing.T) {
	for _, test := range []struct {
		args     []string
		path     string
		consumed int
	}{
		{[]string{"init", "plan", "--output", "x"}, "init plan", 2},
		{[]string{"init", "--project-dir", "x"}, "init", 1},
		{[]string{"git", "commit"}, "git commit", 2},
		{[]string{"workflow", "state", "f"}, "workflow state", 2},
		{[]string{"agent", "run", "claude-code", "--help"}, "agent run", 2},
	} {
		spec, consumed, ok := lookup(test.args)
		if !ok || strings.Join(spec.path, " ") != test.path || consumed != test.consumed {
			t.Errorf("lookup(%v) = %v, %d, %t; want %q, %d", test.args, spec.path, consumed, ok, test.path, test.consumed)
		}
	}
	for _, args := range [][]string{nil, {"nope"}, {"--help"}, {""}} {
		if _, _, ok := lookup(args); ok {
			t.Errorf("lookup(%q) matched", args)
		}
	}
}
