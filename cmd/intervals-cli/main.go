package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/teoruiz/intervals-mcp/internal/cli"
	"github.com/teoruiz/intervals-mcp/internal/config"
	"github.com/teoruiz/intervals-mcp/internal/insights"
	"github.com/teoruiz/intervals-mcp/internal/intervals"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "intervals-cli: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	global, commandArgs, err := cli.ParseGlobals(args)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var service cli.Service
	if cli.NeedsService(commandArgs) {
		cfg, err := config.LoadIntervals(global.EnvPath)
		if err != nil {
			return err
		}
		httpClient := &http.Client{Timeout: cfg.RequestTimeout}
		intervalsClient, err := intervals.NewClient(intervals.Config{
			BaseURL:    cfg.IntervalsBaseURL,
			APIKey:     cfg.IntervalsAPIKey,
			AthleteID:  cfg.IntervalsAthleteID,
			HTTPClient: httpClient,
			Timeout:    cfg.RequestTimeout,
		})
		if err != nil {
			return err
		}
		service = insights.New(intervalsClient)
	}

	app := cli.New(service, cli.Options{
		JSON:   global.JSON,
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	})
	return app.Run(ctx, commandArgs)
}
