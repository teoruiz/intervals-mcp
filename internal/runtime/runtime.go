package runtime

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/teoruiz/intervals-mcp/internal/config"
	"github.com/teoruiz/intervals-mcp/internal/insights"
	"github.com/teoruiz/intervals-mcp/internal/intervals"
	"github.com/teoruiz/intervals-mcp/internal/mcpserver"
)

func NewIntervalsClient(cfg config.Config, httpClient *http.Client) (*intervals.Client, error) {
	return intervals.NewClient(intervals.Config{
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
	return mcpserver.New(service), nil
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
