package checks

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/germanmunozs/ai-dev/internal/doctor"
)

func TestGitRepoCheckDelegatesToGit(t *testing.T) {
	exec := &fakeExecutor{stdout: "  true\n"}
	got := NewGitRepoCheck(exec).Run(context.Background())
	if got.Status != doctor.Pass || got.ID != "git-repository" || got.Area != "Repository" {
		t.Fatalf("unexpected result: %#v", got)
	}
	if exec.runName != "git" || !reflect.DeepEqual(exec.runArgs, []string{"rev-parse", "--is-inside-work-tree"}) {
		t.Fatalf("execution = %q %v", exec.runName, exec.runArgs)
	}
}

func TestGitRepoCheckFailuresAreActionableAndContained(t *testing.T) {
	tests := []struct {
		name string
		exec *fakeExecutor
	}{
		{"command not found", &fakeExecutor{runErr: errors.New("not found")}},
		{"nonzero", &fakeExecutor{stdout: "true", runErr: errors.New("exit 128")}},
		{"false", &fakeExecutor{stdout: "false"}},
		{"empty", &fakeExecutor{}},
		{"malformed", &fakeExecutor{stdout: "yes"}},
		{"panic", &fakeExecutor{panicRun: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewGitRepoCheck(tt.exec).Run(context.Background())
			if got.Status != doctor.Fail || got.Explanation == "" || got.Remediation == "" {
				t.Fatalf("unexpected result: %#v", got)
			}
		})
	}
}

func TestGitRepoCheckAcceptsRepositoryWithNoCommits(t *testing.T) {
	exec := &fakeExecutor{stdout: "true"}
	if got := NewGitRepoCheck(exec).Run(context.Background()); got.Status != doctor.Pass {
		t.Fatalf("unexpected result: %#v", got)
	}
}
