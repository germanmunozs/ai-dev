package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const guideStatement = "options for the human only; not authorization. Running an agent or a review is not approval; approval is your own Human Gate decision after completing the activity"

func guideRepoDigest(t *testing.T, dir string) []string {
	t.Helper()
	var digest []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		content := ""
		if entry.Type().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			content = string(data)
		}
		digest = append(digest, path+"|"+info.Mode().String()+"|"+info.ModTime().String()+"|"+content)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

// guideAgentPath returns a PATH directory holding the named fake agents. Each
// fake records any run in marker, which must never appear.
func guideAgentPath(t *testing.T, marker string, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		script := "#!/bin/sh\ntouch '" + marker + "'\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestWorkflowGuideBinaryContract(t *testing.T) {
	bin := buildWorkflowNextBinary(t)
	specified := `{"state":"specified"}`
	marker := filepath.Join(t.TempDir(), "agent-ran")
	cases := []struct {
		name     string
		agents   []string
		wantText string
		wantJSON string
	}{
		{"no agents installed", nil,
			"ai-dev: outcome=step_recommended feature=\"f\" state=specified activity=plan role=planning agent=claude-code availability=unavailable source=built_in note=\"" + guideStatement + "\"\n" +
				"  # select the agent for this role (003)\n  ai-dev agent select -- planning\n" +
				"  # after completing the activity, check the transition (004)\n  ai-dev workflow transition -- 'f' )target-state(\n" +
				"  # then record your decision (006)\n  ai-dev workflow approve --approver )approver( -- 'f' )target-state(\n" +
				"  # replace each )slot( before running; --reason )reason( is optional for approval (006 FR-010)\n",
			`{"schema_version":"ai-dev.workflow-guide/v1","feature_id":"f","outcome":"step_recommended","current_state":"specified","activity":"plan","role":"planning","agent":"claude-code","agent_availability":"unavailable","source":"built_in","templates":[` +
				`{"kind":"select","label":"select the agent for this role (003)","command":"ai-dev agent select -- planning","human_slots":[]},` +
				`{"kind":"transition","label":"after completing the activity, check the transition (004)","command":"ai-dev workflow transition -- 'f' )target-state(","human_slots":["target-state"]},` +
				`{"kind":"approve","label":"then record your decision (006)","command":"ai-dev workflow approve --approver )approver( -- 'f' )target-state(","human_slots":["approver","target-state"],"optional_flags":["--reason"]}],` +
				`"statement":"` + guideStatement + `"}` + "\n"},
		{"mapped agent installed", []string{"claude"},
			"ai-dev: outcome=step_recommended feature=\"f\" state=specified activity=plan role=planning agent=claude-code availability=available source=built_in note=\"" + guideStatement + "\"\n" +
				"  # select the agent for this role (003)\n  ai-dev agent select -- planning\n" +
				"  # run the agent (002)\n  ai-dev agent run -- claude-code )instruction(\n" +
				"  # after completing the activity, check the transition (004)\n  ai-dev workflow transition -- 'f' )target-state(\n" +
				"  # then record your decision (006)\n  ai-dev workflow approve --approver )approver( -- 'f' )target-state(\n" +
				"  # replace each )slot( before running; --reason )reason( is optional for approval (006 FR-010)\n",
			""},
		{"only the other agent installed", []string{"codex"},
			"ai-dev: outcome=step_recommended feature=\"f\" state=specified activity=plan role=planning agent=claude-code availability=unavailable source=built_in note=\"" + guideStatement + "\"\n",
			""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := makeWorkflowNextRepo(t, &specified, "spec.md")
			env := []string{"PATH=" + guideAgentPath(t, marker, tc.agents...), "HOME=" + t.TempDir()}
			before := guideRepoDigest(t, repo)

			cmd := exec.Command(bin, "workflow", "guide", "f")
			cmd.Dir, cmd.Env = repo, env
			stdout, stderr, code := runCommand(cmd)
			if code != 0 || stderr != "" || !strings.HasPrefix(stdout, tc.wantText) {
				t.Fatalf("text code/stdout/stderr = %d/%q/%q, want 0/%q/empty", code, stdout, stderr, tc.wantText)
			}
			if strings.Count(tc.wantText, "\n") > 1 && stdout != tc.wantText {
				t.Fatalf("text stdout = %q, want exactly %q", stdout, tc.wantText)
			}

			cmd = exec.Command(bin, "workflow", "guide", "--format", "json", "f")
			cmd.Dir, cmd.Env = repo, env
			stdout, stderr, code = runCommand(cmd)
			if code != 0 || stderr != "" || (tc.wantJSON != "" && stdout != tc.wantJSON) {
				t.Fatalf("json code/stdout/stderr = %d/%q/%q, want 0/%q/empty", code, stdout, stderr, tc.wantJSON)
			}

			if after := guideRepoDigest(t, repo); !reflect.DeepEqual(before, after) {
				t.Fatalf("repository changed:\nbefore %v\nafter  %v", before, after)
			}
			if _, err := os.Stat(marker); err == nil {
				t.Fatal("guidance ran a coding agent")
			}
		})
	}
}

func TestWorkflowGuideBinaryOutcomesAndUsage(t *testing.T) {
	bin := buildWorkflowNextBinary(t)
	approved := `{"state":"approved"}`
	planned := `{"state":"planned"}`
	cases := []struct {
		name   string
		record *string
		files  []string
		id     string
		code   int
		first  string
	}{
		{"terminal", &approved, []string{"spec.md", "plan.md", "tasks.md"}, "f", 0, "ai-dev: outcome=workflow_terminal feature=\"f\" state=approved note="},
		{"artifacts inconsistent", &planned, []string{"spec.md"}, "f", 1, "ai-dev: outcome=artifacts_inconsistent feature=\"f\" state=planned missing=plan.md note="},
		{"feature unknown", nil, nil, "missing", 1, "ai-dev: outcome=feature_unknown feature=\"missing\" note="},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := makeWorkflowNextRepo(t, tc.record, tc.files...)
			cmd := exec.Command(bin, "workflow", "guide", tc.id)
			cmd.Dir = repo
			stdout, stderr, code := runCommand(cmd)
			if code != tc.code || stderr != "" || !strings.HasPrefix(stdout, tc.first) {
				t.Fatalf("code/stdout/stderr = %d/%q/%q, want %d/%q.../empty", code, stdout, stderr, tc.code, tc.first)
			}
		})
	}

	repo := makeWorkflowNextRepo(t, nil)
	const wantUsage = "Usage: ai-dev workflow guide [--format text|json] <feature-id>\n"
	for _, args := range [][]string{
		{"workflow", "guide"},
		{"workflow", "guide", "   "},
		{"workflow", "guide", "f", "extra"},
		{"workflow", "guide", "--format", "yaml", "f"},
		{"workflow", "guide", "f", "--format", "json"},
	} {
		cmd := exec.Command(bin, args...)
		cmd.Dir = repo
		stdout, stderr, code := runCommand(cmd)
		if code != 2 || stdout != "" || stderr != wantUsage {
			t.Fatalf("args=%v code/stdout/stderr = %d/%q/%q", args, code, stdout, stderr)
		}
	}
}

// TestNoAutonomousCapabilityExists covers US2 and F007-FR-014: no command
// or option can plan, authorize or start an agent chain, and the existing
// usage failures stay unchanged.
func TestNoAutonomousCapabilityExists(t *testing.T) {
	bin := buildWorkflowNextBinary(t)
	specified := `{"state":"specified"}`
	repo := makeWorkflowNextRepo(t, &specified, "spec.md")
	marker := filepath.Join(t.TempDir(), "agent-ran")
	env := []string{"PATH=" + guideAgentPath(t, marker, "claude", "codex"), "HOME=" + t.TempDir()}
	const (
		workflowUsage = "Usage: ai-dev workflow <state|transition|next|guide|approve|abandon> [options]\n"
		topUsage      = "Usage: ai-dev <doctor|agents|agent|workflow|init|git|setup|tui|open|version|help|completion> [options]\n"
		guideUsage    = "Usage: ai-dev workflow guide [--format text|json] <feature-id>\n"
	)
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"workflow", "run", "f"}, workflowUsage},
		{[]string{"workflow", "chain", "f"}, workflowUsage},
		{[]string{"workflow", "orchestrate", "f"}, workflowUsage},
		{[]string{"workflow", "auto", "f"}, workflowUsage},
		{[]string{"orchestrate", "f"}, topUsage},
		{[]string{"guide", "f"}, topUsage},
		{[]string{"workflow", "guide", "--autonomous", "f"}, guideUsage},
		{[]string{"workflow", "guide", "--execute", "f"}, guideUsage},
		{[]string{"workflow", "guide", "--run", "f"}, guideUsage},
		{[]string{"workflow", "guide", "--approve", "f"}, guideUsage},
	}
	before := guideRepoDigest(t, repo)
	for _, tc := range cases {
		cmd := exec.Command(bin, tc.args...)
		cmd.Dir, cmd.Env = repo, env
		stdout, stderr, code := runCommand(cmd)
		if code != 2 || stdout != "" || stderr != tc.want {
			t.Fatalf("args=%v code/stdout/stderr = %d/%q/%q, want 2/empty/%q", tc.args, code, stdout, stderr, tc.want)
		}
	}
	for _, variable := range []string{"AI_DEV_AUTONOMOUS=1", "AI_DEV_MODE=autonomous", "AI_DEV_EXECUTE=1"} {
		cmd := exec.Command(bin, "workflow", "guide", "f")
		cmd.Dir, cmd.Env = repo, append(append([]string{}, env...), variable)
		stdout, stderr, code := runCommand(cmd)
		if code != 0 || stderr != "" || !strings.Contains(stdout, ")instruction(") {
			t.Fatalf("%s: code/stdout/stderr = %d/%q/%q, want the ordinary report", variable, code, stdout, stderr)
		}
	}
	if after := guideRepoDigest(t, repo); !reflect.DeepEqual(before, after) {
		t.Fatalf("repository changed:\nbefore %v\nafter  %v", before, after)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a coding agent ran")
	}
}
