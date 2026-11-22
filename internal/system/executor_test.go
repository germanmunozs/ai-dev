package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOSExecutorLookPath(t *testing.T) {
	executor := OSExecutor{}
	path, err := executor.LookPath("go")
	if err != nil {
		t.Fatalf("LookPath(go): %v", err)
	}
	if path == "" {
		t.Fatal("LookPath(go) returned empty path")
	}
}

func TestOSExecutorRunPropagatesStdoutAndErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper is a POSIX shell script")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "helper")
	contents := []byte("#!/bin/sh\nprintf '%s' \"$1\"\nif [ \"$2\" = fail ]; then exit 7; fi\n")
	if err := os.WriteFile(script, contents, 0o755); err != nil {
		t.Fatal(err)
	}

	executor := OSExecutor{}
	out, err := executor.Run(context.Background(), script, "fixed output", "ok")
	if err != nil || out != "fixed output" {
		t.Fatalf("Run success = (%q, %v), want (%q, nil)", out, err, "fixed output")
	}
	out, err = executor.Run(context.Background(), script, "preserved", "fail")
	if out != "preserved" || err == nil {
		t.Fatalf("Run failure = (%q, %v), want preserved stdout and error", out, err)
	}
	var exitErr interface{ ExitCode() int }
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("error = %v, want exit code 7", err)
	}
}

func TestOSExecutorRunDoesNotInvokeShell(t *testing.T) {
	executor := OSExecutor{}
	_, err := executor.Run(context.Background(), "command-that-does-not-exist-"+strings.Repeat("x", 8), "$(echo unsafe)")
	if err == nil {
		t.Fatal("Run unexpectedly succeeded")
	}
}
