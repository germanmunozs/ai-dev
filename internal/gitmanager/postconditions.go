package gitmanager

// preparedIndexArgvPath stands for the private prepared index when the
// planner builds the read-tree row: the path reaches only GIT_INDEX_FILE,
// so the planned argv never depends on it (F008-FR-185).
const preparedIndexArgvPath = "/prepared-index"

// commitGroup derives the initial-commit operation group (F008-FR-188,
// 191–192): object creation, the initial-ref CAS, the prepared-index
// installation and a final verification, each with its complete
// preconditions, effects and postconditions. Every OID is computed in
// process from the frozen intent, so the plan names the exact objects.
func commitGroup(commands GitCommandBuilder, intent ImmutableIntent, snapshot PreconditionSnapshot) ([]DerivedOperationInput, *PlanRejection) {
	observation := snapshot.Observation
	ref, rejection := initialCommitRef(intent.Strategy, intent.BranchParameters(), observation)
	if rejection != nil {
		return nil, rejection
	}
	if rejection := frozenFilesMatch(intent, observation); rejection != nil {
		return nil, rejection
	}
	objects, err := planCommit(intent)
	if err != nil {
		return nil, reject(ReasonPlanInvalid, "the initial commit cannot be encoded: %v", err)
	}
	commit, tree := objects.commit.oid, objects.rootTree()
	build := func(command GitCommand) ([]string, *PlanRejection) {
		invocation, rejection := buildCommand(commands, intent, snapshot, command)
		return invocation.Args, rejection
	}
	commitArgv, rejection := build(commitTreeCommand(intent, tree))
	if rejection != nil {
		return nil, rejection
	}
	casArgv, rejection := build(GitCommand{Kind: GitCommandUpdateRef, RefName: []byte(ref), ObjectOID: commit})
	if rejection != nil {
		return nil, rejection
	}
	indexArgv, rejection := build(GitCommand{Kind: GitCommandPreparedIndex, TreeOID: tree, IndexFile: []byte(preparedIndexArgvPath)})
	if rejection != nil {
		return nil, rejection
	}

	profile := snapshot.EnvironmentProfile.Version
	repositoryNormal := Condition{Kind: ConditionRepositoryKind, Expected: string(RepositoryKindNormal)}
	headRef := Condition{Kind: ConditionHeadSymbolicRef, Subject: refSubject(ref)}
	refAbsent := Condition{Kind: ConditionRefAbsent, Subject: refSubject(ref)}
	refCreated := Condition{Kind: ConditionRefTarget, Subject: refSubject(ref), Expected: string(commit)}
	commitExists := Condition{Kind: ConditionObjectType, Subject: string(commit), Expected: "commit"}
	index := Condition{Kind: ConditionIndexAbsent}
	if observation.Index.Presence == IndexPresent {
		index = Condition{Kind: ConditionIndexEmpty}
	}
	lockAbsent := Condition{Kind: ConditionLockFileAbsent}

	// The approved files, by content, and by identity where the plan read
	// them; the configuration written by this attempt has no identity yet.
	var files []Condition
	for _, file := range intent.AuthorizedFiles() {
		files = append(files, Condition{Kind: ConditionPathContent, Subject: pathSubject(file.PathUTF8), Expected: string(file.ContentSHA256)})
		if !placeholderIdentity(file) {
			files = append(files, Condition{Kind: ConditionPathIdentity, Subject: pathSubject(file.PathUTF8),
				Expected: file.ObservedIdentity.DeviceDecimal + ":" + file.ObservedIdentity.InodeDecimal})
		}
	}
	var created []Condition
	seen := map[Condition]bool{}
	add := func(c Condition) {
		if !seen[c] {
			seen[c] = true
			created = append(created, c)
		}
	}
	for _, file := range objects.blobs {
		add(Condition{Kind: ConditionObjectType, Subject: string(file.BlobOID), Expected: "blob"})
	}
	for _, t := range objects.trees {
		add(Condition{Kind: ConditionObjectType, Subject: string(t.oid), Expected: "tree"})
	}
	add(commitExists)

	indexTree := Condition{Kind: ConditionIndexTree, Expected: string(tree)}
	installed := []Condition{indexTree}
	for _, file := range intent.AuthorizedFiles() {
		installed = append(installed, Condition{Kind: ConditionPathStatusClean, Subject: pathSubject(file.PathUTF8)})
	}

	group := []DerivedOperationInput{
		{
			Kind: OperationKindInitialCommit, ExactArgv: commitArgv, EnvironmentProfile: profile,
			StdinDigest: contentDigest(intent.CommitMessageBytes()), TargetIdentity: "commit:" + string(commit),
			Preconditions:   append([]Condition{repositoryNormal, {Kind: ConditionHeadState, Expected: string(HeadStateUnborn)}, headRef, refAbsent, index, lockAbsent}, files...),
			ExpectedEffects: created, ExpectedPostconditions: created,
			FailureOutcomes:             []Reason{ReasonOperationFailed, ReasonConcurrentChange, ReasonSensitiveContentSuspected},
			PartialEffectClassification: PartialEffectPartialPossible,
		},
		{
			Kind: OperationKindInitialRefCAS, ExactArgv: casArgv, EnvironmentProfile: profile, TargetIdentity: "ref:" + ref,
			AffectedRefs:           []ObservedRef{NewObservedRef([]byte(ref))},
			Preconditions:          []Condition{repositoryNormal, headRef, refAbsent, commitExists},
			ExpectedEffects:        []Condition{refCreated},
			ExpectedPostconditions: []Condition{refCreated, {Kind: ConditionHeadState, Expected: string(HeadStateAttached)}, {Kind: ConditionReflogAbsent, Subject: refSubject(ref)}},
			FailureOutcomes:        []Reason{ReasonOperationFailed, ReasonConcurrentChange},
		},
		{
			Kind: OperationKindPreparedIndex, ExactArgv: indexArgv, EnvironmentProfile: profile, TargetIdentity: "index:" + string(tree),
			AffectedPaths: []ObservedPath{NewObservedPath([]byte(".git/index"))},
			Preconditions: []Condition{refCreated, index, lockAbsent}, ExpectedEffects: []Condition{indexTree},
			ExpectedPostconditions: installed,
			FailureOutcomes:        []Reason{ReasonOperationFailed, ReasonConcurrentChange, ReasonLockConflict},
			TemporaryResourceRoles: []ResourceRole{ResourceRoleIndexLock, ResourceRolePreparedIndex},
		},
		{
			Kind: OperationKindVerification, TargetIdentity: "commit:" + string(commit),
			Preconditions:   append([]Condition{refCreated, {Kind: ConditionHeadState, Expected: string(HeadStateAttached)}, indexTree}, preservedPaths(intent, observation)...),
			FailureOutcomes: []Reason{ReasonConcurrentChange},
		},
	}
	return group, nil
}

// commitTreeCommand is the commit-tree row of the frozen intent.
func commitTreeCommand(intent ImmutableIntent, tree ObjectOID) GitCommand {
	return GitCommand{Kind: GitCommandCommitTree, TreeOID: tree, Author: intent.Author, Committer: intent.Committer,
		AuthorDate: intent.AuthorTimestamp, CommitterDate: intent.CommitterTimestamp, Stdin: intent.CommitMessageBytes()}
}

// placeholderIdentity reports the in-memory project configuration that
// this attempt writes before the commit group.
func placeholderIdentity(file AuthorizedFile) bool {
	return file.PathUTF8 == ProjectConfigPath && file.ObservedIdentity.DeviceDecimal == "0" && file.ObservedIdentity.InodeDecimal == "0"
}

// frozenFilesMatch binds every approved file to the snapshot: a file the
// plan read must be the frozen identity and content, and the in-memory
// configuration must be absent from it. A tampered intent cannot name
// other bytes.
func frozenFilesMatch(intent ImmutableIntent, observation RepositoryObservation) *PlanRejection {
	identities := map[string]WorktreePathIdentity{}
	for _, identity := range observation.WorktreePathIdentities() {
		identities[string(identity.Path.Bytes())] = identity
	}
	for _, file := range intent.AuthorizedFiles() {
		frozen, present := identities[file.PathUTF8]
		switch {
		case placeholderIdentity(file):
			if present || file.ContentSHA256 != intent.ProjectConfigBytesDigest {
				return reject(ReasonPlanInvalid, "the in-memory project configuration does not match the snapshot")
			}
		case !present || frozen.Identity != file.ObservedIdentity || frozen.ContentDigest != file.ContentSHA256:
			return reject(ReasonPlanInvalid, "an approved file does not match the frozen snapshot")
		}
	}
	return nil
}

// preservedPaths are the frozen paths the commit does not authorize: each
// keeps its identity, and a regular file its content (F008-FR-125).
func preservedPaths(intent ImmutableIntent, observation RepositoryObservation) []Condition {
	authorized := map[string]bool{}
	for _, file := range intent.AuthorizedFiles() {
		authorized[file.PathUTF8] = true
	}
	var conditions []Condition
	for _, identity := range observation.WorktreePathIdentities() {
		path := string(identity.Path.Bytes())
		if authorized[path] {
			continue
		}
		subject := EncodeReversiblePath(identity.Path.Bytes())
		conditions = append(conditions, Condition{Kind: ConditionPathIdentity, Subject: subject,
			Expected: identity.Identity.DeviceDecimal + ":" + identity.Identity.InodeDecimal})
		if identity.ContentDigest != "" {
			conditions = append(conditions, Condition{Kind: ConditionPathContent, Subject: subject, Expected: string(identity.ContentDigest)})
		}
	}
	return conditions
}
