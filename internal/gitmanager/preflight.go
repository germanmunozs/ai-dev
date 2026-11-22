package gitmanager

import (
	"bytes"
	"errors"
)

// approvedBytes reads and proves every approved file of an initial commit
// (F008-FR-085, 191). In Phase 1, before the claim and the first mutation
// of the attempt, the project configuration this attempt will write is
// its in-memory rendering; in Phase 2, the barrier immediately before the
// first blob, it must exist on disk with those bytes. Every other file is
// read through a descriptor and must keep its frozen identity, content,
// blob OID and Git mode, and pass secret screening. Any failure refuses
// the whole group before any blob exists; nothing is returned partially.
// Attribute and conversion evidence is part of the snapshot the fresh
// inspection reproduced, so it is not reread here.
func approvedBytes(plan GitInitializationPlan, barrier bool) (map[string][]byte, *PlanRejection) {
	intent := plan.ImmutableIntent
	if !intent.InitialCommit {
		return nil, nil
	}
	drift := ReasonPlanStale
	if barrier {
		drift = ReasonConcurrentChange
	}
	changed := func(detail string) *PlanRejection { return reject(drift, "%s", detail) }
	config, err := renderProjectConfig(intent.ProjectID, intent.Strategy, intent.GitFlowDialect, intent.BranchParameters(), intent.PrefixParameters())
	if err != nil || contentDigest(config) != intent.ProjectConfigBytesDigest {
		return nil, reject(ReasonPlanInvalid, "the project configuration does not reproduce the frozen digest")
	}
	root := plan.PreconditionSnapshot.ProjectIdentity.ReversiblePathBytes()
	fileMode := configFlag(plan.PreconditionSnapshot.Observation, "core.filemode", true)
	approved := map[string][]byte{}
	var findings []SecurityFinding
	for _, file := range intent.AuthorizedFiles() {
		var data []byte
		if placeholderIdentity(file) && !barrier {
			data = config
		} else {
			content, err := readLiteralFile(root, file.PathUTF8, LimitAuthorizedFileBytes.Max())
			if err != nil {
				var limit *LimitError
				if errors.As(err, &limit) {
					return nil, reject(ReasonRepositoryStateIncompatible, "an approved file exceeds its bound")
				}
				return nil, changed("an approved file is no longer a literal regular file")
			}
			if !placeholderIdentity(file) && content.Identity != file.ObservedIdentity {
				return nil, changed("an approved file changed identity or metadata")
			}
			executable := fileMode && content.Identity.Mode.Perm()&0o100 != 0
			if executable != (file.Mode == "100755") {
				return nil, changed("an approved file changed its Git mode")
			}
			data = content.Data
		}
		if contentDigest(data) != file.ContentSHA256 || blobOID(intent.ObjectFormat, data) != file.BlobOID ||
			(file.PathUTF8 == ProjectConfigPath && !bytes.Equal(data, config)) {
			return nil, changed("an approved file no longer has its approved bytes")
		}
		findings = append(findings, secretFindings([]byte(file.PathUTF8), data)...)
		approved[file.PathUTF8] = data
	}
	if len(findings) > 0 {
		return nil, &PlanRejection{Reason: ReasonSensitiveContentSuspected, Detail: "high-risk content was found in approved files", Findings: findings}
	}
	return approved, nil
}
