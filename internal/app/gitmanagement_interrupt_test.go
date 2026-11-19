//go:build (darwin && !ios) || (linux && !android)

package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// T121/INT-01: the issuing apply of a real initialization with an initial
// commit is interrupted with SIGINT, SIGTERM or SIGHUP, sent to its whole process
// group as a terminal does, before and after every Git invocation and
// every operation's audit events. Each interrupt must leave:
//
//   - a process that exits on its own with a valid result: a refusal with
//     reason interrupted and the authorization still issued before the
//     mutation intent, and after it either the completed result or a
//     terminal result with reason interrupted whose later operations were
//     not started;
//   - no Git process killed in the middle of an operation, so no
//     index.lock, config.lock or reference lock in the project;
//   - exactly the durable state the result reports, and a later
//     consumption that runs nothing again.
func TestRealEngineInterrupts(t *testing.T) {
	if testing.Short() {
		t.Skip("re-executes the test binary once per interrupt point")
	}
	baseline := newCrashCase(t)
	trace := filepath.Join(baseline.f.base, "trace")
	if state, output := baseline.child(t, "", trace); !state.Success() {
		t.Fatalf("baseline child %v\n%s", state, output)
	}
	data, err := os.ReadFile(trace)
	if err != nil {
		t.Fatal(err)
	}
	// Interrupt points: issuance, the apply lock and the claim CAS before
	// the claim; every Git invocation and operation event after it. lastStart
	// is the position of the last operation's start: an interrupt from there
	// on leaves no operation to stop.
	labels := strings.Fields(string(data))
	var points []string
	position, lastStart := map[string]int{}, 0
	for i, label := range labels {
		if strings.Contains(label, ":operation_started") {
			lastStart = i
		}
		if strings.HasPrefix(label, "git-") || strings.Contains(label, ":operation_") || strings.Contains(label, ":mutation_intent") ||
			strings.HasPrefix(label, "IssueAuthorization#") || strings.HasPrefix(label, "AcquireApplyLock#") || label == "TransitionAuthorization#1" {
			points = append(points, label+":before", label+":after")
			position[label] = i
		}
	}
	var mu sync.Mutex
	outcomes := map[string]int{}
	if len(points) < 20 {
		t.Fatalf("only %d interrupt points in %s", len(points), data)
	}
	t.Run("points", func(t *testing.T) {
		for _, signal := range []string{"SIGINT", "SIGTERM", "SIGHUP"} {
			for _, point := range points {
				t.Run(signal+" "+point, func(t *testing.T) {
					t.Parallel()
					c := newCrashCase(t)
					c.issue = append(c.issue, "--format", "json")
					out := filepath.Join(c.f.base, "result.json")
					state, output := c.child(t, point, "", crashSignalEnv+"="+signal, crashOutEnv+"="+out)
					if !state.Exited() {
						t.Fatalf("the interrupted child did not exit on its own: %v\n%s", state, output)
					}
					data, err := os.ReadFile(out)
					if err != nil {
						t.Fatalf("no result: %v\n%s", err, output)
					}
					var result struct {
						Result struct {
							Status     string   `json:"status"`
							Reason     string   `json:"reason"`
							State      string   `json:"durable_authorization_state"`
							NotStarted []string `json:"not_started_operation_ids"`
						} `json:"result"`
					}
					if err := json.Unmarshal(data, &result); err != nil {
						t.Fatalf("result %q: %v", data, err)
					}
					r := result.Result
					var code int
					if _, err := fmt.Sscanf(output[strings.Index(output, "child exit "):], "child exit %d", &code); err != nil {
						t.Fatalf("no apply exit code: %v\n%s", err, output)
					}
					validApplyResult(t, "interrupted apply", code, string(data))
					label := point[:strings.LastIndex(point, ":")]
					mu.Lock()
					outcomes[r.Status]++
					mu.Unlock()
					switch {
					case r.Status == "applied" && position[label] < lastStart:
						t.Fatalf("an interrupt before the last operation started was ignored")
					case r.Status == "applied":
					case r.Reason != "interrupted":
						t.Fatalf("an interrupt produced %s/%s", r.Status, r.Reason)
					case r.Status == "refused" && r.State != "issued":
						t.Fatalf("an interrupt before the intent left the authorization %s", r.State)
					case r.Status != "refused" && len(r.NotStarted) == 0:
						t.Fatalf("an interrupt after the intent reported no operation not started: %+v", r)
					}
					for _, lock := range []string{"index.lock", "config.lock", "HEAD.lock", "refs/heads/main.lock"} {
						if _, err := os.Lstat(filepath.Join(c.f.project, ".git", lock)); err == nil {
							t.Fatalf("the interrupt left .git/%s", lock)
						}
					}
					if f := c.f.terminalClosures(t); r.Status != "refused" && f != 1 {
						t.Fatalf("%d terminal closures", f)
					}

					// A later consumption runs nothing again, except a clean
					// refusal before the intent, which left the authorization
					// usable.
					counter := &countingRunner{GitRunner: c.f.deps.Runner}
					c.f.deps.Runner = counter
					before := projectDigest(t, c.f.project)
					laterCode, stdout, _ := c.f.run("apply", "--plan", c.plan, "--confirm-plan", c.digest, "--authorization-id", crashAuthorizationID, "--format", "json")
					later := validApplyResult(t, "later consumption", laterCode, stdout)
					if r.Status == "refused" {
						if later.Result.Status != "applied" {
							t.Fatalf("an authorization left usable by an interrupt was not applied: %+v", later.Result)
						}
						return
					}
					if len(counter.runs) != 0 || projectDigest(t, c.f.project) != before || later.Result.Status == "applied" {
						t.Fatalf("a later consumption ran %v: %+v", counter.runs, later.Result)
					}
				})
			}
		}
	})
	// Both sides of the claim were reached: a clean refusal that left the
	// authorization usable, and a mutation stopped with reason interrupted.
	if outcomes["refused"] == 0 || outcomes["partially_applied"]+outcomes["state_indeterminate"]+outcomes["failed"] == 0 {
		t.Fatalf("outcomes %v do not cover both sides of the claim", outcomes)
	}
	t.Logf("outcomes %v", outcomes)
}

// R-03: a second interrupt after a deferred one exits at once with 130,
// so a hung operation can still be stopped. That is the crash path: the
// attempt stays unclosed, nothing runs again, and reconcile reports it.
func TestRealEngineSecondInterruptExits(t *testing.T) {
	for _, signal := range []string{"SIGINTx2", "SIGTERMx2"} {
		t.Run(signal, func(t *testing.T) {
			c := newCrashCase(t)
			state, output := c.child(t, "git-update-ref#1:before", "", crashSignalEnv+"="+signal)
			if state.ExitCode() != 130 || !strings.Contains(output, "second interrupt") {
				t.Fatalf("exit %d\n%s", state.ExitCode(), output)
			}
			checkCrashRecovery(t, c)
		})
	}
}
