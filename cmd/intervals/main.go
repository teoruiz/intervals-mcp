package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/teoruiz/intervals-mcp/internal/cli"
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
	return cli.Run(ctx, cli.RunOptions{
		Args:       args,
		BinaryName: filepath.Base(os.Args[0]),
		In:         os.Stdin,
		Out:        os.Stdout,
		ErrOut:     os.Stderr,
	})
}
