package runtime

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/teoruiz/intervals-mcp/internal/adapters/intervalsicu"
	mcpadapter "github.com/teoruiz/intervals-mcp/internal/adapters/mcp"
	"github.com/teoruiz/intervals-mcp/internal/application/insights"
	"github.com/teoruiz/intervals-mcp/internal/platform/config"
)

func NewIntervalsClient(cfg config.Config, httpClient *http.Client) (*intervalsicu.Client, error) {
	return intervalsicu.NewClient(intervalsicu.Config{
		BaseURL:    cfg.IntervalsBaseURL,
		APIKey:     cfg.IntervalsAPIKey,
		AthleteID:  cfg.IntervalsAthleteID,
		HTTPClient: httpClient,
		Timeout:    cfg.RequestTimeout,
	})
}

func NewInsightsService(cfg config.Config, httpClient *http.Client) (*insights.Service, error) {
	intervalsClient, err := NewIntervalsClient(cfg, httpClient)
	if err != nil {
		return nil, err
	}
	return insights.New(intervalsClient), nil
}

func NewMCPServer(cfg config.Config, httpClient *http.Client) (*mcp.Server, error) {
	service, err := NewInsightsService(cfg, httpClient)
	if err != nil {
		return nil, err
	}
	return mcpadapter.New(service), nil
}

func NewMCPHTTPHandler(cfg config.Config, httpClient *http.Client) (http.Handler, error) {
	server, err := NewMCPServer(cfg, httpClient)
	if err != nil {
		return nil, err
	}
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{Stateless: true}), nil
}
