package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/germanmunozs/ai-dev/internal/agent"
	"github.com/germanmunozs/ai-dev/internal/guide"
	"github.com/germanmunozs/ai-dev/internal/role"
	"github.com/germanmunozs/ai-dev/internal/sdd"
	"github.com/germanmunozs/ai-dev/internal/selection"
	"github.com/germanmunozs/ai-dev/internal/workflow"
)

const guideNote = `note="options for the human only; not authorization. Running an agent or a review is not approval; approval is your own Human Gate decision after completing the activity"`

type guideTestSelector struct{ outcome selection.Outcome }

func (s guideTestSelector) Select(supplied string) selection.Report {
	requested, _ := role.Recognized(supplied)
	mapped := agent.ClaudeCode
	if requested == role.Implementation {
		mapped = agent.CodexCLI
	}
	report := selection.Report{SuppliedRole: supplied, RequestedRole: requested, Outcome: s.outcome, Source: selection.SourceBuiltIn}
	if s.outcome == selection.ConfigurationInvalid {
		report.Source = selection.SourceProject
		return report
	}
	if s.outcome == selection.RoleSelected {
		report.SelectedAgentID = mapped
	} else {
		report.MappedAgentID = mapped
	}
	return report
}

// guideReport evaluates featureID in state (draft when state is empty), with
// the given selection outcome, and the artifacts that state requires unless
// missing is set.
func guideReport(t *testing.T, featureID, state string, outcome selection.Outcome, missing bool) guide.Report {
	t.Helper()
	root := fstest.MapFS{"specs/" + featureID: &fstest.MapFile{Mode: fs.ModeDir | 0o755}}
	if state != "" {
		root["specs/"+featureID+"/"+workflow.RecordFileName] = &fstest.MapFile{Data: []byte(`{"state":"` + state + `"}`)}
	}
	if !missing {
		for _, name := range []string{sdd.SpecArtifact, sdd.PlanArtifact, sdd.TasksArtifact} {
			root["specs/"+featureID+"/"+name] = &fstest.MapFile{Data: []byte("fixture")}
		}
	}
	report := guide.Evaluate(root, featureID, guideTestSelector{outcome: outcome})
	if !report.Valid() {
		t.Fatalf("fixture report invalid: %+v", report)
	}
	return report
}

func TestWriteGuideTextEveryOutcome(t *testing.T) {
	planned := `{"state":"Planned"}`
	invalidRoot := fstest.MapFS{
		"specs/f":                            &fstest.MapFile{Mode: fs.ModeDir | 0o755},
		"specs/f/" + workflow.RecordFileName: &fstest.MapFile{Data: []byte(planned)},
	}
	cases := []struct {
		name   string
		report guide.Report
		want   string
	}{
		{"step available", guideReport(t, "f", "specified", selection.RoleSelected, false),
			"ai-dev: outcome=step_recommended feature=\"f\" state=specified activity=plan role=planning agent=claude-code availability=available source=built_in " + guideNote + "\n" +
				"  # select the agent for this role (003)\n  ai-dev agent select -- planning\n" +
				"  # run the agent (002)\n  ai-dev agent run -- claude-code )instruction(\n" +
				"  # after completing the activity, check the transition (004)\n  ai-dev workflow transition -- 'f' )target-state(\n" +
				"  # then record your decision (006)\n  ai-dev workflow approve --approver )approver( -- 'f' )target-state(\n" +
				"  # replace each )slot( before running; --reason )reason( is optional for approval (006 FR-010)\n"},
		{"step unavailable", guideReport(t, "f", "tasked", selection.AgentUnavailable, false),
			"ai-dev: outcome=step_recommended feature=\"f\" state=tasked activity=implement role=implementation agent=codex-cli availability=unavailable source=built_in " + guideNote + "\n" +
				"  # select the agent for this role (003)\n  ai-dev agent select -- implementation\n" +
				"  # after completing the activity, check the transition (004)\n  ai-dev workflow transition -- 'f' )target-state(\n" +
				"  # then record your decision (006)\n  ai-dev workflow approve --approver )approver( -- 'f' )target-state(\n" +
				"  # replace each )slot( before running; --reason )reason( is optional for approval (006 FR-010)\n"},
		{"step inconsistent", guideReport(t, "f", "", selection.SelectionInconsistent, false),
			"ai-dev: outcome=step_recommended feature=\"f\" state=draft activity=specify role=specification agent=claude-code availability=inconsistent source=built_in " + guideNote + "\n" +
				"  # after completing the activity, check the transition (004)\n  ai-dev workflow transition -- 'f' )target-state(\n" +
				"  # then record your decision (006)\n  ai-dev workflow approve --approver )approver( -- 'f' )target-state(\n" +
				"  # replace each )slot( before running; --reason )reason( is optional for approval (006 FR-010)\n"},
		{"terminal", guideReport(t, "f", "approved", selection.RoleSelected, false),
			"ai-dev: outcome=workflow_terminal feature=\"f\" state=approved " + guideNote + "\n" +
				"  # report the state (004)\n  ai-dev workflow state -- 'f'\n"},
		{"artifacts inconsistent", guideReport(t, "f", "planned", selection.RoleSelected, true),
			"ai-dev: outcome=artifacts_inconsistent feature=\"f\" state=planned missing=spec.md,plan.md " + guideNote + "\n"},
		{"feature unknown", guide.Evaluate(fstest.MapFS{}, "missing", guideTestSelector{}),
			"ai-dev: outcome=feature_unknown feature=\"missing\" " + guideNote + "\n"},
		{"state invalid", guide.Evaluate(invalidRoot, "f", guideTestSelector{}),
			"ai-dev: outcome=state_invalid feature=\"f\" " + guideNote + "\n"},
		{"quoted identifier", guideReport(t, "it's", "approved", selection.RoleSelected, false),
			"ai-dev: outcome=workflow_terminal feature=\"it's\" state=approved " + guideNote + "\n" +
				"  # report the state (004)\n  ai-dev workflow state -- 'it'\\''s'\n"},
		{"unsafe identifier", guideReport(t, "a"+string(rune(0x202e))+"b", "approved", selection.RoleSelected, false),
			"ai-dev: outcome=workflow_terminal feature=\"a\\u202eb\" state=approved " + guideNote + "\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := WriteGuideText(&out, tc.report); err != nil {
				t.Fatal(err)
			}
			if out.String() != tc.want {
				t.Fatalf("text =\n%s\nwant\n%s", out.String(), tc.want)
			}
			for _, banned := range []string{"approved by", "authorized", "authenticated"} {
				if strings.Contains(out.String(), banned) {
					t.Fatalf("text contains %q", banned)
				}
			}
		})
	}
}

func TestWriteGuideJSONEveryOutcome(t *testing.T) {
	step := guideReport(t, "f", "specified", selection.RoleSelected, false)
	var out bytes.Buffer
	if err := WriteGuideJSON(&out, step); err != nil {
		t.Fatal(err)
	}
	const wantStep = `{"schema_version":"ai-dev.workflow-guide/v1","feature_id":"f","outcome":"step_recommended","current_state":"specified","activity":"plan","role":"planning","agent":"claude-code","agent_availability":"available","source":"built_in","templates":[` +
		`{"kind":"select","label":"select the agent for this role (003)","command":"ai-dev agent select -- planning","human_slots":[]},` +
		`{"kind":"run","label":"run the agent (002)","command":"ai-dev agent run -- claude-code )instruction(","human_slots":["instruction"]},` +
		`{"kind":"transition","label":"after completing the activity, check the transition (004)","command":"ai-dev workflow transition -- 'f' )target-state(","human_slots":["target-state"]},` +
		`{"kind":"approve","label":"then record your decision (006)","command":"ai-dev workflow approve --approver )approver( -- 'f' )target-state(","human_slots":["approver","target-state"],"optional_flags":["--reason"]}],` +
		`"statement":"options for the human only; not authorization. Running an agent or a review is not approval; approval is your own Human Gate decision after completing the activity"}` + "\n"
	if out.String() != wantStep {
		t.Fatalf("json =\n%s\nwant\n%s", out.String(), wantStep)
	}

	cases := []struct {
		name   string
		report guide.Report
		keys   []string
	}{
		{"step", step, []string{"activity", "agent", "agent_availability", "current_state", "feature_id", "outcome", "role", "schema_version", "source", "statement", "templates"}},
		{"terminal", guideReport(t, "f", "abandoned", selection.RoleSelected, false), []string{"current_state", "feature_id", "outcome", "schema_version", "statement", "templates"}},
		{"artifacts inconsistent", guideReport(t, "f", "tasked", selection.RoleSelected, true), []string{"current_state", "feature_id", "missing_artifacts", "outcome", "schema_version", "statement", "templates"}},
		{"feature unknown", guide.Evaluate(fstest.MapFS{}, "missing", guideTestSelector{}), []string{"feature_id", "outcome", "schema_version", "statement", "templates"}},
		{"unsafe identifier", guideReport(t, "a"+string(rune(0x200b))+"b", "", selection.RoleSelected, false), []string{"activity", "agent", "agent_availability", "current_state", "feature_id", "outcome", "role", "schema_version", "source", "statement", "templates"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := WriteGuideJSON(&out, tc.report); err != nil {
				t.Fatal(err)
			}
			decoder := json.NewDecoder(bytes.NewReader(out.Bytes()))
			var object map[string]any
			if err := decoder.Decode(&object); err != nil {
				t.Fatal(err)
			}
			if decoder.More() || !strings.HasSuffix(out.String(), "}\n") || strings.Count(out.String(), "\n") != 1 {
				t.Fatalf("stdout is not exactly one JSON object: %q", out.String())
			}
			keys := make([]string, 0, len(object))
			for key := range object {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			if !reflect.DeepEqual(keys, tc.keys) {
				t.Fatalf("keys = %v, want %v", keys, tc.keys)
			}
			if object["schema_version"] != "ai-dev.workflow-guide/v1" || object["statement"] != guide.Statement || object["outcome"] != string(tc.report.Outcome) || object["feature_id"] != tc.report.FeatureID {
				t.Fatalf("fixed fields = %v", object)
			}
			templates, ok := object["templates"].([]any)
			if !ok || len(templates) != len(tc.report.Templates) {
				t.Fatalf("templates = %#v, want %d entries", object["templates"], len(tc.report.Templates))
			}
		})
	}
}

func TestWriteGuideRefusesInvalidReport(t *testing.T) {
	report := guideReport(t, "f", "specified", selection.RoleSelected, false)
	report.Statement = ""
	for name, write := range map[string]func(*bytes.Buffer) error{
		"text": func(b *bytes.Buffer) error { return WriteGuideText(b, report) },
		"json": func(b *bytes.Buffer) error { return WriteGuideJSON(b, report) },
	} {
		var out bytes.Buffer
		if err := write(&out); err == nil || out.Len() != 0 {
			t.Fatalf("%s: err=%v out=%q, want error and no output", name, err, out.String())
		}
	}
}

func TestWriteGuideTextIsOneWrite(t *testing.T) {
	report := guideReport(t, "f", "specified", selection.RoleSelected, false)
	writer := &guideCountingWriter{}
	if err := WriteGuideText(writer, report); err != nil {
		t.Fatal(err)
	}
	if writer.calls != 1 {
		t.Fatalf("writes = %d, want 1", writer.calls)
	}
	failing := &guideCountingWriter{err: errors.New("closed")}
	if err := WriteGuideText(failing, report); err == nil {
		t.Fatal("write error not returned")
	}
}

type guideCountingWriter struct {
	calls int
	err   error
}

func (w *guideCountingWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.err != nil {
		return 0, w.err
	}
	return len(p), nil
}

// --- SC-004: pasting templates into POSIX shells ---

// pasteShell is one shell, with the options a user may have set, to paste
// templates into.
type pasteShell struct {
	argv    []string
	prelude string
}

// guideShells returns every shell mode to paste into. sh and bash are
// required, and zsh is required in CI; dash is used when installed. zsh is
// pasted into under the option sets that change how it reads parentheses
// (review R7-01).
func guideShells(t *testing.T) map[string]pasteShell {
	t.Helper()
	modes := []struct {
		name, binary string
		args         []string
		prelude      string
	}{
		{"sh", "sh", []string{"-c"}, ""},
		{"dash", "dash", []string{"-c"}, ""},
		{"bash", "bash", []string{"--norc", "--noprofile", "-c"}, ""},
		{"bash extglob", "bash", []string{"--norc", "--noprofile", "-c"}, "shopt -s extglob\n"},
		{"zsh", "zsh", []string{"-f", "-c"}, ""},
		{"zsh emulate sh", "zsh", []string{"-f", "-c"}, "emulate sh\n"},
		{"zsh emulate ksh", "zsh", []string{"-f", "-c"}, "emulate ksh\n"},
		{"zsh shglob nonomatch", "zsh", []string{"-f", "-c"}, "setopt shglob nonomatch\n"},
		{"zsh kshglob", "zsh", []string{"-f", "-c"}, "setopt kshglob\n"},
		{"zsh nobareglobqual nonomatch", "zsh", []string{"-f", "-c"}, "setopt nobareglobqual nonomatch\n"},
	}
	shells := map[string]pasteShell{}
	for _, mode := range modes {
		path, err := exec.LookPath(mode.binary)
		if err != nil {
			if mode.binary == "sh" || mode.binary == "bash" || (mode.binary == "zsh" && os.Getenv("CI") != "") {
				t.Fatalf("%s is required for SC-004: %v", mode.binary, err)
			}
			t.Logf("%s not installed; skipping %q", mode.binary, mode.name)
			continue
		}
		shells[mode.name] = pasteShell{argv: append([]string{path}, mode.args...), prelude: mode.prelude}
	}
	return shells
}

// sentinelPath creates sentinels for ai-dev, both agents and the first word
// of every label and note line, so a label word run as a command is caught.
// Each appends its name and argv to $SENTINEL_LOG, NUL-separated, one
// invocation per line.
func sentinelPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n{ printf '%s' \"$(basename \"$0\")\"; for a in \"$@\"; do printf '\\0%s' \"$a\"; done; printf '\\n'; } >> \"$SENTINEL_LOG\"\n"
	for _, name := range []string{"ai-dev", "claude", "codex", "select", "run", "after", "then", "report", "replace"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func dirListing(t *testing.T, dir string) []string {
	t.Helper()
	var listing []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data := ""
		if entry.Type().IsRegular() {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			data = string(content)
		}
		listing = append(listing, path+"|"+info.Mode().String()+"|"+data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return listing
}

// paste runs command in shell from a fresh working directory that contains
// files named after every slot, and returns the logged invocations and
// whether the working directory and the sentinel log changed.
func paste(t *testing.T, sentinels string, shell pasteShell, command string) (invocations [][]string, dirChanged bool) {
	t.Helper()
	work := t.TempDir()
	for _, name := range []string{guide.SlotInstruction, guide.SlotApprover, guide.SlotTargetState, "reason"} {
		if err := os.WriteFile(filepath.Join(work, name), []byte("keep"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	logDir := t.TempDir()
	log := filepath.Join(logDir, "log")
	before := dirListing(t, work)
	cmd := exec.Command(shell.argv[0], append(shell.argv[1:], shell.prelude+command)...)
	cmd.Dir = work
	cmd.Env = []string{"PATH=" + sentinels + ":/usr/bin:/bin", "HOME=" + logDir, "SENTINEL_LOG=" + log}
	_ = cmd.Run()
	after := dirListing(t, work)
	content, err := os.ReadFile(log)
	if err == nil {
		for _, line := range strings.Split(strings.TrimSuffix(string(content), "\n"), "\n") {
			invocations = append(invocations, strings.Split(line, "\x00"))
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	return invocations, !reflect.DeepEqual(before, after)
}

func filledValue(slot string) string { return "v " + slot + " $(x);'q'" }

func pasteQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'" }

// fill replaces each )slot( with a shell-quoted hostile value, except keep.
func fill(command string, slots []string, keep string) string {
	for _, slot := range slots {
		if slot != keep {
			command = strings.Replace(command, ")"+slot+"(", pasteQuote(filledValue(slot)), 1)
		}
	}
	return command
}

func expectedArgv(featureID string, template guide.Template) []string {
	argv := []string{}
	for _, word := range template.Words {
		switch word.Kind {
		case guide.WordFeature:
			argv = append(argv, featureID)
		case guide.WordHuman:
			argv = append(argv, filledValue(word.Text))
		default:
			argv = append(argv, word.Text)
		}
	}
	return argv
}

func TestGuideTemplatesPasteSafely(t *testing.T) {
	shells := guideShells(t)
	sentinels := sentinelPath(t)
	ids := []string{"a;b", "$(x)", "-f", "it's", "a b", "\u00f1and\u00fa", "`x`", "a\"b\\c", "*"}
	for _, id := range ids {
		reports := []guide.Report{
			guideReport(t, id, "specified", selection.RoleSelected, false),
			guideReport(t, id, "approved", selection.RoleSelected, false),
		}
		for _, report := range reports {
			for _, template := range report.Templates {
				command := RenderGuideCommand(report.FeatureID, template)
				for _, slot := range template.HumanSlots {
					if strings.Count(command, ")"+slot+"(") != 1 {
						t.Fatalf("%q: slot %q not rendered exactly once as )%s(: %s", id, slot, slot, command)
					}
				}
				for shellName, shell := range shells {
					for _, slot := range template.HumanSlots {
						partial := fill(command, template.HumanSlots, slot)
						invocations, changed := paste(t, sentinels, shell, partial)
						if len(invocations) != 0 || changed {
							t.Fatalf("%s %q %s with (%s) unfilled: invocations=%q changed=%v\ncommand: %s", shellName, id, template.Kind, slot, invocations, changed, partial)
						}
					}
					filled := fill(command, template.HumanSlots, "")
					invocations, changed := paste(t, sentinels, shell, filled)
					want := [][]string{expectedArgv(id, template)}
					if !reflect.DeepEqual(invocations, want) || changed {
						t.Fatalf("%s %q %s filled: invocations=%q changed=%v, want %q\ncommand: %s", shellName, id, template.Kind, invocations, changed, want, filled)
					}
				}
			}
		}
	}
}

func TestGuideCommandsCarryNoDerivedValues(t *testing.T) {
	report := guideReport(t, "f", "implemented", selection.RoleSelected, false)
	var text, wire bytes.Buffer
	if err := WriteGuideText(&text, report); err != nil {
		t.Fatal(err)
	}
	if err := WriteGuideJSON(&wire, report); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"draft", "specified", "planned", "tasked", "approved", "abandoned"} {
		for _, output := range []string{text.String(), wire.String()} {
			for _, line := range strings.Split(output, "\n") {
				if strings.Contains(line, "ai-dev workflow") && strings.Contains(line, " "+state) {
					t.Fatalf("template carries derived state %q: %s", state, line)
				}
			}
		}
	}
	if user := os.Getenv("USER"); user != "" && strings.Contains(text.String()+wire.String(), user) {
		t.Fatalf("output carries the operating-system user %q", user)
	}
}

// TestGuideWholeReportPastesSafely pastes the entire human-readable report,
// labels included, into every shell mode. Only the complete read-only
// templates without human slots (select, state) may run; no template with
// an unfilled slot, no agent, no label word and no file change may result.
// The status line itself is covered by TestStatusLinesPasteInert (004 FR-028,
// Amendment 5).
func TestGuideWholeReportPastesSafely(t *testing.T) {
	shells := guideShells(t)
	sentinels := sentinelPath(t)
	reports := []guide.Report{
		guideReport(t, "a;b", "specified", selection.RoleSelected, false),
		guideReport(t, "it's", "tasked", selection.AgentUnavailable, false),
		guideReport(t, "-f", "", selection.SelectionInconsistent, false),
		guideReport(t, "$(x)", "approved", selection.RoleSelected, false),
	}
	for _, report := range reports {
		var text bytes.Buffer
		if err := WriteGuideText(&text, report); err != nil {
			t.Fatal(err)
		}
		var allowed [][]string
		for _, template := range report.Templates {
			if len(template.HumanSlots) == 0 {
				allowed = append(allowed, expectedArgv(report.FeatureID, template))
			}
		}
		for name, shell := range shells {
			invocations, changed := paste(t, sentinels, shell, text.String())
			if changed {
				t.Fatalf("%s %q: pasting the whole report changed files", name, report.FeatureID)
			}
			for _, invocation := range invocations {
				ok := false
				for _, want := range allowed {
					ok = ok || reflect.DeepEqual(invocation, want)
				}
				if !ok {
					t.Fatalf("%s %q: pasting the whole report ran %q; only %q may run\n%s", name, report.FeatureID, invocation, allowed, text.String())
				}
			}
		}
	}
}

// F011 A2: with an invalid project configuration the line and the JSON name
// no agent and carry source=project.
func TestWriteGuideConfigurationInvalid(t *testing.T) {
	r := guideReport(t, "f", "specified", selection.ConfigurationInvalid, false)
	var text, js bytes.Buffer
	if err := WriteGuideText(&text, r); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(text.String(), "ai-dev: outcome=step_recommended feature=\"f\" state=specified activity=plan role=planning availability=configuration_invalid source=project note=") {
		t.Fatalf("text: %q", text.String())
	}
	if strings.Contains(text.String(), "agent=") {
		t.Fatalf("text names an agent: %q", text.String())
	}
	if err := WriteGuideJSON(&js, r); err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(js.Bytes(), &object); err != nil {
		t.Fatal(err)
	}
	if _, ok := object["agent"]; ok || object["agent_availability"] != "configuration_invalid" || object["source"] != "project" {
		t.Fatalf("json: %v", object)
	}
}
