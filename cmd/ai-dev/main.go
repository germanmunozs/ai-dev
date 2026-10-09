package main

import (
	"context"
	"os"
	"os/signal"

	"ai-dev/internal/app"
	"ai-dev/internal/gatestore"
	"ai-dev/internal/system"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	code := app.RunWithGateStore(ctx, os.Args[1:], system.OSExecutor{}, system.OSLauncher{}, os.DirFS("."), gatestore.New("."), os.Stdin, os.Stdout, os.Stderr)
	os.Exit(code)
}
