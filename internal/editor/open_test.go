//go:build darwin || linux

package editor

import (
	"bytes"
	"errors"
	"net"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeNeovim is an in-process peer on a Unix socket. reply receives each
// decoded request and writes whatever it wants back.
type fakeNeovim struct {
	addr     string
	mu       sync.Mutex
	requests [][]any
}

func startFake(t *testing.T, reply func(conn net.Conn, request []any)) *fakeNeovim {
	t.Helper()
	f := &fakeNeovim{addr: filepath.Join(shortDir(t), "n.sock")}
	listener, err := net.Listen("unix", f.addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				for {
					v, err := decode(conn, maxMessage)
					if err != nil {
						return
					}
					request, _ := v.([]any)
					f.mu.Lock()
					f.requests = append(f.requests, request)
					f.mu.Unlock()
					reply(conn, request)
				}
			}()
		}
	}()
	return f
}

func (f *fakeNeovim) seen() [][]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.requests)
}

func respond(conn net.Conn, id any, errValue, result any) {
	data, err := encode([]any{1, id, errValue, result})
	if err != nil {
		panic(err)
	}
	conn.Write(data)
}

func apiInfo(major, minor, patch int) []any {
	return []any{1, map[string]any{"version": map[string]any{"major": major, "minor": minor, "patch": patch, "api_level": 13}}}
}

// neovimReplies answers like Neovim of the given version, and succeeds on
// nvim_exec_lua.
func neovimReplies(version []any) func(net.Conn, []any) {
	return func(conn net.Conn, request []any) {
		if len(request) != 4 {
			return
		}
		switch request[2] {
		case "nvim_get_api_info":
			respond(conn, request[1], nil, version)
		case "nvim_exec_lua":
			respond(conn, request[1], nil, nil)
		}
	}
}

var testLocation = Location{Rel: "a b.txt", Abs: "/p/a b.txt", Line: 12, Column: 3}

// F014-FR-003, FR-005, SC-001, research R-02.
func TestOpenInParentSendsStructuredCalls(t *testing.T) {
	f := startFake(t, neovimReplies(apiInfo(0, 11, 7)))
	if err := OpenInParent(f.addr, testLocation); err != nil {
		t.Fatal(err)
	}
	requests := f.seen()
	if len(requests) != 2 {
		t.Fatalf("requests = %v", requests)
	}
	if requests[0][2] != "nvim_get_api_info" || len(requests[0][3].([]any)) != 0 {
		t.Fatalf("first request = %v", requests[0])
	}
	params := requests[1][3].([]any)
	if requests[1][2] != "nvim_exec_lua" || len(params) != 2 || params[0] != openChunk {
		t.Fatalf("second request = %v", requests[1])
	}
	args := params[1].([]any)
	if len(args) != 3 || args[0] != "/p/a b.txt" || args[1] != int64(12) || args[2] != int64(3) {
		t.Fatalf("exec_lua arguments = %v", args)
	}
	if strings.Contains(openChunk, "a b.txt") {
		t.Fatal("the path is inside the chunk text")
	}
}

func TestOpenInParentChecksTheVersion(t *testing.T) {
	cases := []struct {
		info any
		want string
	}{
		{apiInfo(0, 11, 7), ""},
		{apiInfo(0, 12, 6), ""},
		{apiInfo(1, 0, 0), ""},
		{apiInfo(0, 11, 6), ReasonVersionUnsupported},
		{apiInfo(0, 10, 4), ReasonVersionUnsupported},
		{[]any{1, map[string]any{"functions": []any{}}}, ReasonNotNeovim},
		{[]any{1, map[string]any{"version": "0.11.7"}}, ReasonNotNeovim},
		{[]any{1, map[string]any{"version": map[string]any{"major": 0, "minor": 11}}}, ReasonNotNeovim},
		{"hello", ReasonNotNeovim},
		{[]any{1}, ReasonNotNeovim},
	}
	for _, c := range cases {
		f2 := startFake(t, func(conn net.Conn, request []any) {
			if request[2] == "nvim_get_api_info" {
				respond(conn, request[1], nil, c.info)
				return
			}
			respond(conn, request[1], nil, nil)
		})
		if got := endpointReason(OpenInParent(f2.addr, testLocation)); got != c.want {
			t.Errorf("api info %v: reason %q, want %q", c.info, got, c.want)
		}
		if c.want != "" && len(f2.seen()) != 1 {
			t.Errorf("api info %v: %d requests; nothing may be opened after a refusal", c.info, len(f2.seen()))
		}
	}
}

func TestOpenInParentFailures(t *testing.T) {
	t.Run("no answer", func(t *testing.T) {
		f := startFake(t, func(net.Conn, []any) {})
		start := time.Now()
		got := endpointReason(OpenInParent(f.addr, testLocation))
		if elapsed := time.Since(start); got != ReasonNoAnswer || elapsed > 1200*time.Millisecond {
			t.Fatalf("reason %q after %v", got, elapsed)
		}
	})
	t.Run("slow answers stop at the overall limit", func(t *testing.T) {
		f := startFake(t, func(conn net.Conn, request []any) {
			time.Sleep(900 * time.Millisecond)
			neovimReplies(apiInfo(0, 12, 6))(conn, request)
		})
		saved := overallTimeout
		t.Cleanup(func() { overallTimeout = saved })
		overallTimeout = 1500 * time.Millisecond
		start := time.Now()
		got := endpointReason(OpenInParent(f.addr, testLocation))
		if elapsed := time.Since(start); got != ReasonNoAnswer || elapsed > 1700*time.Millisecond {
			t.Fatalf("reason %q after %v", got, elapsed)
		}
	})
	t.Run("refused connection", func(t *testing.T) {
		addr := filepath.Join(shortDir(t), "gone.sock")
		listener, err := net.Listen("unix", addr)
		if err != nil {
			t.Fatal(err)
		}
		listener.(*net.UnixListener).SetUnlinkOnClose(false)
		listener.Close()
		if got := endpointReason(OpenInParent(addr, testLocation)); got != ReasonNoAnswer {
			t.Fatalf("reason %q", got)
		}
	})
	t.Run("error response", func(t *testing.T) {
		f := startFake(t, func(conn net.Conn, request []any) {
			if request[2] == "nvim_get_api_info" {
				respond(conn, request[1], nil, apiInfo(0, 12, 6))
				return
			}
			respond(conn, request[1], []any{0, "E499: Empty file name \x1b[31m"}, nil)
		})
		err := OpenInParent(f.addr, testLocation)
		var endpointErr *EndpointError
		if !errors.As(err, &endpointErr) || endpointErr.Reason != ReasonEditorError || endpointErr.Message != "E499: Empty file name \x1b[31m" {
			t.Fatalf("error = %#v", err)
		}
	})
	t.Run("oversized response", func(t *testing.T) {
		f := startFake(t, func(conn net.Conn, request []any) {
			conn.Write([]byte{0x94, 0x01})
			conn.Write(append([]byte{0x00, 0xdb, 0x00, 0x20, 0x00, 0x00}, bytes.Repeat([]byte{'x'}, 4096)...))
		})
		if got := endpointReason(OpenInParent(f.addr, testLocation)); got != ReasonNoAnswer {
			t.Fatalf("reason %q", got)
		}
	})
	t.Run("other messages are skipped", func(t *testing.T) {
		f := startFake(t, func(conn net.Conn, request []any) {
			data, _ := encode([]any{2, "nvim_buf_lines_event", []any{}})
			conn.Write(data)
			respond(conn, int64(999), nil, "wrong id")
			neovimReplies(apiInfo(0, 12, 6))(conn, request)
		})
		if err := OpenInParent(f.addr, testLocation); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("peer closes", func(t *testing.T) {
		f := startFake(t, func(conn net.Conn, _ []any) { conn.Close() })
		if got := endpointReason(OpenInParent(f.addr, testLocation)); got != ReasonNoAnswer {
			t.Fatalf("reason %q", got)
		}
	})
}

// The chunk opens names literally and reads its values only from its
// arguments (research R-02); TestNvimOpen proves it with real Neovim.
func TestOpenChunk(t *testing.T) {
	for _, want := range []string{"magic = { file = false, bar = false }", "nvim_cmd", "= ..."} {
		if !strings.Contains(openChunk, want) {
			t.Errorf("openChunk lacks %q", want)
		}
	}
	for _, banned := range []string{"nvim_command", "vim.cmd", "vim.fn.system", "vim.system", "execute(", "loadstring", "nvim_exec2", "nvim_exec("} {
		if strings.Contains(openChunk, banned) {
			t.Errorf("openChunk contains %q", banned)
		}
	}
}

func TestForegroundArgs(t *testing.T) {
	got := ForegroundArgs(Location{Abs: "/p/-x b", Line: 12, Column: 3})
	if want := []string{"+call cursor(12, 3)", "--", "/p/-x b"}; !slices.Equal(got, want) {
		t.Fatalf("ForegroundArgs = %q", got)
	}
}
