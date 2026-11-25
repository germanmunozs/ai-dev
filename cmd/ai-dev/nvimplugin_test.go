package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/germanmunozs/ai-dev/internal/testutil/nvim"
)

// luaRun runs the plugin's Lua tests named by files (all of them when none)
// in a headless Neovim with a private HOME and XDG directories, so no
// person's configuration loads (Feature 014, research R-08). extraEnv is
// added to that environment.
func luaRun(t *testing.T, extraEnv []string, files ...string) (string, error) {
	t.Helper()
	bin := nvim.Require(t)
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--headless", "--clean", "-l", "nvim/tests/run.lua")
	cmd.Dir = repoRoot
	home := t.TempDir()
	cmd.Env = append([]string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "config"),
		"XDG_DATA_HOME=" + filepath.Join(home, "data"),
		"XDG_STATE_HOME=" + filepath.Join(home, "state"),
		"XDG_CACHE_HOME=" + filepath.Join(home, "cache"),
		"TMPDIR=" + t.TempDir(),
		"AI_DEV_LUA_TESTS=" + strings.Join(files, ","),
	}, extraEnv...)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		err = errors.New("Lua tests timed out after 60 s")
	}
	return string(output), err
}

// runLua fails the test with the runner's output when a Lua test fails.
func runLua(t *testing.T, files ...string) string {
	t.Helper()
	output, err := luaRun(t, nil, files...)
	if err != nil {
		t.Fatalf("Lua tests %v: %v\n%s", files, err, output)
	}
	return output
}

func TestNvimLuaHarness(t *testing.T) {
	output := runLua(t, "harness_test.lua")
	if !strings.Contains(output, "ok harness_test.lua each_test_starts_in_a_fresh_state") {
		t.Fatalf("runner output lacks the passing test:\n%s", output)
	}

	failing := filepath.Join(t.TempDir(), "failing_test.lua")
	body := "return { fails = function() error(\"boom\") end }\n"
	if err := os.WriteFile(failing, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	output, err := luaRun(t, nil, failing)
	if err == nil {
		t.Fatalf("a failing Lua test did not fail the run:\n%s", output)
	}
	if !strings.Contains(output, "FAIL failing_test.lua fails:") || !strings.Contains(output, "boom") {
		t.Fatalf("runner output lacks the failure:\n%s", output)
	}
}

// The Lua sanitizer and quoting equal the Go ones on the shared vectors
// (F014-FR-023, SC-007).
func TestNvimLuaSanitizeQuote(t *testing.T) {
	runLua(t, "sanitize_test.lua", "quote_test.lua")
}

// The plugin's configuration, ai-dev runner and command (F014-FR-010,
// FR-019, FR-020; contracts/plugin.md Command, Removal).
func TestNvimLuaCore(t *testing.T) {
	runLua(t, "config_test.lua", "cli_test.lua", "init_test.lua")
}

// The read-only views (F014-FR-007 to FR-010, US2, SC-003, SC-008).
func TestNvimLuaViews(t *testing.T) {
	runLua(t, "views_test.lua", "views_perf_test.lua")
}

// pluginAgentPath returns a PATH directory holding a fake for every catalog
// agent, each appending its arguments to $FAKE_AGENT_LOG as the Lua fake
// ai-dev does (CALL, each argument, END, each followed by a NUL byte), and
// a fake sh that records any call in shMarker. No real agent can be found
// (SC-005).
func pluginAgentPath(t *testing.T, shMarker string) string {
	t.Helper()
	dir := t.TempDir()
	body := "#!/bin/sh\nprintf '%s\\0' CALL \"$@\" END >> \"$FAKE_AGENT_LOG\"\n"
	for _, run := range catalogRuns {
		if err := os.WriteFile(filepath.Join(dir, run.executable), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	sh := "#!/bin/sh\n: > '" + shMarker + "'\nexit 99\n"
	if err := os.WriteFile(filepath.Join(dir, "sh"), []byte(sh), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// The ask action with the built binary and fake agents: the agent receives
// exactly the instruction the preview showed, and no shell starts (SC-004,
// SC-005).
func TestNvimPluginAsk(t *testing.T) {
	bin := buildWorkflowNextBinary(t)
	shMarker := filepath.Join(t.TempDir(), "sh-ran")
	agentLog := filepath.Join(t.TempDir(), "agents.log")
	output, err := luaRun(t, []string{
		"PATH=" + pluginAgentPath(t, shMarker),
		"AI_DEV_E2E_BIN=" + bin,
		"FAKE_AGENT_LOG=" + agentLog,
	}, "e2e_ask_test.lua")
	if err != nil {
		t.Fatalf("Lua tests: %v\n%s", err, output)
	}
	if !strings.Contains(output, "ok e2e_ask_test.lua the_agent_receives_exactly_the_composed_instruction") {
		t.Fatalf("the end-to-end test did not run:\n%s", output)
	}
	if _, err := os.Stat(shMarker); err == nil {
		t.Fatal("a shell was started")
	}
	data, err := os.ReadFile(agentLog)
	if err != nil || strings.Count(string(data), "CALL\x00") != 4 {
		t.Fatalf("agent log: %v, %q", err, data)
	}
}

// The reference and ask action tests (F014-FR-011 to FR-017, US3).
func TestNvimLuaAsk(t *testing.T) {
	runLua(t, "reference_test.lua", "actions_ask_test.lua")
}

// Approve and abandon through the plugin give the same record, output and
// exit code as the same commands typed by hand (SC-004, US4).
func TestNvimPluginDecisions(t *testing.T) {
	bin := buildWorkflowNextBinary(t)
	var projects map[string]map[string]string
	readFixtureJSON(t, "projects.json", &projects)
	viaPlugin := makeFixtureProject(t, projects["views"])
	byHand := makeFixtureProject(t, projects["views"])

	// The same decisions as nvim/tests/e2e_decide_test.lua.
	typed := [][]string{
		{"workflow", "approve", "--approver", "Ana", "--", "01-draft", "specified"},
		{"workflow", "abandon", "--approver", "Ana", "--reason", "out of scope", "--", "02-specified", "abandoned"},
		{"workflow", "approve", "--approver", "Ana", "--reason", "no tasks yet", "--", "03-planned", "tasked"},
	}
	type result struct {
		Output   string `json:"output"`
		ExitCode int    `json:"exit_code"`
	}
	var want []result
	for _, args := range typed {
		cmd := exec.Command(bin, args...)
		cmd.Dir = byHand
		output, err := cmd.CombinedOutput()
		code := 0
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		want = append(want, result{string(output), code})
	}
	if want[2].ExitCode == 0 {
		t.Fatalf("the third decision must be refused: %+v", want[2])
	}

	outPath := filepath.Join(t.TempDir(), "results.json")
	output, err := luaRun(t, []string{
		"AI_DEV_E2E_BIN=" + bin,
		"AI_DEV_E2E_PROJECT=" + viaPlugin,
		"AI_DEV_E2E_OUT=" + outPath,
	}, "e2e_decide_test.lua")
	if err != nil {
		t.Fatalf("Lua tests: %v\n%s", err, output)
	}
	var got []result
	readJSONFile(t, outPath, &got)
	if len(got) != len(want) {
		t.Fatalf("plugin results %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("decision %d: plugin %+v, by hand %+v", i, got[i], want[i])
		}
	}
	for _, id := range []string{"01-draft", "02-specified", "03-planned"} {
		a, b := stateWithoutTimes(t, viaPlugin, id), stateWithoutTimes(t, byHand, id)
		if a != b {
			t.Errorf("%s: workflow-state.json via the plugin %s, by hand %s", id, a, b)
		}
	}
}

func readJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

// stateWithoutTimes returns a feature's workflow-state.json with every
// decided_at removed.
func stateWithoutTimes(t *testing.T, root, id string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "specs", id, "workflow-state.json"))
	if errors.Is(err, os.ErrNotExist) {
		return "(none)"
	}
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if gates, ok := record["human_gates"].([]any); ok {
		for _, gate := range gates {
			if g, ok := gate.(map[string]any); ok {
				delete(g, "decided_at")
			}
		}
	}
	normalized, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	return string(normalized)
}

// The decision and no-confirmation tests (F014-FR-012 to FR-017, FR-024,
// US4, SC-006).
func TestNvimLuaDecisions(t *testing.T) {
	runLua(t, "actions_decide_test.lua", "no_confirm_test.lua")
}

// nvimState starts Neovim with init as its configuration and reports
// whether :AiDev, the AiDev group and any AiDev mapping exist, and the
// text of :checkhealth ai-dev when the command exists.
func nvimState(t *testing.T, init string) map[string]any {
	t.Helper()
	bin := nvim.Require(t)
	dir := t.TempDir()
	initPath := filepath.Join(dir, "init.lua")
	if err := os.WriteFile(initPath, []byte(init), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "state.json")
	check := `local s = { command = vim.fn.exists(":AiDev"), group = vim.fn.exists("#AiDev"),
  maps = vim.fn.execute("map") .. vim.fn.execute("map!") }
if s.command == 2 then
  vim.cmd("silent checkhealth ai-dev")
  s.health = table.concat(vim.api.nvim_buf_get_lines(0, 0, -1, false), "\n")
end
local f = assert(io.open(` + strconvQuote(out) + `, "w"))
f:write(vim.json.encode(s))
f:close()
`
	checkPath := filepath.Join(dir, "check.lua")
	if err := os.WriteFile(checkPath, []byte(check), 0o644); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	cmd := exec.Command(bin, "--headless", "-u", initPath, "-i", "NONE", "-c", "luafile "+checkPath, "-c", "qa!")
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "config"),
		"XDG_DATA_HOME=" + filepath.Join(home, "data"),
		"XDG_STATE_HOME=" + filepath.Join(home, "state"),
		"XDG_CACHE_HOME=" + filepath.Join(home, "cache"),
	}
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("nvim: %v\n%s", err, output)
	}
	var state map[string]any
	readJSONFile(t, out, &state)
	return state
}

// The documented installation works without a plugin manager, and a
// configuration without the plugin leaves nothing of it (US5, F014-FR-018,
// FR-022); the views and actions write nothing, start only ai-dev and show
// no environment value (FR-025).
func TestNvimPluginInstall(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	installed := nvimState(t, "vim.opt.rtp:append("+strconvQuote(filepath.Join(repoRoot, "nvim"))+")\nrequire(\"ai-dev\").setup({})\n")
	if installed["command"] != float64(2) || installed["group"] != float64(1) {
		t.Fatalf("installed: %v", installed)
	}
	if health, _ := installed["health"].(string); !strings.Contains(health, "ai-dev") || !strings.Contains(health, "Neovim") {
		t.Fatalf("checkhealth ai-dev did not run: %q", health)
	}
	if strings.Contains(installed["maps"].(string), "AiDev") {
		t.Fatalf("setup({}) defined a mapping: %s", installed["maps"])
	}
	removed := nvimState(t, "-- the plugin removed\n")
	if removed["command"] != float64(0) || removed["group"] != float64(0) || strings.Contains(removed["maps"].(string), "AiDev") {
		t.Fatalf("removed: %v", removed)
	}

	markers := t.TempDir()
	for _, name := range append([]string{"sh", "bash", "zsh", "git", "specify", "curl", "wget"}, catalogExecutables()...) {
		script := "#!/bin/sh\n: > \"$AI_DEV_MARKER_DIR/" + name + ".ran\"\n"
		if err := os.WriteFile(filepath.Join(markers, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	runLuaEnv(t, []string{
		"PATH=" + markers + ":/usr/bin:/bin",
		"AI_DEV_MARKER_DIR=" + markers,
		"AI_DEV_TEST_SECRET=sentinel-4d1c9e",
		"NVIM=" + filepath.Join(t.TempDir(), "not-a-socket"),
	}, "install_test.lua", "ok install_test.lua views_and_actions_write_nothing_start_only_ai_dev_and_show_no_secret")
}

// TestNvimLuaInstall runs the health, preset and no-confirmation tests
// (F014-FR-019, FR-021, FR-024, SC-006).
func TestNvimLuaInstall(t *testing.T) {
	runLua(t, "health_test.lua", "lazyvim_test.lua", "no_confirm_test.lua")
}

// runLuaEnv runs one Lua test file with extraEnv and fails unless the
// output holds want, so a file that has no tests cannot pass silently.
func runLuaEnv(t *testing.T, extraEnv []string, file, want string) {
	t.Helper()
	output, err := luaRun(t, extraEnv, file)
	if err != nil {
		t.Fatalf("Lua tests %s: %v\n%s", file, err, output)
	}
	if !strings.Contains(output, want) {
		t.Fatalf("%s did not run %q:\n%s", file, want, output)
	}
}

func catalogExecutables() []string {
	var names []string
	for _, run := range catalogRuns {
		names = append(names, run.executable)
	}
	return names
}

func strconvQuote(s string) string { return strconv.Quote(s) }
