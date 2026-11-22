package gitmanager

import (
	"errors"
	"testing"
)

func fullPrimitives() FilesystemPrimitives {
	return FilesystemPrimitives{ExclusiveCreate: true, AdvisoryLock: true, FileSync: true, DirectorySync: true,
		AtomicSameFilesystemRename: true, AtomicReplace: true, StableIdentity: true, EvidenceDigest: testSHA256}
}

func observation(goos, fsType string) FilesystemObservation {
	return FilesystemObservation{GOOS: goos, FilesystemType: fsType, MountIdentity: "mount-1", DeviceDecimal: "1", InodeDecimal: "2", Local: true}
}

func TestClassifyFilesystemProductionProfiles(t *testing.T) {
	for _, tc := range []struct {
		name       string
		obs        FilesystemObservation
		primitives FilesystemPrimitives
		production bool
	}{
		{"local APFS on macOS", observation("darwin", "apfs"), fullPrimitives(), true},
		{"local ext4 on Linux", observation("linux", "ext4"), fullPrimitives(), true},
		{"APFS on Linux", observation("linux", "apfs"), fullPrimitives(), false},
		{"ext4 on macOS", observation("darwin", "ext4"), fullPrimitives(), false},
		{"HFS+", observation("darwin", "hfs"), fullPrimitives(), false},
		{"ext3", observation("linux", "ext3"), fullPrimitives(), false},
		{"xfs", observation("linux", "xfs"), fullPrimitives(), false},
		{"overlayfs", observation("linux", "overlay"), fullPrimitives(), false},
		{"NFS", observation("linux", "nfs4"), fullPrimitives(), false},
		{"SMB", observation("darwin", "smbfs"), fullPrimitives(), false},
		{"FUSE", observation("linux", "fuse.sshfs"), fullPrimitives(), false},
		{"tmpfs", observation("linux", "tmpfs"), fullPrimitives(), false},
		{"unknown", observation("linux", "unknown"), fullPrimitives(), false},
		{"read-only APFS", func() FilesystemObservation { o := observation("darwin", "apfs"); o.ReadOnly = true; return o }(), fullPrimitives(), false},
		{"remote-flagged ext4", func() FilesystemObservation { o := observation("linux", "ext4"); o.Local = false; return o }(), fullPrimitives(), false},
		{"ext4 without directory sync", observation("linux", "ext4"), func() FilesystemPrimitives { p := fullPrimitives(); p.DirectorySync = false; return p }(), false},
		{"APFS without advisory lock", observation("darwin", "apfs"), func() FilesystemPrimitives { p := fullPrimitives(); p.AdvisoryLock = false; return p }(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile, err := ClassifyFilesystem(FilesystemRoleStateRoot, tc.obs, tc.primitives)
			if err != nil {
				t.Fatal(err)
			}
			if profile.ProductionSupported != tc.production || profile.ProfileVersion != FilesystemProfileVersion || profile.Role != FilesystemRoleStateRoot {
				t.Fatalf("profile %+v, want production=%t", profile, tc.production)
			}
			err = RequireProductionFilesystem(profile)
			var fsErr *FilesystemError
			if tc.production != (err == nil) || (!tc.production && (!errors.As(err, &fsErr) || fsErr.Reason != ReasonFilesystemUnsupported)) {
				t.Fatalf("RequireProductionFilesystem: %v", err)
			}
		})
	}
}

// tmpfs is a labeled non-durability test fixture: never production, never
// native evidence, but distinguishable from other unsupported filesystems.
func TestTmpfsIsTestOnly(t *testing.T) {
	if !TestOnlyFilesystem(observation("linux", "tmpfs")) {
		t.Fatal("tmpfs not recognized as test-only")
	}
	for _, fsType := range []string{"ext4", "apfs", "overlay", "nfs"} {
		if TestOnlyFilesystem(observation("linux", fsType)) {
			t.Errorf("%s classified as test-only", fsType)
		}
	}
}

// Project-side roles are never probed by writing into the project: their
// primitives come from the supported profile, which native certification
// backs, and only for a production filesystem.
func TestCertifiedPrimitives(t *testing.T) {
	for _, tc := range []struct {
		obs  FilesystemObservation
		want bool
	}{
		{observation("darwin", "apfs"), true},
		{observation("linux", "ext4"), true},
		{observation("linux", "tmpfs"), false},
		{func() FilesystemObservation { o := observation("linux", "ext4"); o.ReadOnly = true; return o }(), false},
	} {
		primitives := CertifiedPrimitives(tc.obs)
		all := primitives.ExclusiveCreate && primitives.AdvisoryLock && primitives.FileSync && primitives.DirectorySync &&
			primitives.AtomicSameFilesystemRename && primitives.AtomicReplace && primitives.StableIdentity
		if all != tc.want || primitives.EvidenceDigest != "" {
			t.Errorf("%s/%s read-only=%t: certified=%+v", tc.obs.GOOS, tc.obs.FilesystemType, tc.obs.ReadOnly, primitives)
		}
		profile, err := ClassifyFilesystem(FilesystemRoleProject, tc.obs, primitives)
		if err != nil || profile.ProductionSupported != tc.want {
			t.Errorf("project profile %+v, %v", profile, err)
		}
	}
}

func TestClassifyFilesystemRejectsIncompleteObservations(t *testing.T) {
	for name, mutate := range map[string]func(*FilesystemObservation){
		"missing type":   func(o *FilesystemObservation) { o.FilesystemType = "" },
		"missing GOOS":   func(o *FilesystemObservation) { o.GOOS = "" },
		"missing mount":  func(o *FilesystemObservation) { o.MountIdentity = "" },
		"invalid device": func(o *FilesystemObservation) { o.DeviceDecimal = "01" },
		"invalid inode":  func(o *FilesystemObservation) { o.InodeDecimal = "" },
		"control flag":   func(o *FilesystemObservation) { o.MountFlags = []string{"a\nb"} },
	} {
		obs := observation("darwin", "apfs")
		mutate(&obs)
		if _, err := ClassifyFilesystem(FilesystemRoleProject, obs, fullPrimitives()); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := ClassifyFilesystem("bogus", observation("darwin", "apfs"), fullPrimitives()); err == nil {
		t.Error("unknown role accepted")
	}
	if _, err := ClassifyFilesystem(FilesystemRoleStateRoot, observation("darwin", "apfs"), FilesystemPrimitives{EvidenceDigest: "sha256:abc"}); err == nil {
		t.Error("malformed evidence digest accepted")
	}
}
