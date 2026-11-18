//go:build (darwin && !ios) || (linux && !android)

package gitmanager

import (
	"strings"
	"testing"
	"time"
)

// T080/AC-016/AC-027: the message must be the canonical UTF-8 bytes of a
// Conventional Commit ending in exactly one LF (F008-FR-102–107).
func TestInitialCommitMessage(t *testing.T) {
	for message, valid := range map[string]bool{
		"feat: add\n":                            true,
		"FEAT(Core)!: shout\n":                   true,
		"fix: x\n\nRefs #123\n":                  true,
		"feat: add":                              false,
		"feat: add\n\n":                          false,
		"feat: add\r\n":                          false,
		"feat: \xff\n":                           false,
		"not conventional\n":                     false,
		"feat: x\x00\n":                          false,
		"feat: x\n" + strings.Repeat("a", 1<<20): false,
	} {
		f := newPlanFixture(t)
		commit := commitRequest()
		commit.MessageBytes = []byte(message)
		_, rejection := f.commitPlan(nil, commit)
		if valid != (rejection == nil) || (!valid && rejection.Reason != ReasonCommitMessageInvalid) {
			t.Errorf("%q: %+v", message, rejection)
		}
	}
}

// T080/AC-027: names and emails are explicit, safe and stored exactly;
// dates are explicit whole seconds (F008-FR-096–100).
func TestInitialCommitIdentity(t *testing.T) {
	for name, mutate := range map[string]func(*CommitRequest){
		"empty name":          func(c *CommitRequest) { c.Author.Name = "" },
		"angle bracket":       func(c *CommitRequest) { c.Author.Name = "Ada <x>" },
		"newline":             func(c *CommitRequest) { c.Committer.Name = "Ada\nLovelace" },
		"trailing dot":        func(c *CommitRequest) { c.Author.Name = "Ada L." },
		"leading space":       func(c *CommitRequest) { c.Committer.Name = " Ada" },
		"email without at":    func(c *CommitRequest) { c.Author.Email = "ada.example.org" },
		"email with space":    func(c *CommitRequest) { c.Author.Email = "ada @example.org" },
		"email with two at":   func(c *CommitRequest) { c.Committer.Email = "a@b@c" },
		"missing date":        func(c *CommitRequest) { c.AuthorDate = time.Time{} },
		"fractional date":     func(c *CommitRequest) { c.CommitterDate = commitTime.Add(time.Millisecond) },
		"control in email":    func(c *CommitRequest) { c.Author.Email = "ada\t@example.org" },
		"quote crud in email": func(c *CommitRequest) { c.Committer.Email = "ada@example.org'" },
	} {
		f := newPlanFixture(t)
		commit := commitRequest()
		mutate(commit)
		_, rejection := f.commitPlan(nil, commit)
		if rejection == nil || rejection.Reason != ReasonIdentityInvalid {
			t.Errorf("%s: %+v", name, rejection)
		}
	}
}

// T080: objects are encoded exactly as Git encodes them, so the plan can
// freeze the OIDs Git must return.
func TestObjectEncoding(t *testing.T) {
	if got := objectOID(ObjectFormatSHA1, "tree", nil); got != "4b825dc642cb6eb9a060e54bf8d69288fbee4904" {
		t.Fatalf("empty tree %s", got)
	}
	if got := blobOID(ObjectFormatSHA1, []byte("hello\n")); got != "ce013625030ba8dba906f756967f9e9ca394464a" {
		t.Fatalf("blob %s", got)
	}
	files := []AuthorizedFile{
		{PathUTF8: "a.txt", Mode: "100644", BlobOID: blobOID(ObjectFormatSHA1, []byte("a"))},
		{PathUTF8: "a/b.txt", Mode: "100755", BlobOID: blobOID(ObjectFormatSHA1, []byte("b"))},
		{PathUTF8: "a-b", Mode: "100644", BlobOID: blobOID(ObjectFormatSHA1, []byte("c"))},
	}
	trees, err := commitTrees(ObjectFormatSHA1, files)
	if err != nil || len(trees) != 2 {
		t.Fatalf("trees %v %v", trees, err)
	}
	// Git orders "a-b" < "a.txt" < "a/" (the subtree sorts as "a/").
	root := string(trees[1].content)
	if !(strings.Index(root, "a-b\x00") < strings.Index(root, "a.txt\x00") && strings.Index(root, "a.txt\x00") < strings.Index(root, "40000 a\x00")) {
		t.Fatalf("root tree order %q", root)
	}
	if _, err := commitTrees(ObjectFormatSHA1, append(files, AuthorizedFile{PathUTF8: "a.txt/x", Mode: "100644", BlobOID: files[0].BlobOID})); err == nil {
		t.Fatal("a path that is both file and directory was encoded")
	}
	f := newPlanFixture(t)
	plan := mustCommitPlan(t, f, map[string]string{"a.txt": "a\n"}, commitRequest("a.txt"))
	commit := commitObject(ObjectFormatSHA1, "4b825dc642cb6eb9a060e54bf8d69288fbee4904", plan.ImmutableIntent)
	want := "tree 4b825dc642cb6eb9a060e54bf8d69288fbee4904\nauthor Ada Lovelace <ada@example.org> 1795015800 -0500\n" +
		"committer Ada Lovelace <ada@example.org> 1795015800 -0500\n\nchore: initial commit\n"
	if string(commit.content) != want || strings.Contains(string(commit.content), "gpgsig") {
		t.Fatalf("commit %q", commit.content)
	}
}

// T080/AC-029: an initial commit needs an unborn HEAD without history and
// an empty index (F008-FR-112).
func TestInitialCommitRepositoryState(t *testing.T) {
	f := newPlanFixture(t)
	f.existingRepository()
	f.request.Candidates = CommitCandidates(nil)
	req := StrategyRequest{Strategy: "basic", Branches: map[string]string{}, Prefixes: map[string]string{}, InitialCommit: commitRequest()}
	_, rejection := f.plan(req)
	expectRejection(t, rejection, ReasonRepositoryStateIncompatible, 1)
}
