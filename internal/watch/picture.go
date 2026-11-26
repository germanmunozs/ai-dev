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
	last       *gate.Evidence
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

func (o observer) feature(id string, exists bool) featureView {
	v := featureView{exists: exists, state: o.source.State(id), next: o.source.Next(id), artifacts: o.artifacts(id), historyLen: -1}
	if entries, ok := gate.ReadHistory(o.root, id); ok {
		v.historyLen = len(entries)
		if len(entries) > 0 {
			v.last = &entries[len(entries)-1]
		}
	}
	return v
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
