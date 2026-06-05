package insights

import "github.com/teoruiz/intervals-mcp/internal/intervals"

type TodayArgs struct {
	Date string `json:"date,omitempty" jsonschema:"Optional local date in YYYY-MM-DD format. Defaults to today in the athlete timezone."`
}

type RecentActivitiesArgs struct {
	Oldest string `json:"oldest,omitempty" jsonschema:"Optional local start date or datetime. Defaults to 14 days ago."`
	Newest string `json:"newest,omitempty" jsonschema:"Optional local end date or datetime. Defaults to today."`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum number of activities to return. Defaults to 10, max 50."`
}

type ActivityArgs struct {
	ID                     string `json:"id" jsonschema:"Intervals.icu activity id."`
	IncludeIntervals       bool   `json:"include_intervals,omitempty" jsonschema:"Whether to include interval data for the activity."`
	IncludeRunningDynamics bool   `json:"include_running_dynamics,omitempty" jsonschema:"Whether to include Garmin running-dynamics averages from activity fields and stream fallback (ground contact time, vertical oscillation, etc.). Requires the matching community custom fields configured in Intervals.icu."`
}

// ActivityDetail is a single activity enriched with optional running dynamics.
// The embedded Activity keeps the JSON flat so existing consumers are unaffected.
type ActivityDetail struct {
	*intervals.Activity
	RunningDynamics *intervals.RunningDynamics `json:"running_dynamics,omitempty"`
}

type RecoveryArgs struct {
	Date string `json:"date,omitempty" jsonschema:"Optional local date in YYYY-MM-DD format. Defaults to today in the athlete timezone."`
}

type CalendarArgs struct {
	Oldest     string   `json:"oldest,omitempty" jsonschema:"Optional local start date. Defaults to today."`
	Newest     string   `json:"newest,omitempty" jsonschema:"Optional local end date. Defaults to seven days from oldest."`
	Categories []string `json:"categories,omitempty" jsonschema:"Optional Intervals event categories, for example WORKOUT or NOTES."`
}

type SearchArgs struct {
	Query string `json:"query,omitempty" jsonschema:"Optional search text. Empty returns recent activities, today's recovery, and upcoming events."`
}

type FetchArgs struct {
	ID string `json:"id" jsonschema:"A record id returned by search, such as activity:abc, recovery:2026-06-01, or event:123."`
}

type ActivitiesContext struct {
	Activities []intervals.Activity `json:"activities"`
}

type TodayContext struct {
	Date          string               `json:"date"`
	Timezone      string               `json:"timezone,omitempty"`
	Athlete       *intervals.Athlete   `json:"athlete,omitempty"`
	Recovery      *intervals.Wellness  `json:"recovery,omitempty"`
	Summary       *intervals.Summary   `json:"summary,omitempty"`
	Activities    []intervals.Activity `json:"activities"`
	LastActivity  *intervals.Activity  `json:"last_activity,omitempty"`
	PlannedEvents []intervals.Event    `json:"planned_events"`
	Nutrition     NutritionContext     `json:"nutrition_context"`
	Notes         []string             `json:"notes,omitempty"`
}

type NutritionContext struct {
	TotalCaloriesBurned       int    `json:"total_calories_burned,omitempty"`
	TotalCarbsUsedGrams       int    `json:"total_carbs_used_grams,omitempty"`
	TotalCarbsIngestedGrams   int    `json:"total_carbs_ingested_grams,omitempty"`
	NetCarbsUsedEstimateGrams int    `json:"net_carbs_used_estimate_grams,omitempty"`
	TotalTrainingLoad         int    `json:"total_training_load,omitempty"`
	Context                   string `json:"context"`
}

type RecoveryContext struct {
	Date     string              `json:"date"`
	Recovery *intervals.Wellness `json:"recovery,omitempty"`
	Summary  *intervals.Summary  `json:"summary,omitempty"`
	Notes    []string            `json:"notes,omitempty"`
}

type CalendarContext struct {
	Oldest string            `json:"oldest"`
	Newest string            `json:"newest"`
	Events []intervals.Event `json:"events"`
}

type SearchResult struct {
	Query   string         `json:"query,omitempty"`
	Results []SearchRecord `json:"results"`
}

type SearchRecord struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Text     string `json:"text"`
	Date     string `json:"date,omitempty"`
	Kind     string `json:"kind"`
	Metadata any    `json:"metadata,omitempty"`
}

type FetchResult struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Text   string `json:"text"`
	Record any    `json:"record,omitempty"`
}
