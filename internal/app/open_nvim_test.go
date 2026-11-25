//go:build darwin || linux

package app

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/germanmunozs/ai-dev/internal/editor"
	"github.com/germanmunozs/ai-dev/internal/testutil/nvim"
)

// headlessNvim is a real Neovim listening on a socket, with project as its
// working directory.
type headlessNvim struct{ sock string }

func startHeadlessNvim(t *testing.T, nvimPath, project string) headlessNvim {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "aidn")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "n.sock")
	cmd := exec.Command(nvimPath, "--headless", "--clean", "--listen", sock)
	cmd.Dir = project
	cmd.Env = []string{"HOME=" + dir, "PATH=/usr/bin:/bin", "XDG_CONFIG_HOME=" + dir, "XDG_DATA_HOME=" + dir, "XDG_STATE_HOME=" + dir}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if info, err := os.Lstat(sock); err == nil && info.Mode()&os.ModeSocket != 0 {
			return headlessNvim{sock: sock}
		}
	}
	t.Fatal("headless Neovim did not create its socket")
	return headlessNvim{}
}

// lua runs a test chunk in the instance and returns its result.
func (n headlessNvim) lua(t *testing.T, chunk string) any {
	t.Helper()
	client, err := editor.Dial(n.sock, time.Now().Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	result, err := client.Call("nvim_exec_lua", chunk, []any{})
	if err != nil {
		t.Fatalf("nvim_exec_lua: %v", err)
	}
	return result
}

// state is the current buffer's name, the cursor, the window count and the
// number of listed buffers.
func (n headlessNvim) state(t *testing.T) (name string, row, col int64, windows, buffers int64) {
	t.Helper()
	v := n.lua(t, `local c = vim.api.nvim_win_get_cursor(0)
local listed = 0
for _, b in ipairs(vim.api.nvim_list_bufs()) do if vim.bo[b].buflisted then listed = listed + 1 end end
return { vim.api.nvim_buf_get_name(0), c[1], c[2], #vim.api.nvim_tabpage_list_wins(0), listed }`).([]any)
	return v[0].(string), v[1].(int64), v[2].(int64), v[3].(int64), v[4].(int64)
}

// TestNvimOpen opens files in a real headless Neovim through the built
// binary (F014-FR-003, FR-005, SC-001, SC-002).
func TestNvimOpen(t *testing.T) {
	nvimPath := nvim.Require(t)
	if nvimPath == "" {
		return
	}
	bin := buildSetupBinary(t)
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(base, "project")
	hostile := "a %b #c $HOME `touch pwned` ;d|e.txt"
	lines := "line one\nline two\nline three\nline four\nline five\n"
	if err := os.MkdirAll(filepath.Join(project, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{hostile, "five.txt", "-x b.txt"} {
		if err := os.WriteFile(filepath.Join(project, name), []byte(lines), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(base, "outside.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	parent := startHeadlessNvim(t, nvimPath, project)
	other := startHeadlessNvim(t, nvimPath, project)

	run := func(endpoint string, args ...string) (int, map[string]any) {
		t.Helper()
		cmd := exec.Command(bin, append([]string{"open", "--format", "json"}, args...)...)
		cmd.Dir = project
		cmd.Env = []string{"HOME=" + base, "PATH=/usr/bin:/bin", "NVIM=" + endpoint}
		out, _ := cmd.Output()
		var report map[string]any
		if err := json.Unmarshal(out, &report); err != nil {
			t.Fatalf("open %q: report %q: %v", args, out, err)
		}
		return cmd.ProcessState.ExitCode(), report
	}

	_, _, _, _, otherBuffers := other.state(t)
	start := time.Now()
	code, report := run(parent.sock, "--line", "3", "--column", "2", "--", hostile)
	if elapsed := time.Since(start); code != 0 || report["outcome"] != "opened" || report["target"] != "parent" || elapsed > time.Second {
		t.Fatalf("open hostile name: exit %d, %v, after %v", code, report, elapsed)
	}
	name, row, col, _, _ := parent.state(t)
	if name != filepath.Join(project, hostile) || row != 3 || col != 1 {
		t.Fatalf("parent shows %q at (%d, %d)", name, row, col)
	}
	if _, err := os.Stat(filepath.Join(project, "pwned")); err == nil {
		t.Fatal("a backquote in the file name ran a command")
	}
	if code, _ := run(parent.sock, "--line", "9999", "--", "five.txt"); code != 0 {
		t.Fatalf("open beyond the end: exit %d", code)
	}
	if _, row, _, _, _ := parent.state(t); row != 5 {
		t.Fatalf("cursor row %d, want the last line 5", row)
	}
	// Only the instance named by $NVIM is used (Edge Case "several Neovim
	// instances").
	if name, _, _, _, buffers := other.state(t); buffers != otherBuffers || name != "" {
		t.Fatalf("the other instance changed: %q, %d buffers", name, buffers)
	}

	// From a terminal window, the file opens in another window, focused.
	parent.lua(t, `vim.cmd.only(); vim.cmd.enew(); vim.fn.jobstart({ "sleep", "30" }, { term = true })`)
	if code, _ := run(parent.sock, "--", "-x b.txt"); code != 0 {
		t.Fatalf("open from a terminal window: exit %d", code)
	}
	name, _, _, windows, buffers := parent.state(t)
	if name != filepath.Join(project, "-x b.txt") || windows != 2 {
		t.Fatalf("from a terminal: current %q, %d windows", name, windows)
	}
	terminalShown := parent.lua(t, `for _, w in ipairs(vim.api.nvim_tabpage_list_wins(0)) do
  if vim.bo[vim.api.nvim_win_get_buf(w)].buftype == "terminal" then return true end
end
return false`)
	if terminalShown != true {
		t.Fatal("the terminal window was replaced")
	}

	for _, c := range []struct{ endpoint, path, outcome, reason string }{
		{filepath.Join(filepath.Dir(parent.sock), "missing.sock"), "five.txt", "editor_refused", "not_socket"},
		{"127.0.0.1:6666", "five.txt", "editor_refused", "not_local_socket"},
		{parent.sock, "../outside.txt", "path_refused", "outside_project"},
		{parent.sock, "/etc/hosts", "path_refused", "absolute_path"},
		{parent.sock, "dir", "path_refused", "not_regular_file"},
		{parent.sock, "missing.txt", "path_refused", "not_found"},
	} {
		code, report := run(c.endpoint, "--", c.path)
		if code != 1 || report["outcome"] != c.outcome || report["reason"] != c.reason {
			t.Errorf("%s with %s: exit %d, %v", c.path, c.endpoint, code, report)
		}
		if p := report["path"].(string); strings.Contains(p, base) || strings.HasPrefix(p, "/") {
			t.Errorf("report names an absolute path: %v", report)
		}
	}
	if _, _, _, _, after := parent.state(t); after != buffers {
		t.Fatalf("refused paths changed the buffer list: %d, want %d", after, buffers)
	}
}
