//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"path/filepath"
	"strings"
	"testing"
)

// syntheticSecrets builds each high-risk format at run time, so this file
// holds no literal credential (F008-FR-091).
func syntheticSecrets() map[string]string {
	alnum := func(n int) string { return strings.Repeat("A1b2", n/4+1)[:n] }
	return map[string]string{
		"private_key":           "-----BEGIN " + "RSA PRIVATE KEY-----\nMIIE\n",
		"aws_access_key_id":     "key = AKIA" + strings.Repeat("Z", 16) + "\n",
		"aws_secret_access_key": "aws_secret_access_key = " + alnum(40) + "\n",
		"github_token":          "token: ghp_" + alnum(36) + "\n",
		"gitlab_token":          "glpat-" + alnum(20) + "\n",
		"slack_token":           "xoxb-" + alnum(24) + "\n",
		"google_api_key":        "AIza" + alnum(35) + "\n",
		"stripe_secret_key":     "sk_live_" + alnum(24) + "\n",
		"anthropic_api_key":     "sk-ant-" + alnum(40) + "\n",
		"npm_token":             "npm_" + alnum(36) + "\n",
		"url_credentials":       "remote = https://user:" + "pa55word@example.org/repo.git\n",
	}
}

// T079: every known high-risk format is found and reported by category.
func TestSecretScreeningCorpus(t *testing.T) {
	for category, content := range syntheticSecrets() {
		if got := screenSecrets([]byte(content)); !containsValue(got, category) {
			t.Errorf("%s: found %v", category, got)
		}
	}
	for _, benign := range []string{"# readme\n", "AKIA is a prefix\n", "https://example.org/repo.git\n", "BEGIN PUBLIC KEY\n"} {
		if got := screenSecrets([]byte(benign)); len(got) != 0 {
			t.Errorf("%q: false positive %v", benign, got)
		}
	}
	if SecretScreeningLimitation == "" {
		t.Fatal("the scanner limitation is not declared (F008-FR-093)")
	}
}

// T079/AC-008/AC-028: a secret in the last allowlisted file refuses the
// group; the finding carries only the path and category, never the bytes.
func TestSecretInLastAllowlistedFile(t *testing.T) {
	f := newPlanFixture(t)
	secret := syntheticSecrets()["github_token"]
	_, rejection := f.commitPlan(map[string]string{"a.txt": "a\n", "b.txt": "b\n", "z.txt": secret}, commitRequest("a.txt", "b.txt", "z.txt"))
	if rejection == nil || rejection.Reason != ReasonSensitiveContentSuspected {
		t.Fatalf("rejection %+v", rejection)
	}
	if len(rejection.Findings) != 1 || string(rejection.Findings[0].Path.Bytes()) != "z.txt" || rejection.Findings[0].Category != "github_token" {
		t.Fatalf("findings %+v", rejection.Findings)
	}
	if strings.Contains(rejection.Error(), "ghp_") {
		t.Fatal("the rejection exposes the secret")
	}
}

// T079/T085: Phase 1 rereads every approved file before anything is
// claimed; a change is plan_stale, and the barrier before the first blob
// reports the same drift as concurrent_change (F008-FR-191).
func TestPreflightPhases(t *testing.T) {
	f := newPlanFixture(t)
	plan := mustCommitPlan(t, f, map[string]string{"a.txt": "a\n"}, commitRequest("a.txt"))
	approved, rejection := approvedBytes(plan, false)
	if rejection != nil || string(approved["a.txt"]) != "a\n" {
		t.Fatalf("phase 1 %v %+v", approved, rejection)
	}
	if config := approved[ProjectConfigPath]; contentDigest(config) != plan.ImmutableIntent.ProjectConfigBytesDigest {
		t.Fatal("phase 1 does not use the in-memory project configuration")
	}
	// Phase 2 needs the configuration on disk, written by this attempt.
	if _, rejection := approvedBytes(plan, true); rejection == nil || rejection.Reason != ReasonConcurrentChange {
		t.Fatalf("barrier without the configuration: %+v", rejection)
	}
	mustWrite(t, filepath.Join(f.root, "a.txt"), "changed\n")
	if _, rejection := approvedBytes(plan, false); rejection == nil || rejection.Reason != ReasonPlanStale {
		t.Fatalf("phase 1 after a change: %+v", rejection)
	}
}
