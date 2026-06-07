package insights

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/teoruiz/intervals-mcp/internal/domain"
)

type Service struct {
	repo Repository
	now  func() time.Time
}

func New(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

func (s *Service) TodayContext(ctx context.Context, args TodayArgs) (TodayContext, error) {
	athlete, date, tz := s.resolveDate(ctx, args.Date)
	activities, err := s.repo.ListActivities(ctx, domain.ActivityQuery{Oldest: date, Newest: date, Limit: 20})
	if err != nil {
		return TodayContext{}, fmt.Errorf("list today's activities: %w", err)
	}
	recovery, err := s.repo.GetRecovery(ctx, domain.LocalDate(date))
	if err != nil {
		return TodayContext{}, fmt.Errorf("get recovery: %w", err)
	}
	summary, err := s.summaryForDate(ctx, date)
	if err != nil {
		return TodayContext{}, err
	}
	events, err := s.repo.ListEvents(ctx, domain.EventQuery{Oldest: date, Newest: date, Limit: 20})
	if err != nil {
		return TodayContext{}, fmt.Errorf("list calendar: %w", err)
	}

	var last *domain.Activity
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
	activities, err := s.repo.ListActivities(ctx, domain.ActivityQuery{Oldest: oldest, Newest: newest, Limit: limit})
	if err != nil {
		return ActivitiesContext{}, fmt.Errorf("list recent activities: %w", err)
	}
	return ActivitiesContext{Activities: activities}, nil
}

func (s *Service) Activity(ctx context.Context, args ActivityArgs) (*ActivityDetail, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, errors.New("id is required")
	}
	activity, err := s.repo.GetActivity(ctx, domain.ActivityID(args.ID), domain.ActivityDetailOptions{IncludeIntervals: args.IncludeIntervals})
	if err != nil {
		return nil, fmt.Errorf("get activity: %w", err)
	}
	detail := &ActivityDetail{Activity: activity}
	if args.IncludeRunningDynamics {
		dynamics := domain.RunningDynamicsFromActivity(activity)
		streams, err := s.repo.GetActivityRunningDynamicsStreams(ctx, domain.ActivityID(args.ID))
		if err != nil {
			return nil, fmt.Errorf("get activity streams: %w", err)
		}
		dynamics = domain.MergeRunningDynamics(dynamics, domain.AggregateRunningDynamics(streams))
		detail.RunningDynamics = &dynamics
		if args.IncludeIntervals {
			detail.IntervalRunningDynamics = domain.AggregateIntervalRunningDynamics(activity.Intervals, streams)
		}
	}
	return detail, nil
}

func (s *Service) Recovery(ctx context.Context, args RecoveryArgs) (RecoveryContext, error) {
	_, date, _ := s.resolveDate(ctx, args.Date)
	recovery, err := s.repo.GetRecovery(ctx, domain.LocalDate(date))
	if err != nil {
		return RecoveryContext{}, fmt.Errorf("get wellness: %w", err)
	}
	summary, err := s.summaryForDate(ctx, date)
	if err != nil {
		return RecoveryContext{}, err
	}
	return RecoveryContext{
		Date:     date,
		Recovery: recovery,
		Summary:  summary,
		Notes:    availabilityNotes(recovery, summary, nil, nil),
	}, nil
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
	events, err := s.repo.ListEvents(ctx, domain.EventQuery{Oldest: oldest, Newest: newest, Categories: args.Categories, Limit: 100})
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

	activities, err := s.repo.ListActivities(ctx, domain.ActivityQuery{Oldest: oldest, Newest: newest, Limit: 50})
	if err != nil {
		return SearchResult{}, fmt.Errorf("search activities: %w", err)
	}
	events, err := s.repo.ListEvents(ctx, domain.EventQuery{Oldest: today, Newest: upcoming, Limit: 50})
	if err != nil {
		return SearchResult{}, fmt.Errorf("search events: %w", err)
	}
	recovery, err := s.repo.GetRecovery(ctx, domain.LocalDate(today))
	if err != nil {
		return SearchResult{}, fmt.Errorf("search recovery: %w", err)
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
		activity, err := s.repo.GetActivity(ctx, domain.ActivityID(id), domain.ActivityDetailOptions{})
		if err != nil {
			return FetchResult{}, fmt.Errorf("fetch activity: %w", err)
		}
		record := activityRecord(*activity)
		return FetchResult{ID: record.ID, Title: record.Title, Text: record.Text, Record: activity}, nil
	case "recovery":
		recovery, err := s.repo.GetRecovery(ctx, domain.LocalDate(id))
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
		event, err := s.repo.GetEvent(ctx, domain.EventID(eventID))
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

func (s *Service) resolveDate(ctx context.Context, explicit string) (*domain.Athlete, string, string) {
	athlete, _ := s.repo.GetAthlete(ctx)
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

func (s *Service) summaryForDate(ctx context.Context, date string) (*domain.AthleteSummary, error) {
	summaries, err := s.repo.GetAthleteSummary(ctx, domain.DateRange{Start: date, End: date})
	if err != nil {
		return nil, fmt.Errorf("get athlete summary: %w", err)
	}
	if len(summaries) == 0 {
		return nil, nil
	}
	return &summaries[0], nil
}

func nutritionContext(activities []domain.Activity) NutritionContext {
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

func availabilityNotes(recovery *domain.Recovery, summary *domain.AthleteSummary, activities []domain.Activity, events []domain.CalendarEvent) []string {
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

func activityRecord(activity domain.Activity) SearchRecord {
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

func eventRecord(event domain.CalendarEvent) SearchRecord {
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

func recoveryText(date string, recovery *domain.Recovery) string {
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
