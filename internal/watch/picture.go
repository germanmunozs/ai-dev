package watch

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"time"

	"github.com/germanmunozs/ai-dev/internal/gate"
	"github.com/germanmunozs/ai-dev/internal/sdd"
	"github.com/germanmunozs/ai-dev/internal/workflow"
)

// artifactNames are the artifacts reported, in this order (contracts/watch.md).
var artifactNames = []string{sdd.SpecArtifact, sdd.PlanArtifact, sdd.TasksArtifact}

// featureView is one Observed Feature (data-model.md).
type featureView struct {
	exists     bool // a directory Feature 004 recognizes; false for a --feature name that does not exist
	state      StateResult
	next       []byte
	artifacts  []Artifact
	historyLen int // entries read at this observation; -1 when the history could not be read
	history    []gate.Evidence
}

// picture is the Project Picture: every result the snapshot carries.
type picture struct {
	features map[string]featureView
	setup    []byte
	env      EnvironmentResult
}

// observer computes results through the Source and reads metadata through
// the read-only fs.FS.
type observer struct {
	source  Source
	root    fs.FS
	project string
	filter  []string
}

// featureIDs lists the features to observe: every recognized feature, or,
// with --feature, the named ones whether or not they exist.
func (o observer) featureIDs() (ids []string, existing map[string]bool, err error) {
	listed, err := o.source.Features()
	if err != nil {
		return nil, nil, err
	}
	existing = map[string]bool{}
	for _, id := range listed {
		existing[id] = true
	}
	if len(o.filter) == 0 {
		return listed, existing, nil
	}
	ids = slices.Clone(o.filter)
	slices.Sort(ids)
	return slices.Compact(ids), existing, nil
}

// observeAttempts bounds how often an observation is repeated while a
// Human Gate keeps changing the record under it.
const observeAttempts = 3

// feature observes one feature. A name that is not a feature (a --feature
// that does not exist) is never read through: its artifacts are absent and
// its history unknown (review F6). The state and the history are two reads
// of the same record, so the history is read before and after the state and
// the observation is repeated until both reads agree; otherwise a Human
// Gate written in between would lose its evidence (review F7).
func (o observer) feature(id string, exists bool) featureView {
	if !exists {
		v := featureView{state: o.source.State(id), next: o.source.Next(id), historyLen: -1}
		for _, name := range artifactNames {
			v.artifacts = append(v.artifacts, Artifact{Name: name, Status: "absent"})
		}
		return v
	}
	var v featureView
	for range observeAttempts {
		before, okBefore := gate.ReadHistory(o.root, id)
		v = featureView{exists: true, state: o.source.State(id), next: o.source.Next(id), historyLen: -1}
		after, okAfter := gate.ReadHistory(o.root, id)
		if okBefore && okAfter && slices.EqualFunc(before, after, sameEntry) {
			v.historyLen, v.history = len(after), after
			break
		}
		if !okBefore && !okAfter {
			break // unreadable both times: no history
		}
	}
	v.artifacts = o.artifacts(id)
	return v
}

// sameEntry reports whether two history entries are the same record.
func sameEntry(a, b gate.Evidence) bool {
	return a.FeatureID == b.FeatureID && a.Decision == b.Decision && a.Approver == b.Approver && a.FromState == b.FromState &&
		a.ToState == b.ToState && a.DecidedAt.Equal(b.DecidedAt) && a.Reason == b.Reason && a.ReasonSet == b.ReasonSet
}

// artifacts reports the metadata of each artifact, never its content
// (F015-FR-021); a symbolic link is not followed.
func (o observer) artifacts(id string) []Artifact {
	dir, ok := workflow.FeatureDir(id)
	out := make([]Artifact, 0, len(artifactNames))
	for _, name := range artifactNames {
		a := Artifact{Name: name, Status: "absent"}
		if ok {
			if info, err := lstat(o.root, path.Join(dir, name)); err == nil {
				if info.Mode().IsRegular() {
					size := info.Size()
					a.Status, a.Size, a.Modified = "present", &size, info.ModTime().UTC().Format(time.RFC3339Nano)
				} else {
					a.Status = "not_regular_file"
				}
			}
		}
		out = append(out, a)
	}
	return out
}

func (o observer) build(ctx context.Context) (picture, error) {
	ids, existing, err := o.featureIDs()
	if err != nil {
		return picture{}, err
	}
	p := picture{features: map[string]featureView{}}
	for _, id := range ids {
		p.features[id] = o.feature(id, existing[id])
	}
	p.setup = o.source.Setup()
	p.env = o.source.Environment(ctx)
	return p, nil
}

func (p picture) snapshot() SnapshotData {
	ids := make([]string, 0, len(p.features))
	for id := range p.features {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	d := SnapshotData{Features: make([]FeatureData, 0, len(ids)), Setup: p.setup, Environment: p.env.JSON}
	for _, id := range ids {
		d.Features = append(d.Features, p.features[id].data(id))
	}
	return d
}

func (v featureView) data(id string) FeatureData {
	return FeatureData{Feature: id, State: v.state.JSON, Artifacts: v.artifacts, Next: v.next}
}

// watchSet is the set of paths to observe (data-model.md Watch Set): the
// directories that exist, and the regular files the macOS adapter needs.
func (o observer) watchSet(ids []string) (dirs, files []string) {
	abs := func(rel string) string { return filepath.Join(o.project, filepath.FromSlash(rel)) }
	isDir := func(rel string) bool { info, err := lstat(o.root, rel); return err == nil && info.IsDir() }
	isFile := func(rel string) bool { info, err := lstat(o.root, rel); return err == nil && info.Mode().IsRegular() }
	dirs = append(dirs, o.project)
	for _, rel := range []string{".ai-dev", "specs"} {
		if isDir(rel) {
			dirs = append(dirs, abs(rel))
		}
	}
	if isFile(".ai-dev/agents.json") {
		files = append(files, abs(".ai-dev/agents.json"))
	}
	for _, id := range ids {
		dir, ok := workflow.FeatureDir(id)
		if !ok || !isDir(dir) {
			continue
		}
		dirs = append(dirs, abs(dir))
		for _, name := range append([]string{workflow.RecordFileName}, artifactNames...) {
			if rel := path.Join(dir, name); isFile(rel) {
				files = append(files, abs(rel))
			}
		}
	}
	return dirs, files
}

// lstat inspects name without following a symbolic link.
func lstat(root fs.FS, name string) (fs.FileInfo, error) {
	l, ok := root.(fs.ReadLinkFS)
	if !ok {
		return nil, errors.New("no-follow inspection is unavailable")
	}
	return l.Lstat(name)
}
