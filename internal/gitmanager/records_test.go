package gitmanager

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func fullAuthorization(t *testing.T) GitAuthorization {
	t.Helper()
	in := validAuthorizationInput(t)
	in.SchemaVersion = GitAuthorizationSchema
	in.ExpiresAt = time.Unix(3600, 0).UTC()
	in.InitialCommitAuthorized = true
	ignored := validAuthorizedFile("build/out.txt")
	ignored.Ignored, ignored.ApprovalKind = true, ApprovalKindIgnored
	in.ApprovedFiles = []AuthorizedFile{validAuthorizedFile(ProjectConfigPath), ignored}
	in.PreviousAuthorizationID, in.PreviousApplyID, in.PreviousPlanDigest = "123e4567-e89b-42d3-a456-426614174005", testOtherApplyID, testOtherSHA256
	issued, err := NewGitAuthorization(in)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := issued.Transition(AuthorizationTransitionRequest{ProjectIdentityDigest: issued.ProjectIdentity.Digest(), AuthorizationID: issued.AuthorizationID,
		ExpectedState: AuthorizationStateIssued, ExpectedVersion: 1, To: AuthorizationStateClaimed, Owner: testApplyID})
	if err != nil {
		t.Fatal(err)
	}
	return claimed
}

func TestAuthorizationRecordRoundTrip(t *testing.T) {
	for name, authorization := range map[string]GitAuthorization{
		"minimal": func() GitAuthorization {
			in := validAuthorizationInput(t)
			in.SchemaVersion = GitAuthorizationSchema
			v, err := NewGitAuthorization(in)
			if err != nil {
				t.Fatal(err)
			}
			return v
		}(),
		"full": fullAuthorization(t),
	} {
		data, err := EncodeAuthorizationRecord(authorization)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		decoded, err := DecodeAuthorizationRecord(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		again, _ := EncodeAuthorizationRecord(decoded)
		if !bytes.Equal(again, data) || decoded.OwnerApplyID != authorization.OwnerApplyID || len(decoded.ApprovedFiles()) != len(authorization.ApprovedFiles()) {
			t.Fatalf("%s: round trip changed the record", name)
		}
	}
}

func TestAuthorizationRecordDecodeIsStrict(t *testing.T) {
	data, err := EncodeAuthorizationRecord(fullAuthorization(t))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for name, bad := range map[string]string{
		"unknown member":   strings.Replace(text, `{"approved_files"`, `{"admin":true,"approved_files"`, 1),
		"missing member":   strings.Replace(text, `"declared_operator":"operator",`, ``, 1),
		"wrong schema":     strings.Replace(text, GitAuthorizationSchema, "ai-dev.git-authorization/v2", 1),
		"forged history":   strings.Replace(text, `"state_version":2`, `"state_version":3`, 1),
		"wrong type":       strings.Replace(text, `"initial_commit_authorized":true`, `"initial_commit_authorized":"true"`, 1),
		"non-canonical":    " " + text,
		"tampered project": strings.Replace(text, `"inode_decimal":"2"`, `"inode_decimal":"5"`, 1),
	} {
		if bad == text {
			t.Fatalf("%s: mutation did not apply", name)
		}
		if _, err := DecodeAuthorizationRecord([]byte(bad)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	subSecond := validAuthorizationInput(t)
	subSecond.SchemaVersion = GitAuthorizationSchema
	subSecond.IssuedAt = time.Unix(10, 5).UTC()
	if v, err := NewGitAuthorization(subSecond); err != nil {
		t.Fatal(err)
	} else if _, err := EncodeAuthorizationRecord(v); err == nil {
		t.Fatal("sub-second issue time encoded")
	}
}

func TestAttemptAndEventRecordsCarryNoContent(t *testing.T) {
	attempt := GitApplyAttempt{SchemaVersion: GitApplyAttemptSchema, ApplyID: testApplyID, StartedAt: time.Unix(1, 0).UTC(), InvocationMode: InvocationModeDirect}
	data, err := EncodeAttemptRecord(attempt)
	if err != nil || string(data) != `{"apply_id":"123e4567-e89b-42d3-a456-426614174001","invocation_mode":"direct","schema_version":"ai-dev.git-apply-attempt/v1","started_at":"1970-01-01T00:00:01Z"}` {
		t.Fatalf("attempt record %s %v", data, err)
	}
	f := newKernelFixture(t)
	journal := f.journal(testApplyID)
	if _, err := journal.append(context.Background(), AuditEventMutationIntent, "", map[string]any{"apply_id": "forged"}); err == nil {
		t.Fatal("event detail overrode an identifying member")
	}
}
