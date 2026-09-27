package main

import (
	"context"
	"os"

	"ai-dev/internal/app"
	"ai-dev/internal/system"
)

func main() {
	code := app.Run(context.Background(), os.Args[1:], system.OSExecutor{}, os.DirFS("."), os.Stdout, os.Stderr)
	os.Exit(code)
}
