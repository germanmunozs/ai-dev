package workflow

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"testing"
	"testing/fstest"
	"time"
)

const testFeature = "004-workflow-state"

// recordingFS wraps a filesystem and records the name passed to every fs.FS
// and fs.ReadLinkFS method call. It implements and forwards fs.ReadLinkFS, so
// production code never rejects it for lacking no-follow capability: a zero
// count genuinely means no filesystem access occurred.
type recordingFS struct {
	inner fs.ReadLinkFS
	calls []string
}

func (r *recordingFS) Open(name string) (fs.File, error) {
	r.calls = append(r.calls, "Open:"+name)
	return r.inner.Open(name)
}

func (r *recordingFS) Lstat(name string) (fs.FileInfo, error) {
	r.calls = append(r.calls, "Lstat:"+name)
	return r.inner.Lstat(name)
}

func (r *recordingFS) ReadLink(name string) (string, error) {
	r.calls = append(r.calls, "ReadLink:"+name)
	return r.inner.ReadLink(name)
}

// openOnlyFS exposes only fs.FS (Open) and deliberately does not implement
// fs.ReadLinkFS.
type openOnlyFS struct {
	inner fs.FS
}

func (o openOnlyFS) Open(name string) (fs.File, error) { return o.inner.Open(name) }

func featureFS(featureID string, record *string) fstest.MapFS {
	fsys := fstest.MapFS{
		"specs/" + featureID: &fstest.MapFile{Mode: fs.ModeDir | 0o755},
	}
	if record != nil {
		fsys["specs/"+featureID+"/"+RecordFileName] = &fstest.MapFile{Data: []byte(*record)}
	}
	return fsys
}

func ptr(s string) *string { return &s }

func TestResolveRecordReadingRules(t *testing.T) {
	cases := []struct {
		name        string
		record      *string
		wantState   State
		wantInvalid bool
	}{
		{"absent file defaults to draft", nil, Draft, false},
		{"valid canonical value", ptr(`{"state":"planned"}`), Planned, false},
		{"unknown extra field is ignored", ptr(`{"state":"planned","extra":"ignored"}`), Planned, false},
		{"absent state field", ptr(`{}`), "", true},
		{"null state field", ptr(`{"state":null}`), "", true},
		{"empty state field", ptr(`{"state":""}`), "", true},
		{"whitespace-only state field", ptr(`{"state":"   "}`), "", true},
		{"wrong case", ptr(`{"state":"Planned"}`), "", true},
		{"padded value", ptr(`{"state":" planned "}`), "", true},
		{"non-canonical value", ptr(`{"state":"reviewing"}`), "", true},
		{"unparseable JSON", ptr(`not json`), "", true},
		{"empty file", ptr(``), "", true},
		{"state field of wrong type", ptr(`{"state":3}`), "", true},
		{"non-object JSON string", ptr(`"planned"`), "", true},
		{"top-level null", ptr(`null`), "", true},
		{"top-level array", ptr(`[{"state":"planned"}]`), "", true},
		{"trailing content", ptr(`{"state":"planned"} {}`), "", true},
		// Member names are exact and case-sensitive; a case variant of "state"
		// is never the state field and makes the record invalid.
		{"title-case key", ptr(`{"State":"approved"}`), "", true},
		{"upper-case key", ptr(`{"STATE":"planned"}`), "", true},
		{"unicode case-folding key", ptr(`{"ſtate":"approved"}`), "", true},
		{"padded key", ptr(`{" state":"planned"}`), "", true},
		{"exact key plus case variant", ptr(`{"state":"draft","State":"approved"}`), "", true},
		{"case variant plus exact key", ptr(`{"State":"approved","state":"draft"}`), "", true},
		// A duplicated exact state member is invalid, never last-key-wins.
		{"duplicate exact key", ptr(`{"state":"draft","state":"approved"}`), "", true},
		{"duplicate exact key same value", ptr(`{"state":"planned","state":"planned"}`), "", true},
		{"duplicate exact key first null", ptr(`{"state":null,"state":"planned"}`), "", true},
		{"nested state is not top-level", ptr(`{"meta":{"state":"approved"}}`), "", true},
		{"nested state ignored beside exact key", ptr(`{"meta":{"state":"approved"},"state":"planned"}`), Planned, false},
		{"duplicate unrelated key ignored", ptr(`{"extra":1,"extra":2,"state":"tasked"}`), Tasked, false},
	}
	for _, s := range allStates {
		cases = append(cases, struct {
			name        string
			record      *string
			wantState   State
			wantInvalid bool
		}{"canonical " + string(s), ptr(`{"state":"` + string(s) + `"}`), s, false})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fsys := featureFS(testFeature, tc.record)
			if !FeatureExists(fsys, testFeature) {
				t.Fatal("FeatureExists = false for a real feature directory")
			}
			state, invalid := ResolveRecord(fsys, testFeature)
			if state != tc.wantState || invalid != tc.wantInvalid {
				t.Fatalf("ResolveRecord = (%q, %v), want (%q, %v)", state, invalid, tc.wantState, tc.wantInvalid)
			}
		})
	}
}

func TestResolveRecordUnknownFieldResolvesLikePlainRecord(t *testing.T) {
	plainState, plainInvalid := ResolveRecord(featureFS(testFeature, ptr(`{"state":"planned"}`)), testFeature)
	extraState, extraInvalid := ResolveRecord(featureFS(testFeature, ptr(`{"state":"planned","extra":"ignored"}`)), testFeature)
	if plainState != extraState || plainInvalid != extraInvalid || extraState != Planned || extraInvalid {
		t.Fatalf("extra field changed resolution: plain=(%q,%v) extra=(%q,%v)", plainState, plainInvalid, extraState, extraInvalid)
	}
}

func TestResolveRecordSymlinkAndNonRegularAreInvalidWithoutReading(t *testing.T) {
	cases := map[string]fstest.MapFS{
		"symlinked record": {
			"specs/" + testFeature:                        &fstest.MapFile{Mode: fs.ModeDir | 0o755},
			"specs/" + testFeature + "/" + RecordFileName: &fstest.MapFile{Data: []byte("elsewhere.json"), Mode: fs.ModeSymlink},
			"specs/" + testFeature + "/elsewhere.json":    &fstest.MapFile{Data: []byte(`{"state":"planned"}`)},
		},
		"directory at record path": {
			"specs/" + testFeature + "/" + RecordFileName: &fstest.MapFile{Mode: fs.ModeDir | 0o755},
		},
	}
	for name, inner := range cases {
		t.Run(name, func(t *testing.T) {
			rec := &recordingFS{inner: inner}
			state, invalid := ResolveRecord(rec, testFeature)
			if state != "" || !invalid {
				t.Fatalf("ResolveRecord = (%q, %v), want (\"\", true)", state, invalid)
			}
			for _, call := range rec.calls {
				if call != "Lstat:specs/"+testFeature+"/"+RecordFileName {
					t.Fatalf("unexpected access %q: record content must never be read", call)
				}
			}
		})
	}
}

// faultFS injects deterministic, non-not-exist I/O errors for one path.
type faultFS struct {
	inner         fstest.MapFS
	path          string
	lstatErr      bool
	openErr       bool
	fakeRegularOK bool
}

var errInjected = errors.New("injected I/O failure")

func (f faultFS) Open(name string) (fs.File, error) {
	if f.openErr && name == f.path {
		return nil, &fs.PathError{Op: "open", Path: name, Err: errInjected}
	}
	return f.inner.Open(name)
}

func (f faultFS) ReadLink(name string) (string, error) { return f.inner.ReadLink(name) }

func (f faultFS) Lstat(name string) (fs.FileInfo, error) {
	if name == f.path {
		if f.lstatErr {
			return nil, &fs.PathError{Op: "lstat", Path: name, Err: errInjected}
		}
		if f.fakeRegularOK {
			return regularInfo{name: path.Base(name)}, nil
		}
	}
	return f.inner.Lstat(name)
}

type regularInfo struct{ name string }

func (r regularInfo) Name() string       { return r.name }
func (r regularInfo) Size() int64        { return 0 }
func (r regularInfo) Mode() fs.FileMode  { return 0o644 }
func (r regularInfo) ModTime() time.Time { return time.Time{} }
func (r regularInfo) IsDir() bool        { return false }
func (r regularInfo) Sys() any           { return nil }

func TestResolveRecordIOErrorsAreInvalid(t *testing.T) {
	recordPath := "specs/" + testFeature + "/" + RecordFileName
	base := featureFS(testFeature, ptr(`{"state":"planned"}`))
	cases := map[string]faultFS{
		"Lstat non-not-exist error":               {inner: base, path: recordPath, lstatErr: true},
		"ReadFile failure after successful Lstat": {inner: base, path: recordPath, openErr: true, fakeRegularOK: true},
	}
	for name, fsys := range cases {
		t.Run(name, func(t *testing.T) {
			if errors.Is(errInjected, fs.ErrNotExist) {
				t.Fatal("injected error must not be fs.ErrNotExist")
			}
			if !FeatureExists(fsys, testFeature) {
				t.Fatal("FeatureExists = false; fixture must reach record resolution")
			}
			state, invalid := ResolveRecord(fsys, testFeature)
			if state != "" || !invalid {
				t.Fatalf("ResolveRecord = (%q, %v), want (\"\", true)", state, invalid)
			}
		})
	}
}

func TestFilesystemWithoutReadLinkFSIsRejected(t *testing.T) {
	fsys := openOnlyFS{inner: featureFS(testFeature, ptr(`{"state":"planned"}`))}
	if _, ok := any(fsys).(fs.ReadLinkFS); ok {
		t.Fatal("test wrapper must not implement fs.ReadLinkFS")
	}
	if FeatureExists(fsys, testFeature) {
		t.Fatal("FeatureExists = true for a filesystem lacking fs.ReadLinkFS; want rejection, not a link-following fallback")
	}
	if state, invalid := ResolveRecord(fsys, testFeature); state != "" || !invalid {
		t.Fatalf("ResolveRecord = (%q, %v), want (\"\", true)", state, invalid)
	}
}

func TestRawFeatureIDContainment(t *testing.T) {
	for _, id := range []string{".", "..", "../x", "a/../../x", "/etc/passwd", "a/b", ""} {
		t.Run("invalid "+id, func(t *testing.T) {
			if validFeatureID(id) {
				t.Fatalf("validFeatureID(%q) = true, want false", id)
			}
			rec := &recordingFS{inner: featureFS(testFeature, nil)}
			if FeatureExists(rec, id) {
				t.Fatalf("FeatureExists(%q) = true, want false", id)
			}
			if len(rec.calls) != 0 {
				t.Fatalf("FeatureExists(%q) accessed the filesystem: %v", id, rec.calls)
			}
			if state, invalid := ResolveRecord(rec, id); state != "" || !invalid {
				t.Fatalf("ResolveRecord(%q) = (%q, %v), want (\"\", true)", id, state, invalid)
			}
			if len(rec.calls) != 0 {
				t.Fatalf("ResolveRecord(%q) accessed the filesystem: %v", id, rec.calls)
			}
		})
	}

	t.Run("valid positive control", func(t *testing.T) {
		if !validFeatureID(testFeature) {
			t.Fatalf("validFeatureID(%q) = false, want true", testFeature)
		}
		rec := &recordingFS{inner: featureFS(testFeature, nil)}
		if !FeatureExists(rec, testFeature) {
			t.Fatal("FeatureExists = false for valid control")
		}
		if len(rec.calls) != 1 || rec.calls[0] != "Lstat:specs/004-workflow-state" {
			t.Fatalf("accessed paths = %v, want exactly [Lstat:specs/004-workflow-state]", rec.calls)
		}
	})
}

func TestSymlinkedFeatureDirectoryIsNotFollowed(t *testing.T) {
	inner := fstest.MapFS{
		"specs/" + testFeature: &fstest.MapFile{Data: []byte("real"), Mode: fs.ModeSymlink},
		"specs/real":           &fstest.MapFile{Mode: fs.ModeDir | 0o755},
	}
	rec := &recordingFS{inner: inner}
	if FeatureExists(rec, testFeature) {
		t.Fatal("FeatureExists = true for a symlinked feature directory")
	}
	for _, call := range rec.calls {
		if call != "Lstat:specs/"+testFeature {
			t.Fatalf("unexpected access %q: symlink must not be followed", call)
		}
	}
}

func TestRealFilesystemSymlinksAreNotFollowed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}
	dir := t.TempDir()
	mustMkdir := func(p string) {
		if err := os.MkdirAll(filepath.Join(dir, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mustMkdir("outside")
	mustMkdir("specs/real-feature")
	if err := os.WriteFile(filepath.Join(dir, "outside", RecordFileName), []byte(`{"state":"planned"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "outside"), filepath.Join(dir, "specs", "linked-feature")); err != nil {
		t.Skipf("symlinks unsupported in this environment: %v", err)
	}
	if err := os.Symlink(filepath.Join(dir, "outside", RecordFileName), filepath.Join(dir, "specs", "real-feature", RecordFileName)); err != nil {
		t.Skipf("symlinks unsupported in this environment: %v", err)
	}
	root := os.DirFS(dir)
	if _, ok := root.(fs.ReadLinkFS); !ok {
		t.Fatal("os.DirFS must implement fs.ReadLinkFS for production no-follow inspection")
	}
	if FeatureExists(root, "linked-feature") {
		t.Fatal("FeatureExists = true for a real symlinked feature directory")
	}
	if !FeatureExists(root, "real-feature") {
		t.Fatal("FeatureExists = false for a real feature directory")
	}
	if state, invalid := ResolveRecord(root, "real-feature"); state != "" || !invalid {
		t.Fatalf("ResolveRecord = (%q, %v) for a real symlinked record, want (\"\", true)", state, invalid)
	}
}
