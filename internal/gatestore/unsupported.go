package gatestore

import (
	"ai-dev/internal/gate"
	"ai-dev/internal/workflow"
)

const unsupportedPlatformWarning = "ai-dev: warning: human gate persistence is supported only on macOS and Linux"

type unsupportedStore struct{}

func (unsupportedStore) Commit(string, workflow.State, gate.Evidence) (gate.CommitStatus, []string) {
	return gate.Failed, []string{unsupportedPlatformWarning}
}
