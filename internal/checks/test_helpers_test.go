package checks

import (
	"context"
	"errors"
)

type fakeExecutor struct {
	paths       map[string]string
	lookErrs    map[string]error
	stdout      string
	runErr      error
	runName     string
	runArgs     []string
	lookups     []string
	panicLookup bool
	panicRun    bool
}

func (f *fakeExecutor) LookPath(file string) (string, error) {
	if f.panicLookup {
		panic("lookup panic")
	}
	f.lookups = append(f.lookups, file)
	if err := f.lookErrs[file]; err != nil {
		return "", err
	}
	if path := f.paths[file]; path != "" {
		return path, nil
	}
	return "", errors.New("not found")
}

func (f *fakeExecutor) Run(_ context.Context, name string, args ...string) (string, error) {
	if f.panicRun {
		panic("run panic")
	}
	f.runName = name
	f.runArgs = append([]string(nil), args...)
	return f.stdout, f.runErr
}
