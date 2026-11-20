package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/germanmunozs/ai-dev/internal/app"
	"github.com/germanmunozs/ai-dev/internal/gatestore"
	"github.com/germanmunozs/ai-dev/internal/system"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	code := app.RunWithDependencies(ctx, os.Args[1:], app.Dependencies{
		Executor:  system.OSExecutor{},
		Launcher:  system.OSLauncher{},
		Root:      os.DirFS("."),
		GateStore: gatestore.New("."),
		Git:       app.DefaultGitManagementDependencies(),
		Stdin:     os.Stdin,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
	})
	os.Exit(code)
}
