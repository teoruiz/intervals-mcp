package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/teoruiz/intervals-mcp/internal/insights"
	"github.com/teoruiz/intervals-mcp/internal/intervals"
)

func TestParseGlobalsAnywhere(t *testing.T) {
	opts, args, err := ParseGlobals([]string{"activities", "--json", "--limit", "5", "--env", "local.env"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.JSON {
		t.Fatal("JSON = false")
	}
	if opts.EnvPath != "local.env" {
		t.Fatalf("EnvPath = %q", opts.EnvPath)
	}
	if !opts.EnvExplicit {
		t.Fatal("EnvExplicit = false")
	}
	want := []string{"activities", "--limit", "5"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %#v", args)
	}
}

func TestTodayJSON(t *testing.T) {
	service := &fakeService{
		today: insights.TodayContext{Date: "2026-06-02"},
	}
	var out bytes.Buffer
	app := New(service, Options{JSON: true, NoStyle: true, Out: &out})

	if err := app.Run(context.Background(), []string{"today"}); err != nil {
		t.Fatal(err)
	}
	if !service.todayCalled {
		t.Fatal("TodayContext was not called")
	}
	if !strings.Contains(out.String(), `"date": "2026-06-02"`) {
		t.Fatalf("output = %s", out.String())
	}
}

func TestActivitiesRoutesFlagsAndWritesTable(t *testing.T) {
	load := 42
	service := &fakeService{
		activities: insights.ActivitiesContext{Activities: []intervals.Activity{{
			ID:             "a1",
			Name:           "Morning Ride",
			Type:           "Ride",
			StartDateLocal: "2026-06-01T08:00:00",
			TrainingLoad:   &load,
		}}},
	}
	var out bytes.Buffer
	app := New(service, Options{NoStyle: true, Out: &out})

	err := app.Run(context.Background(), []string{
		"activities",
		"--oldest", "2026-05-01",
		"--newest", "2026-06-01",
		"--limit", "2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if service.recentArgs.Oldest != "2026-05-01" || service.recentArgs.Newest != "2026-06-01" || service.recentArgs.Limit != 2 {
		t.Fatalf("recentArgs = %#v", service.recentArgs)
	}
	if !strings.Contains(out.String(), "DATE") || !strings.Contains(out.String(), "Morning Ride") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestActivityAllowsIntervalsAfterID(t *testing.T) {
	service := &fakeService{
		activity: &intervals.Activity{ID: "abc", Name: "Workout"},
	}
	var out bytes.Buffer
	app := New(service, Options{NoStyle: true, Out: &out})

	if err := app.Run(context.Background(), []string{"activity", "abc", "--intervals"}); err != nil {
		t.Fatal(err)
	}
	if service.activityArgs.ID != "abc" || !service.activityArgs.IncludeIntervals {
		t.Fatalf("activityArgs = %#v", service.activityArgs)
	}
}

func TestCalendarParsesRepeatedAndCommaCategories(t *testing.T) {
	service := &fakeService{calendar: insights.CalendarContext{
		Oldest: "2026-06-01",
		Newest: "2026-06-08",
	}}
	var out bytes.Buffer
	app := New(service, Options{NoStyle: true, Out: &out})

	err := app.Run(context.Background(), []string{
		"calendar",
		"--category", "WORKOUT,NOTES",
		"--category", "EVENT",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(service.calendarArgs.Categories, ",")
	if got != "WORKOUT,NOTES,EVENT" {
		t.Fatalf("categories = %q", got)
	}
}

func TestSearchJoinsQueryWords(t *testing.T) {
	service := &fakeService{search: insights.SearchResult{Query: "morning ride"}}
	var out bytes.Buffer
	app := New(service, Options{NoStyle: true, Out: &out})

	if err := app.Run(context.Background(), []string{"search", "morning", "ride"}); err != nil {
		t.Fatal(err)
	}
	if service.searchArgs.Query != "morning ride" {
		t.Fatalf("Query = %q", service.searchArgs.Query)
	}
}

func TestRecoveryRejectsInvalidDate(t *testing.T) {
	service := &fakeService{}
	app := New(service, Options{NoStyle: true, Out: ioDiscard{}})

	if err := app.Run(context.Background(), []string{"recovery", "--date", "2026/06/01"}); err == nil {
		t.Fatal("Run() succeeded with invalid date")
	}
}

type fakeService struct {
	today      insights.TodayContext
	activities insights.ActivitiesContext
	activity   *intervals.Activity
	recovery   insights.RecoveryContext
	calendar   insights.CalendarContext
	search     insights.SearchResult

	todayCalled  bool
	recentArgs   insights.RecentActivitiesArgs
	activityArgs insights.ActivityArgs
	recoveryArgs insights.RecoveryArgs
	calendarArgs insights.CalendarArgs
	searchArgs   insights.SearchArgs
}

func (f *fakeService) TodayContext(context.Context, insights.TodayArgs) (insights.TodayContext, error) {
	f.todayCalled = true
	return f.today, nil
}

func (f *fakeService) RecentActivities(_ context.Context, args insights.RecentActivitiesArgs) (insights.ActivitiesContext, error) {
	f.recentArgs = args
	return f.activities, nil
}

func (f *fakeService) Activity(_ context.Context, args insights.ActivityArgs) (*intervals.Activity, error) {
	f.activityArgs = args
	return f.activity, nil
}

func (f *fakeService) Recovery(_ context.Context, args insights.RecoveryArgs) (insights.RecoveryContext, error) {
	f.recoveryArgs = args
	return f.recovery, nil
}

func (f *fakeService) Calendar(_ context.Context, args insights.CalendarArgs) (insights.CalendarContext, error) {
	f.calendarArgs = args
	return f.calendar, nil
}

func (f *fakeService) Search(_ context.Context, args insights.SearchArgs) (insights.SearchResult, error) {
	f.searchArgs = args
	return f.search, nil
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}
