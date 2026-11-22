package report

import (
	"testing"

	"github.com/germanmunozs/ai-dev/internal/gitmanager"
)

func TestConditionText(t *testing.T) {
	for _, test := range []struct {
		condition gitmanager.Condition
		want      string
	}{
		{gitmanager.Condition{Kind: gitmanager.ConditionRepositoryKind, Expected: "normal"}, "repository_kind normal"},
		{gitmanager.Condition{Kind: gitmanager.ConditionHeadSymbolicRef, Subject: gitmanager.EncodeReversiblePath([]byte("refs/heads/trunk-x"))},
			"head_symbolic_ref refs/heads/trunk-x"},
		{gitmanager.Condition{Kind: gitmanager.ConditionConfigValue, Subject: "gitflow.branch.master", Expected: "main"},
			"config_value gitflow.branch.master main"},
		// Bytes that are not UTF-8 keep their reversible encoding.
		{gitmanager.Condition{Kind: gitmanager.ConditionPathAbsent, Subject: gitmanager.EncodeReversiblePath([]byte{0xff})},
			"path_absent " + gitmanager.EncodeReversiblePath([]byte{0xff})},
	} {
		if got := conditionText(test.condition); got != test.want {
			t.Errorf("got %q, want %q", got, test.want)
		}
	}
}
