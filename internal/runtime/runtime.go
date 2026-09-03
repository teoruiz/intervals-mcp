package runtime

import (
	"log/slog"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/teoruiz/intervals-mcp/internal/config"
	"github.com/teoruiz/intervals-mcp/internal/insights"
	"github.com/teoruiz/intervals-mcp/internal/intervals"
	"github.com/teoruiz/intervals-mcp/internal/mcpserver"
)

// Option configures the constructed Intervals client. Existing callers that
// need no options keep their current call shape.
type Option func(*options)

type options struct {
	logger *slog.Logger
}

// WithLogger reports Intervals.icu rate limit budget through logger.
func WithLogger(logger *slog.Logger) Option {
	return func(o *options) { o.logger = logger }
}

func newOptions(opts []Option) options {
	var resolved options
	for _, opt := range opts {
		opt(&resolved)
	}
	return resolved
}

func NewIntervalsClient(cfg config.Config, httpClient *http.Client, opts ...Option) (*intervals.Client, error) {
	resolved := newOptions(opts)
	return intervals.NewClient(intervals.Config{
		BaseURL:    cfg.IntervalsBaseURL,
		APIKey:     cfg.IntervalsAPIKey,
		AthleteID:  cfg.IntervalsAthleteID,
		HTTPClient: httpClient,
		Timeout:    cfg.RequestTimeout,
		Logger:     resolved.logger,
	})
}

func NewInsightsService(cfg config.Config, httpClient *http.Client, opts ...Option) (*insights.Service, error) {
	intervalsClient, err := NewIntervalsClient(cfg, httpClient, opts...)
	if err != nil {
		return nil, err
	}
	return insights.New(intervalsClient), nil
}

func NewMCPServer(cfg config.Config, httpClient *http.Client, opts ...Option) (*mcp.Server, error) {
	service, err := NewInsightsService(cfg, httpClient, opts...)
	if err != nil {
		return nil, err
	}
	return mcpserver.New(service), nil
}

func NewMCPHTTPHandler(cfg config.Config, httpClient *http.Client, opts ...Option) (http.Handler, error) {
	server, err := NewMCPServer(cfg, httpClient, opts...)
	if err != nil {
		return nil, err
	}
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{Stateless: true}), nil
}
