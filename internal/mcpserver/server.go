package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/teoruiz/intervals-mcp/internal/insights"
	"github.com/teoruiz/intervals-mcp/internal/intervals"
)

func New(service *insights.Service) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "intervals-mcp",
		Version: "0.1.0",
	}, &mcp.ServerOptions{
		Instructions: "Read-only access to one Intervals.icu athlete's activities, recovery, and planned workouts.",
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "today_context",
		Description: "Get today's activities, recovery, athlete summary, planned events, and fueling context.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args insights.TodayArgs) (*mcp.CallToolResult, insights.TodayContext, error) {
		return toolResult(service.TodayContext(ctx, args))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_recent_activities",
		Description: "List recent Intervals.icu activities in descending date order.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args insights.RecentActivitiesArgs) (*mcp.CallToolResult, insights.ActivitiesContext, error) {
		return toolResult(service.RecentActivities(ctx, args))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_activity",
		Description: "Get details for one Intervals.icu activity.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args insights.ActivityArgs) (*mcp.CallToolResult, *intervals.Activity, error) {
		return toolResult(service.Activity(ctx, args))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_recovery",
		Description: "Get wellness and fitness summary data for a date.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args insights.RecoveryArgs) (*mcp.CallToolResult, insights.RecoveryContext, error) {
		return toolResult(service.Recovery(ctx, args))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_calendar",
		Description: "List planned workouts, notes, and other calendar events.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args insights.CalendarArgs) (*mcp.CallToolResult, insights.CalendarContext, error) {
		return toolResult(service.Calendar(ctx, args))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search",
		Description: "Search recent activities, today's recovery, and upcoming planned events.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args insights.SearchArgs) (*mcp.CallToolResult, insights.SearchResult, error) {
		return toolResult(service.Search(ctx, args))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "fetch",
		Description: "Fetch a record returned by search.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args insights.FetchArgs) (*mcp.CallToolResult, insights.FetchResult, error) {
		return toolResult(service.Fetch(ctx, args))
	})

	return server
}

func toolResult[T any](value T, err error) (*mcp.CallToolResult, T, error) {
	return nil, value, err
}
