//go:build (darwin && !ios) || (linux && !android)

package gitstore

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"

	"ai-dev/internal/gitmanager"
)

// authorizationLocation names the record of one authorization inside its
// project partition, without creating anything.
func authorizationLocation(project gitmanager.ProjectIdentityDigest, id gitmanager.AuthorizationID) (string, string, error) {
	dir, err := projectPath(project)
	if err != nil {
		return "", "", err
	}
	if err := id.Validate(); err != nil {
		return "", "", err
	}
	return path.Join(dir, "authorizations"), string(id) + ".json", nil
}

// IssueAuthorization exclusively publishes a fresh issued authorization.
// An existing record with the same ID is never overwritten, merged or
// widened.
func (s *Store) IssueAuthorization(_ context.Context, authorization gitmanager.GitAuthorization) error {
	if authorization.State != gitmanager.AuthorizationStateIssued || authorization.StateVersion != 1 ||
		authorization.OwnerApplyID != "" || len(authorization.TransitionHistory()) != 0 {
		return fmt.Errorf("issue authorization: only a fresh issued authorization can be issued")
	}
	data, err := gitmanager.EncodeAuthorizationRecord(authorization)
	if err != nil {
		return err
	}
	if _, err := s.projectDirectory(authorization.ProjectIdentity.Digest()); err != nil {
		return err
	}
	dir, name, err := authorizationLocation(authorization.ProjectIdentity.Digest(), authorization.AuthorizationID)
	if err != nil {
		return err
	}
	if _, err := s.records.publish(dir, name, data); err != nil {
		if errors.Is(err, ErrRecordExists) {
			return fmt.Errorf("%w: %v", gitmanager.ErrAuthorizationExists, err)
		}
		return err
	}
	return nil
}

// LoadAuthorization rereads and strictly decodes one authorization record.
func (s *Store) LoadAuthorization(_ context.Context, project gitmanager.ProjectIdentityDigest, id gitmanager.AuthorizationID) (gitmanager.GitAuthorization, error) {
	dir, name, err := authorizationLocation(project, id)
	if err != nil {
		return gitmanager.GitAuthorization{}, err
	}
	data, err := s.records.read(dir, name)
	if err != nil {
		return gitmanager.GitAuthorization{}, missingAuthorization(err)
	}
	return decodeAuthorization(data, project, id)
}

// TransitionAuthorization performs one compare-and-swap under the record
// directory lock: the durable record is decoded, the model applies the
// request against its state, version and owner, and only an accepted
// transition replaces the record.
func (s *Store) TransitionAuthorization(_ context.Context, request gitmanager.AuthorizationTransitionRequest) (gitmanager.GitAuthorization, error) {
	if err := request.Validate(); err != nil {
		return gitmanager.GitAuthorization{}, err
	}
	dir, name, err := authorizationLocation(request.ProjectIdentityDigest, request.AuthorizationID)
	if err != nil {
		return gitmanager.GitAuthorization{}, err
	}
	var next gitmanager.GitAuthorization
	err = s.records.replace(dir, name, func(current []byte) ([]byte, error) {
		authorization, err := decodeAuthorization(current, request.ProjectIdentityDigest, request.AuthorizationID)
		if err != nil {
			return nil, err
		}
		if next, err = authorization.Transition(request); err != nil {
			return nil, err
		}
		return gitmanager.EncodeAuthorizationRecord(next)
	})
	if err != nil {
		return gitmanager.GitAuthorization{}, missingAuthorization(err)
	}
	return next, nil
}

func decodeAuthorization(data []byte, project gitmanager.ProjectIdentityDigest, id gitmanager.AuthorizationID) (gitmanager.GitAuthorization, error) {
	authorization, err := gitmanager.DecodeAuthorizationRecord(data)
	if err != nil {
		return gitmanager.GitAuthorization{}, fmt.Errorf("authorization record %s: %w", id, err)
	}
	if authorization.AuthorizationID != id || authorization.ProjectIdentity.Digest() != project {
		return gitmanager.GitAuthorization{}, fmt.Errorf("authorization record %s is stored under another identity", id)
	}
	return authorization, nil
}

func missingAuthorization(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %v", gitmanager.ErrAuthorizationMissing, err)
	}
	return err
}
