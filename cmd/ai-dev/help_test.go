package main

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The built binary answers per-command help and completion (F010-FR-001,
// FR-006), and help leaves standard input unread (F010-FR-004).
func TestCommandHelpAndCompletionBinary(t *testing.T) {
	bin := buildWorkflowNextBinary(t)
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"workflow", "state", "--help"}, "Usage: ai-dev workflow state [--format text|json] <feature-id>\n"},
		{[]string{"help", "init", "plan"}, "--strategy STRATEGY"},
		{[]string{"init", "--help"}, "\n  init reconcile "},
		{[]string{"completion", "bash"}, "complete -o filenames -F _ai_dev ai-dev\n"},
		{[]string{"completion", "zsh"}, "#compdef ai-dev\n"},
	} {
		cmd := exec.Command(bin, test.args...)
		cmd.Stdin = strings.NewReader("")
		stdout, stderr, code := runCommand(cmd)
		if code != 0 || stderr != "" || !strings.Contains(stdout, test.want) {
			t.Fatalf("%v: code=%d stderr=%q stdout=%q", test.args, code, stderr, stdout)
		}
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if _, err := writer.Write([]byte("do not read me\n")); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	cmd := exec.Command(bin, "agent", "run", "--help")
	cmd.Stdin = reader
	stdout, stderr, code := runCommand(cmd)
	if code != 0 || stderr != "" || !strings.HasPrefix(stdout, "ai-dev agent run: ") {
		t.Fatalf("agent run --help: code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	if rest, err := io.ReadAll(reader); err != nil || string(rest) != "do not read me\n" {
		t.Fatalf("help read standard input: left %q, %v", rest, err)
	}
}
