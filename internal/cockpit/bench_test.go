package cockpit

import (
	"fmt"
	"testing"
	"time"
)

func fiftyFeatures(t testing.TB) Model {
	source := defaultSource()
	source.rows = nil
	for i := range 50 {
		source.rows = append(source.rows, featureRow(fmt.Sprintf("%03d-feature-with-a-longer-name", i+1), "tasked"))
	}
	m, _ := newLoaded(t, source)
	m, _ = press(t, m, "tab")
	return m
}

// navigate is one navigation key: Update plus View, as Bubble Tea does.
func navigate(m Model, name string) Model {
	m, _ = update(m, key(name))
	_ = m.View()
	return m
}

// TestNavigationIsFast proves SC-001: with 50 features, Update plus View per
// navigation key stays under 50 ms (R-07).
func TestNavigationIsFast(t *testing.T) {
	m := fiftyFeatures(t)
	keys := []string{"j", "j", "k", "down", "up", "enter", "esc", "tab", "shift+tab"}
	const rounds = 20
	start := time.Now()
	for range rounds {
		for _, name := range keys {
			m = navigate(m, name)
		}
	}
	perKey := time.Since(start) / time.Duration(rounds*len(keys))
	t.Logf("Update plus View per key with 50 features: %v", perKey)
	if perKey >= 50*time.Millisecond {
		t.Fatalf("%v per key, want under 50ms", perKey)
	}
}

func BenchmarkNavigation(b *testing.B) {
	m := fiftyFeatures(b)
	for i := 0; b.Loop(); i++ {
		m = navigate(m, []string{"j", "k"}[i%2])
	}
}
