package app

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestCompletionCommand(t *testing.T) {
	for _, shell := range []string{"bash", "zsh"} {
		code, first, stderr, _, _ := runTopLevel(t, "completion", shell)
		_, second, _, _, _ := runTopLevel(t, "completion", shell)
		if code != 0 || stderr != "" || first == "" || first != second {
			t.Fatalf("completion %s: exit %d, stderr %q, stable %t", shell, code, stderr, first == second)
		}
		// Static: no value from the environment (F010-FR-008).
		wd, _ := os.Getwd()
		for _, value := range []string{os.Getenv("HOME"), os.Getenv("USER"), os.Getenv("PATH"), wd} {
			if len(value) > 3 && strings.Contains(first, value) {
				t.Fatalf("completion %s embeds an environment value %q", shell, value)
			}
		}
	}
	for _, args := range [][]string{{"completion"}, {"completion", "fish"}, {"completion", "bash", "extra"}, {"completion", "--zz"}} {
		code, stdout, stderr, _, _ := runTopLevel(t, args...)
		if code != 2 || stdout != "" || stderr != completionUsage+"\n" {
			t.Fatalf("%v: exit %d, stdout %q, stderr %q", args, code, stdout, stderr)
		}
	}
}

// completionCases are command lines, the last word being completed, and
// the expected candidates in order; "FILES" means file-name completion.
var completionCases = []struct {
	words []string
	want  []string
}{
	{[]string{"ai-dev", "wo"}, []string{"workflow"}},
	{[]string{"ai-dev", ""}, []string{"doctor", "agents", "agent", "workflow", "init", "git", "version", "help", "completion"}},
	{[]string{"ai-dev", "workflow", ""}, []string{"state", "transition", "next", "guide", "approve", "abandon"}},
	{[]string{"ai-dev", "workflow", "state", "--f"}, []string{"--format"}},
	{[]string{"ai-dev", "workflow", "state", "--format", ""}, []string{"text", "json"}},
	{[]string{"ai-dev", "init", "plan", "--strategy", ""}, []string{"basic", "git-flow", "github-flow", "trunk-based"}},
	{[]string{"ai-dev", "agent", "run", ""}, []string{"claude-code", "codex-cli"}},
	{[]string{"ai-dev", "agent", "run", "--format", "json", "c"}, []string{"claude-code", "codex-cli"}},
	{[]string{"ai-dev", "agent", "select", "re"}, []string{"review", "remediation"}},
	{[]string{"ai-dev", "help", "init", ""}, []string{"inspect", "plan", "apply", "reconcile"}},
	{[]string{"ai-dev", "help", ""}, []string{"doctor", "agents", "agent", "workflow", "init", "git", "version", "help", "completion"}},
	{[]string{"ai-dev", "completion", ""}, []string{"bash", "zsh"}},
	{[]string{"ai-dev", "git", "commit", ""}, []string{"validate"}},
	{[]string{"ai-dev", "version", "-"}, []string{"--format", "--help"}},
	{[]string{"ai-dev", "init", "plan", "--output", "no"}, []string{"FILES"}},
	{[]string{"ai-dev", "workflow", "state", ""}, nil},
	{[]string{"ai-dev", "workflow", "state", "f", ""}, nil},
	{[]string{"ai-dev", "agent", "run", "claude-code", ""}, nil},
	{[]string{"ai-dev", "init", "plan", "--author-name", ""}, nil},
	{[]string{"ai-dev", "nope", ""}, nil},
	{[]string{"ai-dev", "agent", "run", "--format=json", ""}, []string{"claude-code", "codex-cli"}},
	{[]string{"ai-dev", "agent", "run", "--format=j"}, nil},
	{[]string{"ai-dev", "workflow", "state", "--", ""}, nil},
	{[]string{"ai-dev", "init", "--project-dir", "x", ""}, nil},
	{[]string{"ai-dev", "init", ""}, []string{"inspect", "plan", "apply", "reconcile"}},
}

// bashSplitCases are the words bash 4 and later produce for --option=value,
// because "=" is in COMP_WORDBREAKS: "--option", "=" and the value.
var bashSplitCases = []struct {
	words []string
	want  []string
}{
	{[]string{"ai-dev", "agent", "run", "--format", "=", "json", ""}, []string{"claude-code", "codex-cli"}},
	{[]string{"ai-dev", "agent", "run", "--format", "="}, []string{"text", "json"}},
	{[]string{"ai-dev", "agent", "run", "--format", "=", "j"}, []string{"json"}},
	{[]string{"ai-dev", "agent", "select", "--format", "=", "r"}, nil},
	{[]string{"ai-dev", "git", "commit", "validate", "--file", "=", "no"}, []string{"FILES"}},
	// A file name with a glob character is offered literally, never expanded.
	{[]string{"ai-dev", "init", "plan", "--output", "q*"}, []string{"q*"}},
}

func writeScript(t *testing.T, shell string) string {
	t.Helper()
	code, script, stderr, _, _ := runTopLevel(t, "completion", shell)
	if code != 0 {
		t.Fatalf("completion %s: exit %d %s", shell, code, stderr)
	}
	path := filepath.Join(t.TempDir(), "ai-dev."+shell)
	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// fileFixture is a directory with files whose names start with "no", so
// file-name completion has something known to offer.
func fileFixture(t *testing.T) string {
	dir := t.TempDir()
	for _, name := range []string{"notes.txt", "now.md", "qa", "q*"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func shellWords(words []string) string {
	quoted := make([]string, len(words))
	for i, word := range words {
		quoted[i] = "'" + word + "'"
	}
	return strings.Join(quoted, " ")
}

func checkCandidates(t *testing.T, shell string, words []string, got string, want []string) {
	t.Helper()
	fields := strings.Fields(got)
	if slices.Equal(want, []string{"FILES"}) {
		slices.Sort(fields)
		if !slices.Equal(fields, []string{"notes.txt", "now.md"}) && !slices.Equal(fields, []string{"FILES"}) {
			t.Errorf("%s %v: got %q, want file names", shell, words, got)
		}
		return
	}
	if !slices.Equal(fields, want) {
		t.Errorf("%s %v: got %q, want %q", shell, words, fields, want)
	}
}

func bashBinaries(t *testing.T) []string {
	var shells []string
	if _, err := os.Stat("/bin/bash"); err == nil {
		shells = append(shells, "/bin/bash")
	}
	if path, err := exec.LookPath("bash"); err == nil && !slices.Contains(shells, path) {
		if resolved, _ := filepath.EvalSymlinks(path); resolved != "/bin/bash" {
			shells = append(shells, path)
		}
	}
	if len(shells) == 0 {
		t.Skip("no bash available")
	}
	return shells
}

// The bash script completes every case, in bash 3.2 (macOS /bin/bash) and
// in the current bash (F010-FR-007, FR-008, SC-003).
func TestBashCompletion(t *testing.T) {
	script, dir := writeScript(t, "bash"), fileFixture(t)
	for _, bash := range bashBinaries(t) {
		version, err := exec.Command(bash, "-c", "echo ${BASH_VERSINFO[0]}").Output()
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS == "darwin" && bash == "/bin/bash" && strings.TrimSpace(string(version)) != "3" {
			t.Fatalf("/bin/bash on macOS is version %s, want 3", version)
		}
		for _, test := range slices.Concat(completionCases, bashSplitCases) {
			program := "source '" + script + "'\n" +
				"COMP_WORDS=(" + shellWords(test.words) + ")\nCOMP_CWORD=" + strconv.Itoa(len(test.words)-1) + "\n" +
				"_ai_dev\nprintf '%s\\n' \"${COMPREPLY[@]}\"\n" +
				"complete -p ai-dev\n"
			cmd := exec.Command(bash, "--norc", "--noprofile", "-c", program)
			cmd.Dir = dir
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil || stderr.Len() != 0 {
				t.Fatalf("%s %v: %v, stderr %q", bash, test.words, err, stderr.String())
			}
			lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
			registration := lines[len(lines)-1]
			if registration != "complete -o filenames -F _ai_dev ai-dev" {
				t.Fatalf("%s registration %q", bash, registration)
			}
			checkCandidates(t, bash, test.words, strings.Join(lines[:len(lines)-1], " "), test.want)
		}
	}
}

// The zsh script parses, and completes every case through stub compadd
// and _files functions (F010-FR-007, SC-003).
func TestZshCompletion(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("no zsh available")
	}
	script, dir := writeScript(t, "zsh"), fileFixture(t)
	if out, err := exec.Command(zsh, "-n", script).CombinedOutput(); err != nil {
		t.Fatalf("zsh -n: %v %s", err, out)
	}
	for _, test := range completionCases {
		program := "compdef() { print -r -- \"compdef $*\" }\n" +
			"compadd() { while [[ $# -gt 0 && $1 != -- ]]; do shift; done; shift; local w; for w in \"$@\"; do print -r -- \"$w\"; done }\n" +
			"_files() { print -r -- FILES }\n" +
			"source '" + script + "'\n" +
			"words=(" + shellWords(test.words) + ")\nCURRENT=" + strconv.Itoa(len(test.words)) + "\n" +
			"_ai_dev\n"
		cmd := exec.Command(zsh, "-f", "-c", program)
		cmd.Dir = dir
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil || stderr.Len() != 0 {
			t.Fatalf("zsh %v: %v, stderr %q", test.words, err, stderr.String())
		}
		lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
		if lines[0] != "compdef _ai_dev ai-dev" {
			t.Fatalf("zsh registration %q", lines[0])
		}
		// zsh filters candidates by the typed prefix itself; the stub
		// does not, so filter here as zsh would.
		prefix := test.words[len(test.words)-1]
		var got []string
		for _, line := range lines[1:] {
			if line != "" && (strings.HasPrefix(line, prefix) || line == "FILES") {
				got = append(got, line)
			}
		}
		checkCandidates(t, "zsh", test.words, strings.Join(got, " "), test.want)
	}
}
