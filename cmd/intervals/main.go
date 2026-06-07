package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	appruntime "github.com/teoruiz/intervals-mcp/internal/platform/runtime"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(1)
	}
}

func run(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return appruntime.RunCLI(ctx, appruntime.CLIRunOptions{
		Args:       args,
		BinaryName: filepath.Base(os.Args[0]),
		In:         os.Stdin,
		Out:        os.Stdout,
		ErrOut:     os.Stderr,
	})
}
