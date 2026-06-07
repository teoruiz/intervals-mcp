package runtime

import (
	"context"
	"net/http"

	"github.com/teoruiz/intervals-mcp/internal/adapters/cli"
	"github.com/teoruiz/intervals-mcp/internal/platform/config"
)

type CLIRunOptions = cli.RunOptions

func RunCLI(ctx context.Context, opts CLIRunOptions) error {
	opts.NewService = func(cfg config.Config, httpClient *http.Client) (cli.Service, error) {
		return NewInsightsService(cfg, httpClient)
	}
	opts.NewMCPServer = NewMCPServer
	opts.NewIntervalsClient = func(cfg config.Config, httpClient *http.Client) (cli.IntervalsClient, error) {
		return NewIntervalsClient(cfg, httpClient)
	}
	return cli.Run(ctx, opts)
}
