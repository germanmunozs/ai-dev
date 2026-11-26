package watch

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func (h *harness) typesAfterSnapshot(t *testing.T) []Type {
	t.Helper()
	return h.out.types(t)[1:]
}

func TestAgentsConfigChanged(t *testing.T) {
	for _, path := range []string{"/p/.ai-dev", "/p/.ai-dev/agents.json"} {
		t.Run(path, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newHarness()
				stop := h.running(t, defaultOptions())
				h.source.update(func(s *fakeSource) { s.setup = "configuration_invalid" })
				h.notify(path)
				h.settle()
				events := h.out.events(t)
				if len(events) != 2 || events[1].Type != AgentsConfigChanged {
					t.Fatalf("events %v, want agents_config_changed", h.out.types(t))
				}
				var d SetupChange
				if err := json.Unmarshal(events[1].Data, &d); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(d.Setup), `"outcome":"configuration_invalid"`) {
					t.Fatalf("setup %s, want the source's configuration_invalid bytes", d.Setup)
				}
				h.notify(path) // unchanged: nothing
				h.settle()
				if n := len(h.out.events(t)); n != 2 {
					t.Fatalf("%d events after an unchanged notification", n)
				}
				stop()
			})
		})
	}
}

func TestEnvironmentRevalidation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		h.source.update(func(s *fakeSource) {
			s.checks = map[string]string{"git-tool": "pass", "coding-agent": "pass", "old-check": "pass"}
		})
		options := defaultOptions()
		options.Heartbeat = 300 * time.Second
		stop := h.running(t, options)
		h.source.update(func(s *fakeSource) {
			s.checks = map[string]string{"git-tool": "pass", "coding-agent": "fail", "new-check": "pass"}
			s.setup = "not_configured"
		})
		time.Sleep(59 * time.Second)
		synctest.Wait()
		if got := h.typesAfterSnapshot(t); len(got) != 0 {
			t.Fatalf("events %v before the 60s tick", got)
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if got := fmt.Sprint(h.typesAfterSnapshot(t)); got != fmt.Sprint([]Type{EnvironmentChanged, AgentsConfigChanged}) {
			t.Fatalf("events %s at the tick", got)
		}
		var d EnvironmentChange
		if err := json.Unmarshal(h.out.events(t)[1].Data, &d); err != nil {
			t.Fatal(err)
		}
		got := map[string]string{}
		for _, c := range d.Checks {
			got[c.ID] = str(c.From) + ">" + str(c.To) + ":" + c.Remediation
		}
		want := map[string]string{"coding-agent": "pass>fail:fix coding-agent", "new-check": "<nil>>pass:fix new-check", "old-check": "pass><nil>:"}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("checks %v, want %v", got, want)
		}
		if !strings.Contains(string(d.Environment), `"overall":"not_ready"`) {
			t.Errorf("environment %s", d.Environment)
		}
		time.Sleep(60 * time.Second) // nothing changed since
		synctest.Wait()
		if n := len(h.typesAfterSnapshot(t)); n != 2 {
			t.Fatalf("%d events after an unchanged tick", n)
		}
		stop()
	})
}

func TestEnvironmentIntervalZero(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness()
		options := defaultOptions()
		options.EnvironmentInterval = 0
		options.Heartbeat = 300 * time.Second
		stop := h.running(t, options)
		h.source.update(func(s *fakeSource) { s.checks = map[string]string{"git-tool": "fail"} })
		time.Sleep(10 * time.Minute)
		synctest.Wait()
		for _, typ := range h.typesAfterSnapshot(t) {
			if typ != Heartbeat {
				t.Fatalf("event %s with revalidation disabled", typ)
			}
		}
		stop()
	})
}

func TestHeartbeat(t *testing.T) {
	for _, every := range []time.Duration{heartbeatDefault, 5 * time.Second} {
		t.Run(every.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newHarness()
				options := defaultOptions()
				options.Heartbeat = every
				options.EnvironmentInterval = 0
				stop := h.running(t, options)
				time.Sleep(3*every + every/2)
				synctest.Wait()
				events := h.out.events(t)[1:]
				if len(events) != 3 {
					t.Fatalf("%d events in %v, want 3 heartbeats", len(events), 3*every+every/2)
				}
				for _, e := range events {
					if e.Type != Heartbeat || string(e.Data) != "{}" {
						t.Fatalf("event %s %s, want heartbeat {}", e.Type, e.Data)
					}
				}
				stop()
			})
		})
	}
}
