package editor

import (
	"errors"
	"fmt"
	"time"
)

// openChunk runs in the parent Neovim through nvim_exec_lua. It is a
// constant: the path and position arrive as its arguments (...), never in
// its text. It opens the file in a window other than a terminal (the
// previous window, the first non-terminal window of the tab page, or a new
// split), with file-name expansion disabled so %, # and backquotes stay
// literal (research R-02), then clamps the cursor to the buffer and focuses
// the window (contracts/open.md).
const openChunk = `local path, line, column = ...
local api = vim.api
local function is_terminal(win)
  return vim.bo[api.nvim_win_get_buf(win)].buftype == "terminal"
end
local function is_normal(win)
  return api.nvim_win_get_config(win).relative == "" and not is_terminal(win)
end
local current = api.nvim_get_current_win()
local target = current
if is_terminal(current) then
  target = nil
  local previous = vim.fn.win_getid(vim.fn.winnr("#"))
  if previous ~= 0 and previous ~= current and is_normal(previous) then
    target = previous
  else
    for _, win in ipairs(api.nvim_tabpage_list_wins(0)) do
      if is_normal(win) then
        target = win
        break
      end
    end
  end
end
local cmd = "edit"
if target == nil then
  cmd = "split"
else
  api.nvim_set_current_win(target)
  if vim.bo[api.nvim_win_get_buf(target)].modified and not vim.o.hidden then
    cmd = "split"
  end
end
api.nvim_cmd({ cmd = cmd, args = { path }, magic = { file = false, bar = false } }, {})
local win = api.nvim_get_current_win()
local row = math.min(line, api.nvim_buf_line_count(0))
local text = api.nvim_buf_get_lines(0, row - 1, row, false)[1] or ""
local col = math.max(math.min(column - 1, #text - 1), 0)
api.nvim_win_set_cursor(win, { row, col })
`

// minimumVersion is the oldest supported Neovim (research R-01).
var minimumVersion = [3]int64{0, 11, 7}

// OpenInParent opens loc in the Neovim instance listening at endpoint
// (F014-FR-003). The caller has checked the endpoint with CheckEndpoint.
// It refuses a peer that is not Neovim or is older than 0.11.7, and opens
// nothing then.
func OpenInParent(endpoint string, loc Location) error {
	client, err := Dial(endpoint, time.Now().Add(overallTimeout))
	if err != nil {
		return err
	}
	defer client.Close()
	info, err := client.Call("nvim_get_api_info")
	if err != nil {
		return callFailure(err)
	}
	version, ok := apiVersion(info)
	if !ok {
		return &EndpointError{Reason: ReasonNotNeovim}
	}
	if less3(version, minimumVersion) {
		return &EndpointError{Reason: ReasonVersionUnsupported}
	}
	if _, err := client.Call("nvim_exec_lua", openChunk, []any{loc.Abs, loc.Line, loc.Column}); err != nil {
		return callFailure(err)
	}
	return nil
}

func callFailure(err error) error {
	var callErr *CallError
	if errors.As(err, &callErr) {
		return &EndpointError{Reason: ReasonEditorError, Message: callErr.Message}
	}
	return err
}

// apiVersion reads version.major/minor/patch from nvim_get_api_info's
// [channel, metadata] result.
func apiVersion(info any) ([3]int64, bool) {
	var version [3]int64
	pair, ok := info.([]any)
	if !ok || len(pair) != 2 {
		return version, false
	}
	metadata, ok := pair[1].(map[string]any)
	if !ok {
		return version, false
	}
	fields, ok := metadata["version"].(map[string]any)
	if !ok {
		return version, false
	}
	for i, name := range []string{"major", "minor", "patch"} {
		n, ok := fields[name].(int64)
		if !ok {
			return version, false
		}
		version[i] = n
	}
	return version, true
}

func less3(a, b [3]int64) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// ForegroundArgs is the argument vector of a foreground nvim (research
// R-04): the two validated integers are the only Ex text, and "--" makes
// the path a file name even when it starts with "-" or "+".
func ForegroundArgs(loc Location) []string {
	return []string{fmt.Sprintf("+call cursor(%d, %d)", loc.Line, loc.Column), "--", loc.Abs}
}
