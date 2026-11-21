package app

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/gitmanager"
)

// The text reports of plan, apply and reconcile, over real Git: each block
// names what the operator authorizes, and the approval lines the plan
// prints are exactly the arguments apply accepts.
func TestRealEngineTextReports(t *testing.T) {
	f := newRealEngineFixture(t)
	message := filepath.Join(f.base, "message.txt")
	if err := os.WriteFile(message, []byte("chore: initial commit\n\nBody line.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(f.base, "plan.json")
	code, plan, stderr := f.run("plan", "--project-dir", f.project, "--strategy", "basic", "--initialize-repository",
		"--initial-branch", "main", "--initial-commit", "--initial-commit-message-file", message, "--include", "README.md",
		"--author-name", "Ada", "--author-email", "ada@example.org", "--committer-name", "Grace", "--committer-email", "grace@example.org",
		"--author-date", "2026-11-18T10:00:00Z", "--committer-date", "2026-11-18T11:00:00Z", "--output", planPath)
	if code != 0 || stderr != "" {
		t.Fatalf("plan exit %d %q %q", code, plan, stderr)
	}
	digest := regexp.MustCompile(`(?m)^plan digest:    (sha256:[0-9a-f]{64})$`).FindStringSubmatch(plan)
	if digest == nil {
		t.Fatalf("plan text has no digest line:\n%s", plan)
	}
	for _, want := range []string{
		"plan:           " + planPath + "\n",
		"strategy:       basic\n",
		"toolchain:      git ",
		"initialize:     git init ",
		"commit:         ",
		" (unsigned)\n",
		"initial ref:    refs/heads/main (created only if absent)\n",
		"author:         Ada <ada@example.org> 2026-11-18T10:00:00Z\n",
		"committer:      Grace <grace@example.org> 2026-11-18T11:00:00Z\n",
		"message:        | chore: initial commit\nmessage:        | \nmessage:        | Body line.\n",
		"approve:        --approve-file 'README.md|",
		"screening:      " + gitmanager.SecretScreeningLimitation + "\n",
		"operation 1    ",
	} {
		if !strings.Contains(plan, want) {
			t.Errorf("plan text lacks %q:\n%s", want, plan)
		}
	}
	if !strings.HasSuffix(plan, "authorization:  none; apply needs a separate explicit authorization\n") {
		t.Errorf("plan text does not end by disclaiming authorization:\n%s", plan)
	}
	planFile, err := os.ReadFile(planPath)
	if err != nil || !strings.Contains(string(planFile), digest[1]) {
		t.Fatalf("plan file does not carry the printed digest %s: %v", digest[1], err)
	}

	// The printed approval lines, unquoted, are the apply arguments.
	const id = "123e4567-e89b-42d3-a456-426614174201"
	apply := []string{"apply", "--plan", planPath, "--confirm-plan", digest[1], "--authorization-id", id, "--issue-authorization",
		"--authorized-by", "operator", "--confirm-initial-commit"}
	approvals := regexp.MustCompile(`(?m)^approve:        (--approve-file) '([^']*)'$`).FindAllStringSubmatch(plan, -1)
	// The commit holds the included README.md and the project
	// configuration the plan writes.
	if len(approvals) != 2 || !strings.HasPrefix(approvals[0][2], ".ai-dev/config.json|") || !strings.HasPrefix(approvals[1][2], "README.md|") {
		t.Fatalf("want the config.json and README.md approval lines:\n%s", plan)
	}
	for _, approval := range approvals {
		apply = append(apply, approval[1], approval[2])
	}
	code, applied, stderr := f.run(apply...)
	if code != 0 {
		t.Fatalf("apply exit %d %q %q", code, applied, stderr)
	}
	for _, want := range []string{
		"status:         applied\n",
		"authorization:  " + id + " (applied)\n",
		"plan digest:    " + digest[1] + "\n",
		"completed:      ",
		"effect:         ",
	} {
		if !strings.Contains(applied, want) {
			t.Errorf("apply text lacks %q:\n%s", want, applied)
		}
	}
	if !strings.HasSuffix(applied, "exit:           0\n") || strings.Contains(applied, "recovery:") {
		t.Errorf("apply text:\n%s", applied)
	}

	code, reconciled, stderr := f.run("reconcile", "--plan", planPath, "--authorization-id", id)
	if code != 0 {
		t.Fatalf("reconcile exit %d %q %q", code, reconciled, stderr)
	}
	for _, want := range []string{
		"authorization:  " + id + " (applied, version ",
		"plan digest:    " + digest[1] + "\n",
		"apply attempt:  ",
		"unresolved:     false\n",
		"audit trail:    ",
		"applied:        ",
	} {
		if !strings.Contains(reconciled, want) {
			t.Errorf("reconcile text lacks %q:\n%s", want, reconciled)
		}
	}
	if strings.Contains(reconciled, "continue:") || strings.Contains(reconciled, "crash evidence:") ||
		!strings.HasSuffix(reconciled, "authority:      none; this report grants no mutation authority\n") {
		t.Errorf("reconcile text:\n%s", reconciled)
	}
}

// A refused plan is one "refused:" line; a secret finding adds only the
// path, the category and the screening limitation, never the secret.
func TestRealEnginePlanRejectionText(t *testing.T) {
	f := newRealEngineFixture(t)
	const secret = "s3cr3t-pa55"
	if err := os.WriteFile(filepath.Join(f.project, "notes.txt"), []byte("remote https://ada:"+secret+"@example.org/repo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	message := filepath.Join(f.base, "message.txt")
	if err := os.WriteFile(message, []byte("chore: initial commit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := f.run("plan", "--project-dir", f.project, "--strategy", "basic", "--initialize-repository",
		"--initial-branch", "main", "--initial-commit", "--initial-commit-message-file", message, "--include", "notes.txt",
		"--author-name", "Ada", "--author-email", "ada@example.org", "--committer-name", "Ada", "--committer-email", "ada@example.org",
		"--author-date", "2026-11-18T10:00:00Z", "--committer-date", "2026-11-18T10:00:00Z", "--output", filepath.Join(f.base, "plan.json"))
	output := stdout + stderr
	if code != 1 || !strings.Contains(output, "refused: sensitive_content_suspected: ") ||
		!strings.Contains(output, "finding:        notes.txt url_credentials\n") ||
		!strings.Contains(output, "screening:      "+gitmanager.SecretScreeningLimitation+"\n") || strings.Contains(output, secret) {
		t.Fatalf("exit %d, output:\n%s", code, output)
	}

	code, stdout, stderr = f.run("plan", "--project-dir", f.project, "--strategy", "no-such-strategy", "--output", filepath.Join(f.base, "other.json"))
	output = stdout + stderr
	if code == 0 || !regexp.MustCompile(`(?m)^refused: [a-z_]+: `).MatchString(output) || strings.Contains(output, "finding:") {
		t.Fatalf("exit %d, output:\n%s", code, output)
	}
}
