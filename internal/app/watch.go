package app

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/germanmunozs/ai-dev/internal/doctor"
	"github.com/germanmunozs/ai-dev/internal/fswatch"
	"github.com/germanmunozs/ai-dev/internal/report"
	"github.com/germanmunozs/ai-dev/internal/sdd"
	"github.com/germanmunozs/ai-dev/internal/system"
	"github.com/germanmunozs/ai-dev/internal/watch"
	"github.com/germanmunozs/ai-dev/internal/workflow"
)

const watchUsage = "Usage: ai-dev watch [--format text|json] [--feature <id>]... [--debounce <duration>] [--heartbeat <duration>] [--environment-interval <duration>] [--once]"

// RunWatch implements "ai-dev watch" (Feature 015): it reports each change
// to the project as one event, and writes nothing. It uses only the
// project's read-only file system and the executor (F015-FR-019).
func RunWatch(ctx context.Context, args []string, deps Dependencies) int {
	options, format, ok := parseWatchOptions(args)
	if !ok {
		fmt.Fprintln(deps.Stderr, watchUsage)
		return 2
	}
	release := brokenPipeAsError()
	defer release()
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	encode := watch.EncodeJSON
	if format == "text" {
		encode = func(e watch.Event, seq int64) []byte { return watch.EncodeText(e, seq, report.ShellInertQuote) }
	}
	project, identity := watchProject()
	result := watch.Run(ctx, options, watch.Config{
		Source:     newWatchSource(deps.Root, deps.Executor),
		NewWatcher: fswatch.New,
		Root:       deps.Root,
		Project:    project,
		Output:     deps.Stdout,
		Encode:     encode,
		Identity:   identity,
	})
	return result.ExitCode
}

// watchProject returns the project's absolute path, with symbolic links
// resolved, and a function that fails once that path no longer names the
// same directory (project_gone). The adapters never follow a link, so a
// project reached through one is observed at its real path (review F4).
func watchProject() (string, func() (string, error)) {
	project, err := os.Getwd()
	if err == nil {
		project, err = filepath.EvalSymlinks(project)
	}
	if err != nil {
		return ".", func() (string, error) { return "", err }
	}
	initial, initialErr := os.Stat(project)
	return project, func() (string, error) {
		if initialErr != nil {
			return "", initialErr
		}
		current, err := os.Stat(project)
		if err != nil {
			return "", err
		}
		if !os.SameFile(initial, current) {
			return "", fmt.Errorf("%s now names another directory", project)
		}
		return project, nil
	}
}

// parseWatchOptions validates the options of contracts/watch.md.
func parseWatchOptions(args []string) (watch.Options, string, bool) {
	flags := newFlagSet("watch")
	format := flags.String("format", "text", "")
	var features repeated
	flags.Var(&features, "feature", "")
	debounce := flags.Duration("debounce", 250*time.Millisecond, "")
	heartbeat := flags.Duration("heartbeat", 15*time.Second, "")
	environment := flags.Duration("environment-interval", 60*time.Second, "")
	once := flags.Bool("once", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || (*format != "text" && *format != "json") {
		return watch.Options{}, "", false
	}
	within := func(d, low, high time.Duration) bool { return d >= low && d <= high }
	if !within(*debounce, 50*time.Millisecond, 5*time.Second) || !within(*heartbeat, 5*time.Second, 300*time.Second) ||
		*environment != 0 && !within(*environment, 5*time.Second, 3600*time.Second) {
		return watch.Options{}, "", false
	}
	for _, id := range features {
		if _, ok := workflow.FeatureDir(id); !ok || id == "" {
			return watch.Options{}, "", false
		}
	}
	return watch.Options{Features: features, Debounce: *debounce, Heartbeat: *heartbeat,
		EnvironmentInterval: *environment, Once: *once}, *format, true
}

// newWatchSource computes each result with the function its command uses
// (F015-FR-006); it receives only the project's read-only file system and
// the executor.
func newWatchSource(root fs.FS, executor system.Executor) watch.Source {
	return watchSource{root: root, executor: executor}
}

// watchSource renders each result as its command's JSON, so ai-dev watch
// carries the commands' own bytes (research R-03).
type watchSource struct {
	root     fs.FS
	executor system.Executor
}

// Features lists the features as the cockpit does, except that a specs that
// is not a real, readable directory (a file, a symbolic link, unreadable)
// holds no features, as if it were absent, so watch keeps running
// (review F13). An unreadable project is still an error.
func (s watchSource) Features() ([]string, error) {
	if _, err := fs.Stat(s.root, "."); err != nil {
		return nil, fmt.Errorf("the project directory cannot be read: %v", err)
	}
	if l, ok := s.root.(fs.ReadLinkFS); ok {
		info, err := l.Lstat("specs")
		if err != nil || !info.IsDir() {
			return nil, nil
		}
	}
	if _, err := fs.ReadDir(s.root, "specs"); err != nil {
		return nil, nil
	}
	return listFeatures(s.root)
}

func (s watchSource) State(id string) watch.StateResult {
	r := workflow.EvaluateState(s.root, id)
	var out bytes.Buffer
	if err := report.WriteWorkflowStateJSON(&out, r); err != nil {
		panic(fmt.Sprintf("workflow state report cannot be rendered for feature %q: %v", id, err))
	}
	return watch.StateResult{JSON: out.Bytes(), State: string(r.State), Recognized: r.Outcome == workflow.StateReported, Outcome: string(r.Outcome)}
}

func (s watchSource) Next(id string) []byte {
	r := sdd.Evaluate(s.root, id)
	var out bytes.Buffer
	if !r.Valid() {
		panic(fmt.Sprintf("workflow next report cannot be rendered for feature %q", id))
	}
	if err := report.WriteWorkflowNextJSON(&out, r); err != nil {
		panic(fmt.Sprintf("workflow next report cannot be rendered for feature %q: %v", id, err))
	}
	return out.Bytes()
}

func (s watchSource) Setup() []byte {
	var out bytes.Buffer
	if err := report.WriteSetupJSON(&out, setupShowReport(s.executor, s.root)); err != nil {
		panic(fmt.Sprintf("setup report cannot be rendered: %v", err))
	}
	return out.Bytes()
}

func (s watchSource) Environment(ctx context.Context) watch.EnvironmentResult {
	r := RunDoctor(ctx, s.executor, s.root)
	var out bytes.Buffer
	if err := report.WriteJSON(&out, r); err != nil {
		panic(fmt.Sprintf("doctor report cannot be rendered: %v", err))
	}
	result := watch.EnvironmentResult{JSON: out.Bytes()}
	for _, check := range r.Checks {
		status := "pass"
		if check.Status == doctor.Fail {
			status = "fail"
		}
		result.Checks = append(result.Checks, watch.CheckStatus{ID: check.ID, Status: status, Remediation: check.Remediation})
	}
	return result
}
