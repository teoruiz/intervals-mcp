package mcp

import (
	"context"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/teoruiz/intervals-mcp/internal/application/insights"
)

func New(service *insights.Service) *mcpsdk.Server {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "intervals-mcp",
		Version: "0.1.0",
	}, &mcpsdk.ServerOptions{
		Instructions: "Read-only access to one Intervals.icu athlete's activities, recovery, and planned workouts.",
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "today_context",
		Description: "Get today's activities, recovery, athlete summary, planned events, and fueling context.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, args insights.TodayArgs) (*mcpsdk.CallToolResult, insights.TodayContext, error) {
		return toolResult(service.TodayContext(ctx, args))
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "list_recent_activities",
		Description: "List recent Intervals.icu activities in descending date order.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, args insights.RecentActivitiesArgs) (*mcpsdk.CallToolResult, insights.ActivitiesContext, error) {
		return toolResult(service.RecentActivities(ctx, args))
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "get_activity",
		Description: "Get details for one Intervals.icu activity. Set include_running_dynamics to add Garmin running-dynamics averages (ground contact time, vertical oscillation, etc.); combine it with include_intervals to add interval-aligned running dynamics when source interval fields or streams exist.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, args insights.ActivityArgs) (*mcpsdk.CallToolResult, *insights.ActivityDetail, error) {
		return toolResult(service.Activity(ctx, args))
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "get_recovery",
		Description: "Get wellness and fitness summary data for a date.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, args insights.RecoveryArgs) (*mcpsdk.CallToolResult, insights.RecoveryContext, error) {
		return toolResult(service.Recovery(ctx, args))
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "list_calendar",
		Description: "List planned workouts, notes, and other calendar events.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, args insights.CalendarArgs) (*mcpsdk.CallToolResult, insights.CalendarContext, error) {
		return toolResult(service.Calendar(ctx, args))
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "search",
		Description: "Search recent activities, today's recovery, and upcoming planned events.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, args insights.SearchArgs) (*mcpsdk.CallToolResult, insights.SearchResult, error) {
		return toolResult(service.Search(ctx, args))
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "fetch",
		Description: "Fetch a record returned by search.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, args insights.FetchArgs) (*mcpsdk.CallToolResult, insights.FetchResult, error) {
		return toolResult(service.Fetch(ctx, args))
	})

	return server
}

func toolResult[T any](value T, err error) (*mcpsdk.CallToolResult, T, error) {
	return nil, value, err
}
