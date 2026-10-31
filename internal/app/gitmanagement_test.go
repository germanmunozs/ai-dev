package app

import (
	"bytes"
	"context"
	"testing"
	"testing/fstest"
)

func TestGitManagementCompositionIsInertForExistingCommands(t *testing.T) {
	dependencies := unavailableGitManagementDependencies()
	if dependencies.Runner != nil || dependencies.Store != nil || dependencies.Filesystem != nil ||
		dependencies.Clock != nil || dependencies.RandomSource != nil || dependencies.Reporter != nil {
		t.Fatal("unavailable Git Management dependencies must expose no capability")
	}

	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"workflow", "state", "missing"}, &fakeExecutor{}, &appRunLauncher{}, fstest.MapFS{}, bytes.NewReader(nil), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = RunWithGateStore(context.Background(), []string{"workflow", "state", "missing"}, &fakeExecutor{}, &appRunLauncher{}, fstest.MapFS{}, nil, bytes.NewReader(nil), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("RunWithGateStore code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
