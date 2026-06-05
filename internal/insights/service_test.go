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

func TestActivityWithRunningDynamics(t *testing.T) {
	client := &fakeIntervals{
		activity: &intervals.Activity{ID: "a1", Type: "Run"},
		streams: []intervals.ActivityStream{
			{Type: "GarminGCT", Data: []*float64{new(float64(200)), nil, new(float64(220))}},
			{Type: "GarminVO", Data: []*float64{new(float64(8)), new(float64(10))}},
		},
	}
	service := New(client)

	detail, err := service.Activity(context.Background(), ActivityArgs{ID: "a1", IncludeRunningDynamics: true})
	if err != nil {
		t.Fatal(err)
	}
	if detail.RunningDynamics == nil || !detail.RunningDynamics.Available {
		t.Fatalf("RunningDynamics = %#v", detail.RunningDynamics)
	}
	if got := detail.RunningDynamics.GroundContactTimeMs; got == nil || *got != 210 {
		t.Fatalf("GroundContactTimeMs = %v, want 210", got)
	}
	if !client.streamsCalled {
		t.Fatal("expected GetActivityStreams to be called")
	}
}

func TestActivityWithRunningDynamicsFromActivityFields(t *testing.T) {
	gct := 278.5
	vo := 7.7
	vr := 7.91
	vo2 := 43.9
	client := &fakeIntervals{
		activity: &intervals.Activity{
			ID:                  "a1",
			Type:                "Run",
			GCT:                 &gct,
			VerticalOscillation: &vo,
			VerticalRatio:       &vr,
			VO2MaxGarmin:        &vo2,
		},
		streams: nil,
	}
	service := New(client)

	detail, err := service.Activity(context.Background(), ActivityArgs{ID: "a1", IncludeRunningDynamics: true})
	if err != nil {
		t.Fatal(err)
	}
	if detail.RunningDynamics == nil || !detail.RunningDynamics.Available {
		t.Fatalf("RunningDynamics = %#v", detail.RunningDynamics)
	}
	if got := detail.RunningDynamics.GroundContactTimeMs; got == nil || *got != 278.5 {
		t.Fatalf("GroundContactTimeMs = %v, want 278.5", got)
	}
	if got := detail.RunningDynamics.VerticalOscillationCm; got == nil || *got != 7.7 {
		t.Fatalf("VerticalOscillationCm = %v, want 7.7", got)
	}
	if got := detail.RunningDynamics.VerticalRatioPct; got == nil || *got != 7.91 {
		t.Fatalf("VerticalRatioPct = %v, want 7.91", got)
	}
	if got := detail.RunningDynamics.VO2MaxGarmin; got == nil || *got != 43.9 {
		t.Fatalf("VO2MaxGarmin = %v, want 43.9", got)
	}
	if !client.streamsCalled {
		t.Fatal("expected GetActivityStreams to be called for stream fallback")
	}
}

func TestActivityWithIntervalRunningDynamics(t *testing.T) {
	client := &fakeIntervals{
		activity: &intervals.Activity{
			ID:   "a1",
			Type: "Run",
			Intervals: []any{
				map[string]any{"id": float64(42), "type": "WORK", "start_index": float64(0), "end_index": float64(2)},
			},
		},
		streams: []intervals.ActivityStream{
			{Type: "GarminGCT", Data: []*float64{new(float64(200)), new(float64(220))}},
		},
	}
	service := New(client)

	detail, err := service.Activity(context.Background(), ActivityArgs{ID: "a1", IncludeIntervals: true, IncludeRunningDynamics: true})
	if err != nil {
		t.Fatal(err)
	}
	if !client.includeIntervals {
		t.Fatal("expected GetActivity to include intervals")
	}
	if len(detail.IntervalRunningDynamics) != 1 {
		t.Fatalf("IntervalRunningDynamics len = %d, want 1", len(detail.IntervalRunningDynamics))
	}
	record := detail.IntervalRunningDynamics[0]
	if record.IntervalID == nil || *record.IntervalID != 42 {
		t.Fatalf("IntervalID = %v, want 42", record.IntervalID)
	}
	if got := record.RunningDynamics.GroundContactTimeMs; got == nil || *got != 210 {
		t.Fatalf("GroundContactTimeMs = %v, want 210", got)
	}
}

func TestActivityIntervalRunningDynamicsRequiresBothFlags(t *testing.T) {
	client := &fakeIntervals{
		activity: &intervals.Activity{
			ID:        "a1",
			Type:      "Run",
			Intervals: []any{map[string]any{"start_index": float64(0), "end_index": float64(2)}},
		},
		streams: []intervals.ActivityStream{
			{Type: "GarminGCT", Data: []*float64{new(float64(200)), new(float64(220))}},
		},
	}
	service := New(client)

	withDynamics, err := service.Activity(context.Background(), ActivityArgs{ID: "a1", IncludeRunningDynamics: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(withDynamics.IntervalRunningDynamics) != 0 {
		t.Fatalf("IntervalRunningDynamics = %#v, want empty without include_intervals", withDynamics.IntervalRunningDynamics)
	}

	client.streamsCalled = false
	withIntervals, err := service.Activity(context.Background(), ActivityArgs{ID: "a1", IncludeIntervals: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(withIntervals.IntervalRunningDynamics) != 0 {
		t.Fatalf("IntervalRunningDynamics = %#v, want empty without include_running_dynamics", withIntervals.IntervalRunningDynamics)
	}
	if client.streamsCalled {
		t.Fatal("did not expect streams to be fetched without include_running_dynamics")
	}
}

func TestActivityWithoutRunningDynamics(t *testing.T) {
	client := &fakeIntervals{activity: &intervals.Activity{ID: "a1", Type: "Run"}}
	service := New(client)

	detail, err := service.Activity(context.Background(), ActivityArgs{ID: "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if detail.RunningDynamics != nil {
		t.Fatalf("RunningDynamics = %#v, want nil", detail.RunningDynamics)
	}
	if client.streamsCalled {
		t.Fatal("did not expect GetActivityStreams to be called")
	}
}

func TestActivityRunningDynamicsAbsent(t *testing.T) {
	client := &fakeIntervals{
		activity: &intervals.Activity{ID: "a1", Type: "Run"},
		streams:  nil,
	}
	service := New(client)

	detail, err := service.Activity(context.Background(), ActivityArgs{ID: "a1", IncludeRunningDynamics: true})
	if err != nil {
		t.Fatal(err)
	}
	if detail.RunningDynamics == nil || detail.RunningDynamics.Available {
		t.Fatalf("RunningDynamics = %#v, want unavailable", detail.RunningDynamics)
	}
	if detail.RunningDynamics.Note == "" {
		t.Fatal("expected absence note")
	}
}

type fakeIntervals struct {
	athlete          *intervals.Athlete
	activities       []intervals.Activity
	activity         *intervals.Activity
	streams          []intervals.ActivityStream
	streamsCalled    bool
	includeIntervals bool
	wellness         *intervals.Wellness
	summaries        []intervals.Summary
	events           []intervals.Event
}

func (f *fakeIntervals) GetAthlete(context.Context) (*intervals.Athlete, error) {
	return f.athlete, nil
}

func (f *fakeIntervals) ListActivities(context.Context, string, string, int) ([]intervals.Activity, error) {
	return f.activities, nil
}

func (f *fakeIntervals) GetActivity(_ context.Context, _ string, includeIntervals bool) (*intervals.Activity, error) {
	f.includeIntervals = includeIntervals
	if f.activity != nil {
		return f.activity, nil
	}
	return &f.activities[0], nil
}

func (f *fakeIntervals) GetActivityStreams(context.Context, string, []string) ([]intervals.ActivityStream, error) {
	f.streamsCalled = true
	return f.streams, nil
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
