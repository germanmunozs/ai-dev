//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

var commitTime = time.Date(2026, 11, 18, 10, 30, 0, 0, time.FixedZone("", -5*3600))

func commitRequest(includes ...string) *CommitRequest {
	identity := GitIdentity{Name: "Ada Lovelace", Email: "ada@example.org"}
	return &CommitRequest{MessageBytes: []byte("chore: initial commit\n"), Includes: includes,
		Author: identity, Committer: identity, AuthorDate: commitTime, CommitterDate: commitTime}
}

// commitPlan writes files into a new project and plans the basic initial
// commit of includes.
func (f *planFixture) commitPlan(files map[string]string, commit *CommitRequest) (PlanResult, *PlanRejection) {
	for name, content := range files {
		mode := os.FileMode(0o644)
		if strings.HasSuffix(name, ".sh") {
			mode = 0o755
		}
		mustMkdir(f.t, filepath.Dir(filepath.Join(f.root, name)))
		if err := os.WriteFile(filepath.Join(f.root, name), []byte(content), mode); err != nil {
			f.t.Fatal(err)
		}
	}
	f.request.Candidates = CommitCandidates(commit.Includes)
	req := basicNewRepo()
	req.InitialCommit = commit
	return f.plan(req)
}

func mustCommitPlan(t *testing.T, f *planFixture, files map[string]string, commit *CommitRequest) GitInitializationPlan {
	t.Helper()
	result, rejection := f.commitPlan(files, commit)
	if rejection != nil {
		t.Fatalf("plan refused: %+v", rejection)
	}
	return result.Plan
}

// T078/AC-003: each literal file is frozen with its path, Git mode, SHA-256,
// blob OID and ignored status; .ai-dev/config.json is always included with
// mode 100644 (F008-FR-082–083, 161).
func TestInitialCommitAllowlist(t *testing.T) {
	f := newPlanFixture(t)
	plan := mustCommitPlan(t, f, map[string]string{".gitignore": "*.log\n", "README.md": "# p\n", "bin/run.sh": "#!/bin/sh\n", "trace.log": "x\n"},
		commitRequest("README.md", "bin/run.sh", "trace.log", ".gitignore"))
	files := map[string]AuthorizedFile{}
	for _, file := range plan.ImmutableIntent.AuthorizedFiles() {
		files[file.PathUTF8] = file
	}
	if len(files) != 5 {
		t.Fatalf("files %v", files)
	}
	readme := files["README.md"]
	if readme.Mode != "100644" || readme.ContentSHA256 != contentDigest([]byte("# p\n")) || readme.BlobOID != blobOID(ObjectFormatSHA1, []byte("# p\n")) ||
		readme.Ignored || readme.ApprovalKind != ApprovalKindNormal {
		t.Fatalf("README %+v", readme)
	}
	if files["bin/run.sh"].Mode != "100755" {
		t.Fatalf("executable mode %+v", files["bin/run.sh"])
	}
	if log := files["trace.log"]; !log.Ignored || log.ApprovalKind != ApprovalKindIgnored {
		t.Fatalf("ignored file %+v", log)
	}
	config := files[ProjectConfigPath]
	if config.Mode != "100644" || config.ContentSHA256 != plan.ImmutableIntent.ProjectConfigBytesDigest || !placeholderIdentity(config) {
		t.Fatalf("project configuration %+v", config)
	}
	var kinds []OperationKind
	for _, operation := range plan.DerivedOperations() {
		kinds = append(kinds, operation.Kind)
	}
	want := []OperationKind{OperationKindRepositoryInit, OperationKindCreateProjectConfigDirectory, OperationKindVerification,
		OperationKindProjectConfigWrite, OperationKindInitialCommit, OperationKindInitialRefCAS, OperationKindPreparedIndex, OperationKindVerification}
	if !equalLists(kinds, want) {
		t.Fatalf("operations %v", kinds)
	}
	// Approval: the exact set, ignored files only in the ignored form.
	tuples := map[string]string{}
	for _, file := range plan.ImmutableIntent.AuthorizedFiles() {
		tuples[file.PathUTF8] = file.PathUTF8 + "|" + file.Mode + "|" + string(file.ContentSHA256) + "|blob:" + string(file.BlobOID)
	}
	issue := func(normal, ignored []string) *PlanRejection {
		_, rejection := newAuthorization(IssueRequest{AuthorizationID: issuedID, AuthorizedBy: "op", ConfirmInitialCommit: true,
			Approvals: normal, IgnoredApprovals: ignored}, plan, time.Unix(1000, 0))
		return rejection
	}
	all := []string{tuples["README.md"], tuples["bin/run.sh"], tuples[ProjectConfigPath], tuples[".gitignore"]}
	if rejection := issue(all, []string{tuples["trace.log"]}); rejection != nil {
		t.Fatalf("exact approvals refused: %+v", rejection)
	}
	for name, test := range map[string]struct {
		normal, ignored []string
		reason          Reason
	}{
		"ignored file as normal": {append(all, tuples["trace.log"]), nil, ReasonAuthorizationMismatch},
		"config missing":         {all[:2], []string{tuples["trace.log"]}, ReasonAuthorizationInputIncomplete},
		"extra file":             {append(all, "other|100644|"+testSHA256+"|blob:"+testCommitOID), []string{tuples["trace.log"]}, ReasonAuthorizationMismatch},
		"duplicate":              {append(all, all[0]), []string{tuples["trace.log"]}, ReasonAuthorizationMismatch},
	} {
		if rejection := issue(test.normal, test.ignored); rejection == nil || rejection.Reason != test.reason {
			t.Errorf("%s: %+v", name, rejection)
		}
	}
}

// T078/AC-012: only literal regular files beneath the root are admissible
// (F008-FR-086–089); every refusal happens at planning.
func TestInitialCommitRefusesNonLiteralPaths(t *testing.T) {
	for name, test := range map[string]struct {
		include string
		setup   func(root string)
		reason  Reason
	}{
		"traversal":         {include: "../outside", reason: ReasonPathUnrepresentable},
		"absolute":          {include: "/etc/hosts", reason: ReasonPathUnrepresentable},
		"glob":              {include: "*.md", reason: ReasonPathUnrepresentable},
		"directory slash":   {include: "docs/", reason: ReasonPathUnrepresentable},
		"git directory":     {include: ".GIT/config", reason: ReasonPathUnrepresentable},
		"pipe":              {include: "a|b", reason: ReasonPathUnrepresentable},
		"control":           {include: "a\x01b", reason: ReasonPathUnrepresentable},
		"dot component":     {include: "./README.md", reason: ReasonPathUnrepresentable},
		"directory":         {include: "docs", setup: func(root string) { mustMkdir(t, filepath.Join(root, "docs")) }, reason: ReasonRepositoryStateIncompatible},
		"missing":           {include: "missing.txt", reason: ReasonRepositoryStateIncompatible},
		"final symlink":     {include: "link.txt", setup: func(root string) { os.Symlink("README.md", filepath.Join(root, "link.txt")) }, reason: ReasonRepositoryStateIncompatible},
		"symlink component": {include: "alias/README.md", setup: func(root string) { os.Symlink(".", filepath.Join(root, "alias")) }, reason: ReasonPathUnrepresentable},
		"fifo": {include: "pipe", setup: func(root string) {
			if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, reason: ReasonRepositoryStateIncompatible},
		"file over the bound": {include: "big.bin", setup: func(root string) {
			file, err := os.Create(filepath.Join(root, "big.bin"))
			if err != nil {
				t.Fatal(err)
			}
			file.Truncate(LimitAuthorizedFileBytes.Max() + 1)
			file.Close()
		}, reason: ReasonRepositoryStateIncompatible},
	} {
		t.Run(name, func(t *testing.T) {
			f := newPlanFixture(t)
			mustWrite(t, filepath.Join(f.root, "README.md"), "# p\n")
			if test.setup != nil {
				test.setup(f.root)
			}
			_, rejection := f.commitPlan(nil, commitRequest(test.include))
			if rejection == nil || rejection.Reason != test.reason {
				t.Fatalf("got %+v, want %s", rejection, test.reason)
			}
			if _, err := os.Lstat(filepath.Join(f.root, ".git")); !os.IsNotExist(err) {
				t.Fatal("planning created .git")
			}
		})
	}
}

// T078/AC-012: on macOS a new repository folds case and precomposes
// Unicode, so case-colliding and non-ASCII paths are refused (F008-FR-088,
// 183).
func TestInitialCommitPathEquivalence(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("case folding and Unicode precomposition are assumed only for new repositories on macOS")
	}
	f := newPlanFixture(t)
	_, rejection := f.commitPlan(map[string]string{"README.md": "x\n"}, commitRequest("README.md", "readme.MD"))
	expectRejection(t, rejection, ReasonPathCollision, 1)
	f = newPlanFixture(t)
	_, rejection = f.commitPlan(map[string]string{"café.txt": "x\n"}, commitRequest("café.txt"))
	expectRejection(t, rejection, ReasonPathNormalizationIncompatible, 1)
}

// T078/AC-003: a file ignored only by the user's global excludes file is
// still ignored and needs the reinforced approval (F008-FR-090, 160).
func TestInitialCommitGloballyIgnoredFile(t *testing.T) {
	f := newPlanFixture(t)
	mustWrite(t, filepath.Join(f.home, ".config", "git", "ignore"), "local.env\n")
	plan := mustCommitPlan(t, f, map[string]string{"local.env": "MODE=dev\n"}, commitRequest("local.env"))
	for _, file := range plan.ImmutableIntent.AuthorizedFiles() {
		if file.PathUTF8 == "local.env" && (!file.Ignored || file.ApprovalKind != ApprovalKindIgnored) {
			t.Fatalf("globally ignored file %+v", file)
		}
	}
}

// T078/AC-008: a file that changes between inspection and planning is
// refused rather than frozen with bytes nobody inspected.
func TestInitialCommitFileChangedAfterInspection(t *testing.T) {
	f := newPlanFixture(t)
	mustWrite(t, filepath.Join(f.root, "README.md"), "# p\n")
	f.request.Candidates = CommitCandidates([]string{"README.md"})
	inspection := f.inspect()
	mustWrite(t, filepath.Join(f.root, "README.md"), "# changed\n")
	req := basicNewRepo()
	req.InitialCommit = commitRequest("README.md")
	_, rejection := Plan(t.Context(), f.deps, req, inspection, validEnvironmentProfile(), PlanMetadata{PlanID: "plan-1", GeneratedAt: time.Unix(200, 0).UTC()})
	expectRejection(t, rejection, ReasonRepositoryStateIncompatible, 1)
}
