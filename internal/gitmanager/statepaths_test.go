package gitmanager

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// fakeEnvironment is a lookup over an explicit map: an absent key is unset,
// a present empty value is set but empty.
func fakeEnvironment(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func TestResolveStateRoot(t *testing.T) {
	tests := []struct {
		name string
		goos string
		env  map[string]string
		want string
	}{
		{"macOS", "darwin", map[string]string{"HOME": "/Users/u"}, "/Users/u/Library/Application Support/ai-dev/state/git"},
		{"macOS ignores XDG", "darwin", map[string]string{"HOME": "/Users/u", "XDG_STATE_HOME": "/x"}, "/Users/u/Library/Application Support/ai-dev/state/git"},
		{"Linux XDG", "linux", map[string]string{"HOME": "/home/u", "XDG_STATE_HOME": "/srv/state"}, "/srv/state/ai-dev/git"},
		{"Linux XDG without HOME", "linux", map[string]string{"XDG_STATE_HOME": "/srv/state"}, "/srv/state/ai-dev/git"},
		{"Linux unset XDG", "linux", map[string]string{"HOME": "/home/u"}, "/home/u/.local/state/ai-dev/git"},
		// o-09: an empty XDG_STATE_HOME is treated as unset.
		{"Linux empty XDG", "linux", map[string]string{"HOME": "/home/u", "XDG_STATE_HOME": ""}, "/home/u/.local/state/ai-dev/git"},
		{"trailing slash", "linux", map[string]string{"HOME": "/home/u/"}, "/home/u/.local/state/ai-dev/git"},
		{"filesystem root HOME", "linux", map[string]string{"HOME": "/"}, "/.local/state/ai-dev/git"},
		{"non-UTF-8 HOME", "linux", map[string]string{"HOME": "/home/\xff"}, "/home/\xff/.local/state/ai-dev/git"},
		// The resolved root ends in exactly one "git" even when the base
		// already does.
		{"base ending in git", "linux", map[string]string{"XDG_STATE_HOME": "/srv/git"}, "/srv/git/ai-dev/git"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, err := ResolveStateRoot(test.goos, fakeEnvironment(test.env))
			if err != nil {
				t.Fatal(err)
			}
			if got := string(root.Bytes()); got != test.want {
				t.Fatalf("root=%q want %q", got, test.want)
			}
			if !strings.HasSuffix(test.want, "/git") || strings.HasSuffix(test.want, "/git/git") {
				t.Fatalf("fixture %q appends another git segment", test.want)
			}
		})
	}

	refusals := []struct {
		name   string
		goos   string
		env    map[string]string
		reason Reason
	}{
		{"relative XDG", "linux", map[string]string{"HOME": "/home/u", "XDG_STATE_HOME": "state"}, ReasonAuditUnavailable},
		{"dot-relative XDG", "linux", map[string]string{"HOME": "/home/u", "XDG_STATE_HOME": "./state"}, ReasonAuditUnavailable},
		{"XDG with dot-dot", "linux", map[string]string{"XDG_STATE_HOME": "/srv/../etc"}, ReasonAuditUnavailable},
		{"XDG with dot", "linux", map[string]string{"XDG_STATE_HOME": "/srv/./state"}, ReasonAuditUnavailable},
		{"XDG with NUL", "linux", map[string]string{"XDG_STATE_HOME": "/srv\x00x"}, ReasonAuditUnavailable},
		{"Linux missing HOME", "linux", map[string]string{}, ReasonAuditUnavailable},
		{"Linux empty HOME", "linux", map[string]string{"HOME": ""}, ReasonAuditUnavailable},
		{"relative HOME", "linux", map[string]string{"HOME": "home/u"}, ReasonAuditUnavailable},
		{"HOME with double slash", "darwin", map[string]string{"HOME": "/Users//u"}, ReasonAuditUnavailable},
		{"macOS missing HOME", "darwin", map[string]string{"XDG_STATE_HOME": "/x"}, ReasonAuditUnavailable},
		{"Windows", "windows", map[string]string{"HOME": "/home/u"}, ReasonPlatformUnsupported},
		{"iOS", "ios", map[string]string{"HOME": "/home/u"}, ReasonPlatformUnsupported},
		{"Android", "android", map[string]string{"HOME": "/home/u"}, ReasonPlatformUnsupported},
		{"too long", "linux", map[string]string{"XDG_STATE_HOME": "/" + strings.Repeat("a/", 2100)}, ReasonAuditUnavailable},
	}
	for _, refusal := range refusals {
		t.Run(refusal.name, func(t *testing.T) {
			_, err := ResolveStateRoot(refusal.goos, fakeEnvironment(refusal.env))
			var rootErr *StateRootError
			if !errors.As(err, &rootErr) || rootErr.Reason != refusal.reason {
				t.Fatalf("got %v, want %s", err, refusal.reason)
			}
		})
	}
}

// The profile carries the fixed absent leaves directly beneath the root,
// and the bytes a Git subprocess receives are exactly the frozen bytes that
// plan, digest, and operation IDs cover.
func TestEnvironmentProfileForStateRoot(t *testing.T) {
	root, err := ResolveStateRoot("linux", fakeEnvironment(map[string]string{"HOME": "/home/\xff"}))
	if err != nil {
		t.Fatal(err)
	}
	profile, err := NewEnvironmentProfile(root)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Version != EnvironmentProfileVersion {
		t.Fatalf("version=%q", profile.Version)
	}
	wantHome := "/home/\xff/.local/state/ai-dev/git/environment/nonexistent-home-v1"
	wantXDG := "/home/\xff/.local/state/ai-dev/git/environment/nonexistent-xdg-v1"
	if string(profile.Home.Bytes()) != wantHome || string(profile.XDGConfigHome.Bytes()) != wantXDG {
		t.Fatalf("leaves %q %q", profile.Home.Bytes(), profile.XDGConfigHome.Bytes())
	}
	if profile.StateRoot.DisplayPathUTF8 != "" {
		t.Fatal("non-UTF-8 state root gained a display value")
	}

	entries := profile.GitEnvironmentEntries()
	want := []string{"HOME=" + wantHome, "XDG_CONFIG_HOME=" + wantXDG}
	if len(entries) != len(want) || entries[0] != want[0] || entries[1] != want[1] {
		t.Fatalf("entries=%q want %q", entries, want)
	}
	for i, entry := range entries {
		frozen := [][]byte{profile.Home.Bytes(), profile.XDGConfigHome.Bytes()}[i]
		if !bytes.Equal([]byte(entry[strings.IndexByte(entry, '=')+1:]), frozen) {
			t.Fatalf("execution bytes differ from frozen bytes: %q", entry)
		}
	}

	// The same frozen bytes reach the snapshot, the digest, and every Git
	// operation ID; another root changes all three.
	other, err := ResolveStateRoot("linux", fakeEnvironment(map[string]string{"HOME": "/home/v"}))
	if err != nil {
		t.Fatal(err)
	}
	otherProfile, err := NewEnvironmentProfile(other)
	if err != nil {
		t.Fatal(err)
	}
	in := validOperationInput(0)
	a, _ := DeriveOperationID(in, profile)
	b, _ := DeriveOperationID(in, otherProfile)
	if a == b {
		t.Fatal("state root did not reach the operation ID")
	}
	plan := validPlanInput(t)
	plan.PreconditionSnapshot.EnvironmentProfile = profile
	first, err := ComputePlanDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.PreconditionSnapshot.EnvironmentProfile = otherProfile
	if second, _ := ComputePlanDigest(plan); second == first {
		t.Fatal("state root did not reach the plan digest")
	}

	if _, err := NewEnvironmentProfile(NewObservedPath([]byte("relative/git"))); err == nil {
		t.Fatal("relative state root accepted")
	}
	if _, err := NewEnvironmentProfile(ObservedPath{}); err == nil {
		t.Fatal("empty state root accepted")
	}
}
