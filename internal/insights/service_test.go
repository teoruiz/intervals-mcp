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

//go:fix inline
func ptrFloat(v float64) *float64 { return new(v) }

func TestActivityWithRunningDynamics(t *testing.T) {
	client := &fakeIntervals{
		activity: &intervals.Activity{ID: "a1", Type: "Run"},
		streams: []intervals.ActivityStream{
			{Type: "GarminGCT", Data: []*float64{ptrFloat(200), nil, ptrFloat(220)}},
			{Type: "GarminVO", Data: []*float64{ptrFloat(8), ptrFloat(10)}},
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
	athlete       *intervals.Athlete
	activities    []intervals.Activity
	activity      *intervals.Activity
	streams       []intervals.ActivityStream
	streamsCalled bool
	wellness      *intervals.Wellness
	summaries     []intervals.Summary
	events        []intervals.Event
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
