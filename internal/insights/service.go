package insights

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/teoruiz/intervals-mcp/internal/intervals"
)

type IntervalsClient interface {
	GetAthlete(context.Context) (*intervals.Athlete, error)
	ListActivities(context.Context, string, string, int) ([]intervals.Activity, error)
	GetActivity(context.Context, string, bool) (*intervals.Activity, error)
	GetActivityStreams(context.Context, string, []string) ([]intervals.ActivityStream, error)
	GetWellness(context.Context, string) (*intervals.Wellness, error)
	ListWellness(context.Context, string, string) ([]intervals.Wellness, error)
	GetAthleteSummary(context.Context, string, string) ([]intervals.Summary, error)
	ListEvents(context.Context, string, string, []string, int) ([]intervals.Event, error)
	GetEvent(context.Context, int) (*intervals.Event, error)
}

type Service struct {
	client IntervalsClient
	now    func() time.Time
}

func New(client IntervalsClient) *Service {
	return &Service{client: client, now: time.Now}
}

func (s *Service) TodayContext(ctx context.Context, args TodayArgs) (TodayContext, error) {
	athlete, date, tz := s.resolveDate(ctx, args.Date)

	var (
		activities []intervals.Activity
		recovery   *intervals.Wellness
		summary    *intervals.Summary
		events     []intervals.Event
	)
	if err := gather(ctx,
		fetch(&activities, "list today's activities", func(ctx context.Context) ([]intervals.Activity, error) {
			return s.client.ListActivities(ctx, date, date, 20)
		}),
		fetch(&recovery, "get recovery", func(ctx context.Context) (*intervals.Wellness, error) {
			return s.client.GetWellness(ctx, date)
		}),
		fetch(&summary, "get athlete summary", func(ctx context.Context) (*intervals.Summary, error) {
			return s.summaryForDate(ctx, date)
		}),
		fetch(&events, "list calendar", func(ctx context.Context) ([]intervals.Event, error) {
			return s.client.ListEvents(ctx, date, date, nil, 20)
		}),
	); err != nil {
		return TodayContext{}, err
	}

	var last *intervals.Activity
	if len(activities) > 0 {
		last = &activities[0]
	}
	notes := availabilityNotes(recovery, summary, activities, events)
	return TodayContext{
		Date:          date,
		Timezone:      tz,
		Athlete:       athlete,
		Recovery:      recovery,
		Summary:       summary,
		Activities:    activities,
		LastActivity:  last,
		PlannedEvents: events,
		Nutrition:     nutritionContext(activities),
		Notes:         notes,
	}, nil
}

func (s *Service) RecentActivities(ctx context.Context, args RecentActivitiesArgs) (ActivitiesContext, error) {
	_, today, _ := s.resolveDate(ctx, "")
	limit := clamp(args.Limit, 10, 1, 50)
	newest := args.Newest
	if newest == "" {
		newest = today
	}
	oldest := args.Oldest
	if oldest == "" {
		t, err := time.Parse(time.DateOnly, today)
		if err == nil {
			oldest = t.AddDate(0, 0, -14).Format(time.DateOnly)
		} else {
			oldest = today
		}
	}
	activities, err := s.client.ListActivities(ctx, oldest, newest, limit)
	if err != nil {
		return ActivitiesContext{}, fmt.Errorf("list recent activities: %w", err)
	}
	return ActivitiesContext{Activities: activities}, nil
}

func (s *Service) Activity(ctx context.Context, args ActivityArgs) (*ActivityDetail, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, errors.New("id is required")
	}
	var (
		activity *intervals.Activity
		streams  []intervals.ActivityStream
	)
	tasks := []func(context.Context) error{
		fetch(&activity, "get activity", func(ctx context.Context) (*intervals.Activity, error) {
			return s.client.GetActivity(ctx, args.ID, args.IncludeIntervals)
		}),
	}
	// Streams are keyed by the same id, so they do not wait on the activity.
	if args.IncludeRunningDynamics {
		tasks = append(tasks, fetch(&streams, "get activity streams", func(ctx context.Context) ([]intervals.ActivityStream, error) {
			return s.client.GetActivityStreams(ctx, args.ID, intervals.RunningDynamicsStreamTypes())
		}))
	}
	if err := gather(ctx, tasks...); err != nil {
		return nil, err
	}

	detail := &ActivityDetail{Activity: activity}
	if args.IncludeRunningDynamics {
		dynamics := intervals.RunningDynamicsFromActivity(activity)
		dynamics = intervals.MergeRunningDynamics(dynamics, intervals.AggregateRunningDynamics(streams))
		detail.RunningDynamics = &dynamics
		if args.IncludeIntervals {
			detail.IntervalRunningDynamics = intervals.AggregateIntervalRunningDynamics(activity.Intervals, streams)
		}
	}
	return detail, nil
}

func (s *Service) Recovery(ctx context.Context, args RecoveryArgs) (RecoveryContext, error) {
	_, date, _ := s.resolveDate(ctx, args.Date)

	var (
		recovery *intervals.Wellness
		summary  *intervals.Summary
	)
	if err := gather(ctx,
		fetch(&recovery, "get wellness", func(ctx context.Context) (*intervals.Wellness, error) {
			return s.client.GetWellness(ctx, date)
		}),
		fetch(&summary, "get athlete summary", func(ctx context.Context) (*intervals.Summary, error) {
			return s.summaryForDate(ctx, date)
		}),
	); err != nil {
		return RecoveryContext{}, err
	}
	return RecoveryContext{
		Date:     date,
		Recovery: recovery,
		Summary:  summary,
		Notes:    availabilityNotes(recovery, summary, nil, nil),
	}, nil
}

func (s *Service) WellnessRange(ctx context.Context, args WellnessRangeArgs) (WellnessRangeContext, error) {
	_, today, _ := s.resolveDate(ctx, "")
	newest := args.Newest
	if newest == "" {
		newest = today
	}
	oldest := args.Oldest
	if oldest == "" {
		t, err := time.Parse(time.DateOnly, newest)
		if err == nil {
			oldest = t.AddDate(0, 0, -28).Format(time.DateOnly)
		} else {
			oldest = newest
		}
	}
	records, err := s.client.ListWellness(ctx, oldest, newest)
	if err != nil {
		return WellnessRangeContext{}, fmt.Errorf("list wellness: %w", err)
	}
	var notes []string
	if len(records) == 0 {
		notes = append(notes, "No wellness records were found for this date range.")
	}
	return WellnessRangeContext{Oldest: oldest, Newest: newest, Records: records, Notes: notes}, nil
}

func (s *Service) Calendar(ctx context.Context, args CalendarArgs) (CalendarContext, error) {
	_, today, _ := s.resolveDate(ctx, "")
	oldest := args.Oldest
	if oldest == "" {
		oldest = today
	}
	newest := args.Newest
	if newest == "" {
		t, err := time.Parse(time.DateOnly, oldest)
		if err == nil {
			newest = t.AddDate(0, 0, 7).Format(time.DateOnly)
		} else {
			newest = oldest
		}
	}
	events, err := s.client.ListEvents(ctx, oldest, newest, args.Categories, 100)
	if err != nil {
		return CalendarContext{}, fmt.Errorf("list calendar: %w", err)
	}
	return CalendarContext{Oldest: oldest, Newest: newest, Events: events}, nil
}

func (s *Service) Search(ctx context.Context, args SearchArgs) (SearchResult, error) {
	_, today, _ := s.resolveDate(ctx, "")
	needle := strings.ToLower(strings.TrimSpace(args.Query))

	start, _ := time.Parse(time.DateOnly, today)
	oldest := today
	newest := today
	upcoming := today
	if !start.IsZero() {
		oldest = start.AddDate(0, 0, -30).Format(time.DateOnly)
		upcoming = start.AddDate(0, 0, 14).Format(time.DateOnly)
	}

	var (
		activities []intervals.Activity
		events     []intervals.Event
		recovery   *intervals.Wellness
	)
	if err := gather(ctx,
		fetch(&activities, "search activities", func(ctx context.Context) ([]intervals.Activity, error) {
			return s.client.ListActivities(ctx, oldest, newest, 50)
		}),
		fetch(&events, "search events", func(ctx context.Context) ([]intervals.Event, error) {
			return s.client.ListEvents(ctx, today, upcoming, nil, 50)
		}),
		fetch(&recovery, "search recovery", func(ctx context.Context) (*intervals.Wellness, error) {
			return s.client.GetWellness(ctx, today)
		}),
	); err != nil {
		return SearchResult{}, err
	}

	var records []SearchRecord
	if recovery != nil {
		record := SearchRecord{
			ID:       "recovery:" + today,
			Title:    "Recovery for " + today,
			Text:     recoveryText(today, recovery),
			Date:     today,
			Kind:     "recovery",
			Metadata: recovery,
		}
		if matches(record, needle) {
			records = append(records, record)
		}
	}
	for _, activity := range activities {
		record := activityRecord(activity)
		if matches(record, needle) {
			records = append(records, record)
		}
	}
	for _, event := range events {
		record := eventRecord(event)
		if record.ID != "" && matches(record, needle) {
			records = append(records, record)
		}
	}
	return SearchResult{Query: args.Query, Results: records}, nil
}

func (s *Service) Fetch(ctx context.Context, args FetchArgs) (FetchResult, error) {
	kind, id, ok := strings.Cut(args.ID, ":")
	if !ok || id == "" {
		return FetchResult{}, errors.New("id must be a search record id")
	}
	switch kind {
	case "activity":
		activity, err := s.client.GetActivity(ctx, id, false)
		if err != nil {
			return FetchResult{}, fmt.Errorf("fetch activity: %w", err)
		}
		record := activityRecord(*activity)
		return FetchResult{ID: record.ID, Title: record.Title, Text: record.Text, Record: activity}, nil
	case "recovery":
		recovery, err := s.client.GetWellness(ctx, id)
		if err != nil {
			return FetchResult{}, fmt.Errorf("fetch recovery: %w", err)
		}
		return FetchResult{
			ID:     args.ID,
			Title:  "Recovery for " + id,
			Text:   recoveryText(id, recovery),
			Record: recovery,
		}, nil
	case "event":
		eventID, err := strconv.Atoi(id)
		if err != nil {
			return FetchResult{}, fmt.Errorf("invalid event id: %w", err)
		}
		event, err := s.client.GetEvent(ctx, eventID)
		if err != nil {
			return FetchResult{}, fmt.Errorf("fetch event: %w", err)
		}
		if event == nil {
			return FetchResult{}, errors.New("event not found")
		}
		record := eventRecord(*event)
		return FetchResult{ID: record.ID, Title: record.Title, Text: record.Text, Record: event}, nil
	default:
		return FetchResult{}, fmt.Errorf("unsupported search record kind %q", kind)
	}
}

func (s *Service) resolveDate(ctx context.Context, explicit string) (*intervals.Athlete, string, string) {
	athlete, _ := s.client.GetAthlete(ctx)
	location := time.Local
	timezone := ""
	if athlete != nil && athlete.Timezone != "" {
		if loc, err := time.LoadLocation(athlete.Timezone); err == nil {
			location = loc
			timezone = athlete.Timezone
		}
	}
	if explicit != "" {
		return athlete, explicit, timezone
	}
	return athlete, s.now().In(location).Format(time.DateOnly), timezone
}

func (s *Service) summaryForDate(ctx context.Context, date string) (*intervals.Summary, error) {
	summaries, err := s.client.GetAthleteSummary(ctx, date, date)
	if err != nil {
		return nil, err
	}
	if len(summaries) == 0 {
		return nil, nil
	}
	return &summaries[0], nil
}

func nutritionContext(activities []intervals.Activity) NutritionContext {
	var calories, carbsUsed, carbsIngested, load int
	for _, activity := range activities {
		calories += intValue(activity.Calories)
		carbsUsed += intValue(activity.CarbsUsed)
		carbsIngested += intValue(activity.CarbsIngested)
		load += intValue(activity.TrainingLoad)
	}
	net := max(carbsUsed-carbsIngested, 0)
	context := "No activity fueling data is available for today."
	if len(activities) > 0 {
		context = "Use this as factual workout and fueling context; the model should account for hunger, normal dietary needs, and any user preferences."
	}
	return NutritionContext{
		TotalCaloriesBurned:       calories,
		TotalCarbsUsedGrams:       carbsUsed,
		TotalCarbsIngestedGrams:   carbsIngested,
		NetCarbsUsedEstimateGrams: net,
		TotalTrainingLoad:         load,
		Context:                   context,
	}
}

func availabilityNotes(recovery *intervals.Wellness, summary *intervals.Summary, activities []intervals.Activity, events []intervals.Event) []string {
	var notes []string
	if recovery == nil {
		notes = append(notes, "No wellness record was available for this date.")
	}
	if summary == nil {
		notes = append(notes, "No athlete summary was available for this date.")
	}
	if activities != nil && len(activities) == 0 {
		notes = append(notes, "No activities were found for this date.")
	}
	if events != nil && len(events) == 0 {
		notes = append(notes, "No planned events were found for this date.")
	}
	return notes
}

func activityRecord(activity intervals.Activity) SearchRecord {
	title := activity.Name
	if title == "" {
		title = strings.TrimSpace(activity.Type + " " + activity.StartDateLocal)
	}
	return SearchRecord{
		ID:    "activity:" + activity.ID,
		Title: title,
		Text:  fmt.Sprintf("%s on %s. Type: %s. Load: %d. Calories: %d.", title, activity.StartDateLocal, activity.Type, intValue(activity.TrainingLoad), intValue(activity.Calories)),
		Date:  datePrefix(activity.StartDateLocal),
		Kind:  "activity",
		Metadata: map[string]any{
			"type":          activity.Type,
			"training_load": activity.TrainingLoad,
			"calories":      activity.Calories,
		},
	}
}

func eventRecord(event intervals.Event) SearchRecord {
	if event.ID == nil {
		return SearchRecord{}
	}
	title := event.Name
	if title == "" {
		title = strings.TrimSpace(event.Category + " " + event.StartDateLocal)
	}
	return SearchRecord{
		ID:    fmt.Sprintf("event:%d", *event.ID),
		Title: title,
		Text:  fmt.Sprintf("%s on %s. Category: %s. Type: %s.", title, event.StartDateLocal, event.Category, event.Type),
		Date:  datePrefix(event.StartDateLocal),
		Kind:  "event",
		Metadata: map[string]any{
			"category": event.Category,
			"type":     event.Type,
		},
	}
}

func recoveryText(date string, recovery *intervals.Wellness) string {
	if recovery == nil {
		return "No recovery record available for " + date + "."
	}
	return fmt.Sprintf("Recovery for %s. Readiness: %s. HRV: %s. Resting HR: %s. Sleep seconds: %s.",
		date,
		floatText(recovery.Readiness),
		floatText(recovery.HRV),
		intText(recovery.RestingHR),
		intText(recovery.SleepSecs),
	)
}

func matches(record SearchRecord, needle string) bool {
	if needle == "" {
		return true
	}
	haystack := strings.ToLower(record.ID + " " + record.Title + " " + record.Text + " " + record.Date + " " + record.Kind)
	return strings.Contains(haystack, needle)
}

func clamp(value, fallback, minValue, maxValue int) int {
	if value == 0 {
		value = fallback
	}
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func intValue(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func intText(v *int) string {
	if v == nil {
		return "unknown"
	}
	return strconv.Itoa(*v)
}

func floatText(v *float64) string {
	if v == nil {
		return "unknown"
	}
	return strconv.FormatFloat(*v, 'f', 1, 64)
}

func datePrefix(value string) string {
	if len(value) >= len(time.DateOnly) {
		return value[:len(time.DateOnly)]
	}
	return value
}
