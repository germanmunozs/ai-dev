//go:build (darwin && !ios) || (linux && !android)

package gitmanager_test

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/germanmunozs/ai-dev/internal/gitmanager"
	"github.com/germanmunozs/ai-dev/internal/gitprocess"
)

// T131/AC-011/AC-023: the git-flow-next dialect against real Git, for a new
// project with and without an initial commit. Git ends with exactly the
// gitflow keys the plan writes, in plan order and with the planned values;
// the development branch is created at the initial commit or, without
// one, reported pending; no hooks key is written.
func TestGitFlowNextWithRealGit(t *testing.T) {
	for _, withCommit := range []bool{true, false} {
		name := "without initial commit"
		if withCommit {
			name = "with initial commit"
		}
		t.Run(name, func(t *testing.T) {
			g := newRealGit(t)
			if err := os.WriteFile(filepath.Join(g.project, "README.md"), []byte("# next\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			var candidates [][]byte
			var commit *gitmanager.CommitRequest
			if withCommit {
				at := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
				identity := gitmanager.GitIdentity{Name: "Grace Hopper", Email: "grace@example.org"}
				candidates = gitmanager.CommitCandidates([]string{"README.md"})
				commit = &gitmanager.CommitRequest{MessageBytes: []byte("feat: start the project\n"), Includes: []string{"README.md"},
					Author: identity, Committer: identity, AuthorDate: at, CommitterDate: at}
			}
			// As the CLI does, candidates are inspected only for a commit group.
			inspection, err := g.inspectCandidates(context.Background(), candidates)
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range inspection.Incompatibilities() {
				if item.Reason == gitmanager.ReasonFilesystemUnsupported {
					t.Skip("the temporary directory is not on a production filesystem")
				}
			}
			planner := gitmanager.PlannerDependencies{Git: g.reader, Commands: gitprocess.Builder{}, Random: rand.Reader}
			planned, rejection := gitmanager.Plan(context.Background(), planner, gitmanager.StrategyRequest{
				Strategy: "git-flow", InitializeRepository: true, GitFlowDialect: "git-flow-next",
				Branches: map[string]string{"production-branch": "main", "development-branch": "dev"},
				Prefixes: map[string]string{"feature": "feature/", "bugfix": "bugfix/", "release": "release/", "hotfix": "hotfix/",
					"support": "support/", "version_tag": "v"},
				InitialCommit: commit,
			}, inspection, g.environment, gitmanager.PlanMetadata{PlanID: "plan-next", GeneratedAt: time.Unix(200, 0).UTC()})
			if rejection != nil {
				t.Fatalf("planning refused: %s: %s", rejection.Reason, rejection.Detail)
			}
			var want []string
			for _, operation := range planned.Plan.DerivedOperations() {
				if operation.Kind == gitmanager.OperationKindGitConfigWrite {
					argv := operation.ExactArgv()
					key := argv[len(argv)-2]
					dot := strings.LastIndex(key, ".")
					want = append(want, key[:dot]+"."+strings.ToLower(key[dot+1:])+"="+argv[len(argv)-1])
				}
			}
			if len(want) == 0 {
				t.Fatal("the plan writes no gitflow key")
			}
			p := &commitProject{realGit: g, planned: planned}
			if p.file, err = gitmanager.EncodePlanFile(planned.Plan); err != nil {
				t.Fatal(err)
			}
			// Issuance confirms an initial commit only when the plan has one.
			var result gitmanager.GitApplyResult
			if withCommit {
				result = p.apply(t, p.store(t), "123e4567-e89b-42d3-a456-426614174131")
			} else {
				result = g.apply(p.store(t), p.file, planned.Plan.PlanDigest, "123e4567-e89b-42d3-a456-426614174131")
			}
			if result.Status != gitmanager.ApplyStatusApplied || result.ExitCode != 0 {
				t.Fatalf("result %s/%s exit %d", result.Status, result.Reason, result.ExitCode)
			}
			listed := p.configList(t)
			var got []string
			for _, line := range listed {
				if strings.HasPrefix(line, "gitflow.") {
					got = append(got, line)
				}
				if strings.Contains(strings.ToLower(line), "hooks") {
					t.Fatalf("a hooks key was written: %q", line)
				}
			}
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Fatalf("gitflow keys:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
			gitDir := filepath.Join(g.project, ".git")
			_, devErr := os.Stat(filepath.Join(gitDir, "refs", "heads", "dev"))
			if withCommit {
				main, _ := os.ReadFile(filepath.Join(gitDir, "refs", "heads", "main"))
				dev, _ := os.ReadFile(filepath.Join(gitDir, "refs", "heads", "dev"))
				if !result.DevelopmentBranchCreated || string(main) != string(result.CommitOID)+"\n" || string(dev) != string(main) {
					t.Fatalf("main %q dev %q commit %s", main, dev, result.CommitOID)
				}
			} else if result.DevelopmentBranchCreated || result.Reason != gitmanager.ReasonDevelopmentBranchPendingInitialHistory || devErr == nil {
				t.Fatalf("without history: created %v reason %s", result.DevelopmentBranchCreated, result.Reason)
			}
		})
	}
}

// configList lists the repository's local configuration file as
// lowercase section.subsection.key=value lines in file order.
func (p *commitProject) configList(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(p.project, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	section := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name, sub, _ := strings.Cut(strings.Trim(line, "[]"), " ")
			section = strings.ToLower(name)
			if sub = strings.Trim(sub, "\""); sub != "" {
				section += "." + sub
			}
			continue
		}
		if key, value, found := strings.Cut(line, " = "); found {
			lines = append(lines, section+"."+strings.ToLower(key)+"="+value)
		}
	}
	return lines
}
