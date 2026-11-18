package gitmanager

import (
	"sort"
	"strings"
)

// gitFlowAVHKeys is the normative AVH profile key set (spec, Normative Git
// Flow Profiles).
var gitFlowAVHKeys = []string{
	"gitflow.branch.develop", "gitflow.branch.master", "gitflow.prefix.bugfix", "gitflow.prefix.feature",
	"gitflow.prefix.hotfix", "gitflow.prefix.release", "gitflow.prefix.support", "gitflow.prefix.versiontag",
}

// gitFlowNextProperties are the per-branch properties git-flow-next uses.
var gitFlowNextProperties = map[string]bool{
	"type": true, "parent": true, "startpoint": true, "upstreamstrategy": true, "downstreamstrategy": true,
	"autoupdate": true, "tag": true, "prefix": true, "worktree": true, "tagprefix": true,
}

var gitFlowTopics = []string{"feature", "bugfix", "release", "hotfix", "support"}

// gitFlowNextProfile is the exact normative git-flow-next profile for the
// given branches and prefixes.
func gitFlowNextProfile(production, development string, prefixes map[string]string, tagPrefix string) map[string]string {
	p := map[string]string{"gitflow.version": "1.0", "gitflow.initialized": "true"}
	set := func(branch string, values ...string) {
		for i := 0; i+1 < len(values); i += 2 {
			p["gitflow.branch."+branch+"."+values[i]] = values[i+1]
		}
	}
	set(production, "type", "base", "upstreamstrategy", "none", "downstreamstrategy", "none", "autoupdate", "false", "tag", "false")
	set(development, "type", "base", "parent", production, "upstreamstrategy", "merge", "downstreamstrategy", "merge", "autoupdate", "true", "tag", "false")
	topic := func(name, parent, start, up, down, tag string) {
		set(name, "type", "topic", "parent", parent, "startpoint", start, "upstreamstrategy", up, "downstreamstrategy", down,
			"prefix", prefixes[name], "autoupdate", "false", "tag", tag, "worktree", "false")
		if tag == "true" {
			set(name, "tagprefix", tagPrefix)
		}
	}
	topic("feature", development, development, "merge", "rebase", "false")
	topic("bugfix", development, development, "merge", "rebase", "false")
	topic("release", production, development, "merge", "merge", "true")
	topic("hotfix", production, production, "merge", "rebase", "true")
	topic("support", production, production, "none", "none", "false")
	return p
}

// classifyGitFlow builds the Git Flow evidence of the local repository
// configuration only (F008-FR-139). It never selects a strategy.
func classifyGitFlow(config ConfigEvaluation, local *InspectedSourceIdentity) (GitFlowEvidenceInput, bool) {
	values := make(map[string]string)
	var keys []string
	contradictory := false
	for _, record := range config.values {
		key := record.line.key
		if record.scope != ConfigScopeLocal || !strings.HasPrefix(key, "gitflow.") {
			continue
		}
		if previous, seen := values[key]; seen {
			if previous != record.line.value {
				contradictory = true
			}
		} else {
			keys = append(keys, key)
		}
		values[key] = record.line.value
	}
	if len(keys) == 0 {
		return GitFlowEvidenceInput{ObservationState: ObservationObservedEmpty, Classification: GitFlowAbsent}, true
	}
	if local == nil || local.ContentDigest == "" {
		return GitFlowEvidenceInput{}, false
	}
	sort.Strings(keys)
	in := GitFlowEvidenceInput{ObservationState: ObservationObserved, SourceDigests: []ContentDigest{local.ContentDigest}}
	entry := func(key string) GitFlowEntry {
		return GitFlowEntry{Key: key, Value: values[key], Origin: "local", SourceDigest: local.ContentDigest}
	}
	var avh, next, other bool
	for _, key := range keys {
		rest := strings.TrimPrefix(key, "gitflow.")
		parts := strings.Split(rest, ".")
		switch {
		case len(parts) == 3 && parts[0] == "branch" && parts[2] == "base":
			in.InstanceBases = append(in.InstanceBases, entry(key))
			continue
		case containsValue(gitFlowAVHKeys, key):
			avh = true
		case key == "gitflow.version" || key == "gitflow.initialized" ||
			(len(parts) == 3 && parts[0] == "branch" && gitFlowNextProperties[parts[2]]):
			next = true
		default:
			other = true
		}
		in.Entries = append(in.Entries, entry(key))
		if strings.HasPrefix(key, "gitflow.prefix.") || strings.HasSuffix(key, ".prefix") || strings.HasSuffix(key, ".tagprefix") {
			in.Prefixes = append(in.Prefixes, entry(key))
		}
	}
	switch {
	case contradictory:
		in.Classification = GitFlowContradictory
		in.Issues = []string{"a gitflow key has conflicting values"}
	case other:
		in.Classification = GitFlowUnrecognized
		in.Issues = []string{"gitflow keys outside the AVH and git-flow-next profiles"}
	case avh && next:
		in.Classification = GitFlowMixed
		in.Issues = []string{"keys from both the AVH and git-flow-next profiles"}
	case avh:
		classifyAVH(&in, values)
	case next:
		classifyNext(&in, values, keys)
	default:
		in.Classification = GitFlowUnrecognized
		in.Issues = []string{"only topic base keys are present"}
	}
	if in.Classification != GitFlowCompleteAVH && in.Classification != GitFlowCompleteGitFlowNext {
		in.ProductionBranch, in.DevelopmentBranch = "", ""
		if in.Classification != GitFlowPartial {
			in.Dialect = ""
		}
	}
	return in, true
}

func classifyAVH(in *GitFlowEvidenceInput, values map[string]string) {
	in.Dialect = GitFlowDialectAVH
	for _, key := range gitFlowAVHKeys {
		if _, present := values[key]; !present {
			in.Classification = GitFlowPartial
			return
		}
	}
	production, development := values["gitflow.branch.master"], values["gitflow.branch.develop"]
	if production == "" || development == "" || production == development {
		in.Classification, in.Issues = GitFlowContradictory, []string{"production and development branches are empty or equal"}
		return
	}
	in.Classification, in.ProductionBranch, in.DevelopmentBranch = GitFlowCompleteAVH, production, development
}

func classifyNext(in *GitFlowEvidenceInput, values map[string]string, keys []string) {
	in.Dialect = GitFlowDialectNext
	var production, development string
	for _, key := range keys {
		parts := strings.Split(strings.TrimPrefix(key, "gitflow."), ".")
		if len(parts) != 3 || parts[2] != "type" || values[key] != "base" {
			continue
		}
		if _, hasParent := values["gitflow.branch."+parts[1]+".parent"]; hasParent {
			development = parts[1]
		} else {
			production = parts[1]
		}
	}
	prefixes := make(map[string]string)
	for _, topic := range gitFlowTopics {
		prefixes[topic] = values["gitflow.branch."+topic+".prefix"]
	}
	tagPrefix := values["gitflow.branch.release.tagprefix"]
	if production == "" || development == "" {
		in.Classification = GitFlowPartial
		return
	}
	expected := gitFlowNextProfile(production, development, prefixes, tagPrefix)
	for _, entry := range in.Entries {
		want, known := expected[entry.Key]
		if !known || (want != entry.Value && !strings.HasSuffix(entry.Key, "prefix")) {
			in.Classification, in.Issues = GitFlowContradictory, []string{"a git-flow-next value differs from the normative profile"}
			return
		}
	}
	if len(in.Entries) != len(expected) {
		in.Classification = GitFlowPartial
		return
	}
	in.Classification, in.ProductionBranch, in.DevelopmentBranch = GitFlowCompleteGitFlowNext, production, development
}
