//go:build darwin || linux

package editor

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// shortDir is a temporary directory with a path short enough for a Unix
// socket address on macOS (104 bytes).
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "aid")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func listenUnix(t *testing.T, path string) {
	t.Helper()
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
}

func endpointReason(err error) string {
	var endpointErr *EndpointError
	if errors.As(err, &endpointErr) {
		return endpointErr.Reason
	}
	if err != nil {
		return "unexpected: " + err.Error()
	}
	return ""
}

// F014-FR-003, research R-03.
func TestCheckEndpoint(t *testing.T) {
	dir := shortDir(t)
	sock := filepath.Join(dir, "n.sock")
	listenUnix(t, sock)
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.sock")
	if err := os.Symlink(sock, link); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"":               "not_local_socket",
		"relative.sock":  "not_local_socket",
		"127.0.0.1:6666": "not_local_socket",
		"[::1]:6666":     "not_local_socket",
		file:             "not_socket",
		dir:              "not_socket",
		link:             "not_socket",
		dir + "/missing": "not_socket",
		sock:             "",
	}
	for addr, want := range cases {
		if got := endpointReason(CheckEndpoint(addr)); got != want {
			t.Errorf("CheckEndpoint(%q) = %q, want %q", addr, got, want)
		}
	}
}

func TestCheckEndpointOwner(t *testing.T) {
	sock := filepath.Join(shortDir(t), "n.sock")
	listenUnix(t, sock)
	saved := ownerOf
	t.Cleanup(func() { ownerOf = saved })
	ownerOf = func(os.FileInfo) (int, bool) { return os.Getuid() + 1, true }
	if got := endpointReason(CheckEndpoint(sock)); got != "not_owned" {
		t.Fatalf("a socket owned by another user: %q", got)
	}
	ownerOf = func(os.FileInfo) (int, bool) { return 0, false }
	if got := endpointReason(CheckEndpoint(sock)); got != "not_owned" {
		t.Fatalf("a socket whose owner is unknown: %q", got)
	}
}
