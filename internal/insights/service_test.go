package insights

import (
	"context"
	"testing"
	"time"

	"github.com/teoruiz/intervals-mcp/internal/intervals"
)

func TestTodayContextCombinesData(t *testing.T) {
	calories := 700
	carbsUsed := 90
	carbsIngested := 20
	load := 80
	readiness := 72.5
	eventID := 12
	client := &fakeIntervals{
		athlete: &intervals.Athlete{ID: "i123", Timezone: "Europe/Madrid"},
		activities: []intervals.Activity{{
			ID:            "a1",
			Name:          "Morning Ride",
			Type:          "Ride",
			Calories:      &calories,
			CarbsUsed:     &carbsUsed,
			CarbsIngested: &carbsIngested,
			TrainingLoad:  &load,
		}},
		wellness:  &intervals.Wellness{ID: "2026-06-01", Readiness: &readiness},
		summaries: []intervals.Summary{{Date: "2026-06-01"}},
		events:    []intervals.Event{{ID: &eventID, Name: "Easy run", Category: "WORKOUT"}},
	}
	service := New(client)
	service.now = func() time.Time { return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) }

	got, err := service.TodayContext(context.Background(), TodayArgs{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Date != "2026-06-01" {
		t.Fatalf("Date = %q", got.Date)
	}
	if got.LastActivity == nil || got.LastActivity.ID != "a1" {
		t.Fatalf("LastActivity = %#v", got.LastActivity)
	}
	if got.Nutrition.NetCarbsUsedEstimateGrams != 70 {
		t.Fatalf("NetCarbsUsedEstimateGrams = %d", got.Nutrition.NetCarbsUsedEstimateGrams)
	}
}

type fakeIntervals struct {
	athlete    *intervals.Athlete
	activities []intervals.Activity
	activity   *intervals.Activity
	wellness   *intervals.Wellness
	summaries  []intervals.Summary
	events     []intervals.Event
}

func (f *fakeIntervals) GetAthlete(context.Context) (*intervals.Athlete, error) {
	return f.athlete, nil
}

func (f *fakeIntervals) ListActivities(context.Context, string, string, int) ([]intervals.Activity, error) {
	return f.activities, nil
}

func (f *fakeIntervals) GetActivity(context.Context, string, bool) (*intervals.Activity, error) {
	if f.activity != nil {
		return f.activity, nil
	}
	return &f.activities[0], nil
}

func (f *fakeIntervals) GetWellness(context.Context, string) (*intervals.Wellness, error) {
	return f.wellness, nil
}

func (f *fakeIntervals) GetAthleteSummary(context.Context, string, string) ([]intervals.Summary, error) {
	return f.summaries, nil
}

func (f *fakeIntervals) ListEvents(context.Context, string, string, []string, int) ([]intervals.Event, error) {
	return f.events, nil
}

func (f *fakeIntervals) GetEvent(context.Context, int) (*intervals.Event, error) {
	if len(f.events) == 0 {
		return nil, nil
	}
	return &f.events[0], nil
}
