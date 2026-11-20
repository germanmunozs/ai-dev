//go:build (darwin && !ios) || (linux && !android)

package gitstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/germanmunozs/ai-dev/internal/gitmanager"
)

// The attempt directory holds attempt.json, the events directory and the
// optional result.json (STO-08).
const (
	attemptRecordName = "attempt.json"
	resultRecordName  = "result.json"
)

func attemptPath(id gitmanager.ApplyID) (string, error) {
	if err := id.Validate(); err != nil {
		return "", err
	}
	return path.Join("attempts", string(id)), nil
}

// CreateAttempt exclusively creates the attempt directory and publishes
// its attempt record; an apply ID is never reused.
func (s *Store) CreateAttempt(_ context.Context, attempt gitmanager.GitApplyAttempt) error {
	data, err := gitmanager.EncodeAttemptRecord(attempt)
	if err != nil {
		return err
	}
	dir, err := attemptPath(attempt.ApplyID)
	if err != nil {
		return err
	}
	if err := s.root.Mkdir(dir, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%w: attempt %s", ErrRecordExists, attempt.ApplyID)
		}
		return err
	}
	if err := ensurePrivateDirectory(s.root, dir); err != nil {
		return err
	}
	if err := ensurePrivateDirectory(s.root, path.Join(dir, "events")); err != nil {
		return err
	}
	// Publication syncs the attempt directory, which also makes the events
	// directory durable; the attempts directory then records the new entry.
	if _, err := s.records.publish(dir, attemptRecordName, data); err != nil {
		return err
	}
	return s.records.syncDirectory("attempts")
}

// requireAttempt proves the attempt record exists before anything is
// recorded for the attempt.
func (s *Store) requireAttempt(id gitmanager.ApplyID) (string, error) {
	dir, err := attemptPath(id)
	if err != nil {
		return "", err
	}
	info, err := s.root.Lstat(path.Join(dir, attemptRecordName))
	if err != nil {
		return "", fmt.Errorf("attempt %s has no attempt record: %w", id, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("attempt %s record is not a regular file", id)
	}
	return dir, nil
}

// RecordAttemptResult publishes the attempt's result once.
func (s *Store) RecordAttemptResult(_ context.Context, id gitmanager.ApplyID, record []byte) (gitmanager.ContentDigest, error) {
	dir, err := s.requireAttempt(id)
	if err != nil {
		return "", err
	}
	if len(record) == 0 {
		return "", errors.New("empty attempt result")
	}
	return s.records.publish(dir, resultRecordName, record)
}

// AppendAuditEvent is the sole append-only audit writer: it accepts only
// the next sequence number of the attempt and only an event kind the
// published trail allows, then publishes the event without replacement.
func (s *Store) AppendAuditEvent(_ context.Context, event gitmanager.AuditEvent) (gitmanager.ContentDigest, error) {
	if err := event.Validate(); err != nil {
		return "", err
	}
	dir, err := s.requireAttempt(event.ApplyID)
	if err != nil {
		return "", err
	}
	events := path.Join(dir, "events")
	published, _, err := s.records.listEvents(events)
	if err != nil {
		return "", err
	}
	if event.Sequence != uint64(len(published))+1 {
		return "", fmt.Errorf("audit event sequence %d, next is %d", event.Sequence, len(published)+1)
	}
	if err := gitmanager.CheckAuditEventOrder(kinds(published), event.Kind); err != nil {
		return "", err
	}
	return s.records.publishEvent(events, event.Sequence, event.Kind, event.Record)
}

// ListAuditEvents reports the published trail of one attempt and whether
// crash evidence makes it incomplete.
func (s *Store) ListAuditEvents(_ context.Context, id gitmanager.ApplyID) (gitmanager.AuditTrail, error) {
	dir, err := s.requireAttempt(id)
	if err != nil {
		return gitmanager.AuditTrail{}, err
	}
	published, temporaries, err := s.records.listEvents(path.Join(dir, "events"))
	if err != nil {
		return gitmanager.AuditTrail{}, err
	}
	return gitmanager.AuditTrail{Events: kinds(published), CrashEvidence: len(temporaries) > 0}, nil
}

func kinds(events []event) []gitmanager.AuditEventKind {
	list := make([]gitmanager.AuditEventKind, len(events))
	for i, e := range events {
		list[i] = e.kind
	}
	return list
}
