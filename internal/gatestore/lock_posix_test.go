//go:build (darwin && !ios) || (linux && !android)

package gatestore

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"ai-dev/internal/gate"
	"ai-dev/internal/workflow"
)

func TestGatestoreHelperProcess(t *testing.T) {
	if os.Getenv("GATESTORE_HELPER") == "" {
		return
	}
	args := os.Args
	separator := 0
	for i, arg := range args {
		if arg == "--" {
			separator = i + 1
			break
		}
	}
	if separator == 0 || len(args) <= separator {
		os.Exit(3)
	}
	switch args[separator] {
	case "hold":
		file, err := os.Open(args[separator+1])
		if err != nil {
			os.Exit(4)
		}
		if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			os.Exit(5)
		}
		fmt.Println("ready")
		for {
			time.Sleep(time.Hour)
		}
	case "commit-pause":
		repo, id, step := args[separator+1], args[separator+2], args[separator+4]
		expected := workflow.State(args[separator+3])
		store := New(repo).(*posixStore)
		store.ops.afterStep = func(got string) {
			if got == step {
				fmt.Println("paused:" + got)
				for {
					time.Sleep(time.Hour)
				}
			}
		}
		entry := testEvidence(expected, workflow.Specified, "", false)
		if expected == workflow.Specified {
			entry = testEvidence(expected, workflow.Planned, "", false)
		}
		store.Commit(id, expected, entry)
		os.Exit(0)
	default:
		os.Exit(6)
	}
}

func startStoreHelper(t *testing.T, args ...string) (*exec.Cmd, *bufio.Reader) {
	t.Helper()
	commandArgs := append([]string{"-test.run=^TestGatestoreHelperProcess$", "--"}, args...)
	cmd := exec.Command(os.Args[0], commandArgs...)
	cmd.Env = append(os.Environ(), "GATESTORE_HELPER=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	return cmd, bufio.NewReader(stdout)
}

func TestGatestoreLockActiveAndKilledHolder(t *testing.T) {
	repo := makeStoreRepo(t, `{"state":"draft"}`)
	dir := filepath.Join(repo, "specs", "f")
	file, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(dir, workflow.RecordFileName)
	before, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	status, _ := New(repo).Commit("f", workflow.Draft, testEvidence(workflow.Draft, workflow.Specified, "", false))
	if status != gate.Conflict {
		t.Fatalf("same-process status=%v", status)
	}
	after, err := os.ReadFile(recordPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("same-process record changed: %q err=%v", after, err)
	}
	assertNoStoreTmp(t, repo)
	_ = file.Close()
	cmd, reader := startStoreHelper(t, "hold", dir)
	line, err := reader.ReadString('\n')
	if err != nil || line != "ready\n" {
		t.Fatalf("ready=%q err=%v", line, err)
	}
	status, _ = New(repo).Commit("f", workflow.Draft, testEvidence(workflow.Draft, workflow.Specified, "", false))
	if status != gate.Conflict {
		t.Fatalf("cross-process status=%v", status)
	}
	after, err = os.ReadFile(recordPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("cross-process record changed: %q err=%v", after, err)
	}
	assertNoStoreTmp(t, repo)
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("SIGKILLed helper exited successfully")
	}
	status, _ = New(repo).Commit("f", workflow.Draft, testEvidence(workflow.Draft, workflow.Specified, "", false))
	if status != gate.Committed {
		t.Fatalf("after kill status=%v", status)
	}
}

func TestGatestoreAmbiguousLockFailsClosedOnce(t *testing.T) {
	for _, injected := range []error{syscall.ENOLCK, syscall.EOPNOTSUPP, syscall.EINVAL, syscall.EINTR} {
		t.Run(injected.Error(), func(t *testing.T) {
			repo := makeStoreRepo(t, `{"state":"draft"}`)
			store := New(repo).(*posixStore)
			calls := 0
			store.ops.flock = func(*os.File) error { calls++; return injected }
			entriesBefore, err := os.ReadDir(filepath.Join(repo, "specs", "f"))
			if err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(filepath.Join(repo, "specs", "f", workflow.RecordFileName))
			status, _ := store.Commit("f", workflow.Draft, testEvidence(workflow.Draft, workflow.Specified, "", false))
			after, _ := os.ReadFile(filepath.Join(repo, "specs", "f", workflow.RecordFileName))
			entriesAfter, err := os.ReadDir(filepath.Join(repo, "specs", "f"))
			if err != nil {
				t.Fatal(err)
			}
			if status != gate.Failed || calls != 1 || string(before) != string(after) || dirEntryNames(entriesBefore) != dirEntryNames(entriesAfter) {
				t.Fatalf("status=%v calls=%d", status, calls)
			}
			assertNoStoreTmp(t, repo)
		})
	}
}

func TestGatestoreDirectoryBindingConflict(t *testing.T) {
	repo := makeStoreRepo(t, `{"state":"draft"}`)
	other := filepath.Join(repo, "specs", "other")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	otherInfo, err := os.Lstat(other)
	if err != nil {
		t.Fatal(err)
	}
	store := New(repo).(*posixStore)
	store.ops.lstat = func(*os.Root, string) (os.FileInfo, error) { return otherInfo, nil }
	record := filepath.Join(repo, "specs", "f", workflow.RecordFileName)
	before, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	status, _ := store.Commit("f", workflow.Draft, testEvidence(workflow.Draft, workflow.Specified, "", false))
	if status != gate.Conflict {
		t.Fatalf("status=%v", status)
	}
	if _, err := os.Stat(filepath.Join(repo, "specs", "f", workflow.RecordFileName)); errors.Is(err, os.ErrNotExist) {
		t.Fatal("record disappeared")
	}
	after, err := os.ReadFile(record)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("record changed: %q err=%v", after, err)
	}
	assertNoStoreTmp(t, repo)
}

func TestGatestoreDirectorySymlinkSwapConflicts(t *testing.T) {
	repo := makeStoreRepo(t, `{"state":"draft"}`)
	dir := filepath.Join(repo, "specs", "f")
	moved := filepath.Join(repo, "specs", "f-moved")
	copyDir := filepath.Join(repo, "specs", "copy")
	if err := os.Mkdir(copyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(copyDir, workflow.RecordFileName), []byte(`{"state":"draft"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := New(repo).(*posixStore)
	store.ops.afterStep = func(step string) {
		if step != "locked" {
			return
		}
		if err := os.Rename(dir, moved); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("copy", dir); err != nil {
			t.Fatal(err)
		}
	}
	status, _ := store.Commit("f", workflow.Draft, testEvidence(workflow.Draft, workflow.Specified, "", false))
	if status != gate.Conflict {
		t.Fatalf("status=%v", status)
	}
	for _, record := range []string{filepath.Join(moved, workflow.RecordFileName), filepath.Join(copyDir, workflow.RecordFileName)} {
		data, err := os.ReadFile(record)
		if err != nil || string(data) != `{"state":"draft"}` {
			t.Fatalf("record %s changed: %q err=%v", record, data, err)
		}
	}
	assertNoStoreTmpAt(t, moved)
	assertNoStoreTmpAt(t, copyDir)
}

func dirEntryNames(entries []os.DirEntry) string {
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	return fmt.Sprint(names)
}

func assertNoStoreTmpAt(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("tmp=%v", matches)
	}
}
