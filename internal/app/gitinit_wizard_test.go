package app

import (
	"bytes"
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var digestPattern = regexp.MustCompile(`Plan (sha256:[0-9a-f]{64})`)

func typeDigest(output string) string {
	if match := digestPattern.FindStringSubmatch(output); match != nil {
		return match[1]
	}
	return "missing"
}

func (f *gitAppFixture) runWizard(interactive bool, answers ...func(string) string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	terminal := &scriptedTerminal{out: &stdout, answers: answers}
	code := RunGitWizard(context.Background(), []string{"--project-dir", f.project}, f.deps, terminal, interactive, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestWizardRequiresATerminal(t *testing.T) {
	f := newGitAppFixture(t)
	code, stdout, stderr := f.runWizard(false, literal("1"))
	if code != 2 || !strings.Contains(stderr, "interactive_terminal_required") || stdout != "" {
		t.Fatalf("exit %d stdout %q stderr %q", code, stdout, stderr)
	}
	if f.identify != 0 || f.reader.calls != 0 || f.stateExists() || f.random.count() != 0 {
		t.Fatal("the assistant inspected, planned or created state without a terminal")
	}
}

func TestWizardFullFlow(t *testing.T) {
	f := newGitAppFixture(t)
	destination := filepath.Join(f.base, "plan.json")
	var randomAtConfirmation, authorizationsAtConfirmation int
	code, stdout, stderr := f.runWizard(true,
		literal("1"), literal("yes"), literal("main"), literal(destination), literal("yes"),
		func(output string) string {
			randomAtConfirmation, authorizationsAtConfirmation = f.random.count(), f.authorizations()
			return typeDigest(output)
		},
		literal("operator"))
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout, stderr)
	}
	// Four strategies, none preselected.
	for _, name := range []string{"1) basic", "2) git-flow", "3) github-flow", "4) trunk-based", "no default"} {
		if !strings.Contains(stdout, name) {
			t.Fatalf("missing %q in\n%s", name, stdout)
		}
	}
	// The complete plan is displayed before the destination is asked.
	if strings.Index(stdout, "repository_init") > strings.Index(stdout, "Plan destination") ||
		!strings.Contains(stdout, "project_config_write") || !strings.Contains(stdout, "Write the plan to "+destination) {
		t.Fatalf("plan display\n%s", stdout)
	}
	// Before the digest is typed only the project ID was drawn and no
	// authority exists; the authorization UUID and the apply ID are drawn
	// from the injected source afterwards.
	if randomAtConfirmation != 16 || authorizationsAtConfirmation != 0 || f.random.count() != randomAtConfirmation+32 ||
		strings.Index(stdout, "Authorization ") < strings.Index(stdout, "Type the plan digest") {
		t.Fatalf("random draws %d after confirmation at %d\n%s", f.random.count(), randomAtConfirmation, stdout)
	}
	if f.authorizations() != 1 || f.executor.executed == 0 || !strings.Contains(stdout, "status:         applied") {
		t.Fatalf("authorizations %d executed %d\n%s", f.authorizations(), f.executor.executed, stdout)
	}
}

// No authority exists until every confirmation, including the digest.
func TestWizardRefusalsCreateNoAuthority(t *testing.T) {
	for name, test := range map[string]struct {
		answers []func(string) string
		code    int
	}{
		"cancel initialization": {[]func(string) string{literal("1"), literal("no")}, 1},
		"eof":                   {[]func(string) string{literal("1")}, 1},
		"invalid strategy":      {[]func(string) string{literal("5")}, 2},
		"empty branch":          {[]func(string) string{literal("trunk-based"), literal("yes"), literal("")}, 2},
		"decline destination":   {[]func(string) string{literal("1"), literal("yes"), literal("main"), literal("plan.json"), literal("no")}, 1},
		"wrong digest": {[]func(string) string{literal("1"), literal("yes"), literal("main"), literal("plan.json"), literal("yes"),
			literal("sha256:0000000000000000000000000000000000000000000000000000000000000000")}, 1},
		"invalid confirmation": {[]func(string) string{literal("1"), literal("maybe")}, 2},
	} {
		t.Run(name, func(t *testing.T) {
			f := newGitAppFixture(t)
			code, stdout, stderr := f.runWizard(true, test.answers...)
			if code != test.code {
				t.Fatalf("exit %d, want %d\n%s\n%s", code, test.code, stdout, stderr)
			}
			if f.authorizations() != 0 || f.executor.executed != 0 || strings.Contains(stdout, "Authorization ") {
				t.Fatal("a refused flow created authority or ran operations")
			}
		})
	}
}

func TestWizardStopsBeforeAuthorityWithoutApply(t *testing.T) {
	f := newGitAppFixture(t)
	f.deps.Executor = nil
	code, stdout, _ := f.runWizard(true, literal("4"), literal("yes"), literal("trunk"), literal(filepath.Join(f.base, "p.json")), literal("yes"))
	if code != 0 || !strings.Contains(stdout, "Apply is not enabled yet") || f.authorizations() != 0 || strings.Contains(stdout, "Type the plan digest") {
		t.Fatalf("exit %d\n%s", code, stdout)
	}
}
