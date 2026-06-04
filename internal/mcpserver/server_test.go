package mcpserver

import (
	"context"
	"testing"

	"github.com/teoruiz/intervals-mcp/internal/insights"
	"github.com/teoruiz/intervals-mcp/internal/intervals"
)

func TestNewRegistersToolsWithoutPanic(t *testing.T) {
	service := insights.New(&fakeIntervals{})
	defer func() {
		if err := recover(); err != nil {
			t.Fatalf("New() panicked: %v", err)
		}
	}()
	_ = New(service)
}

type fakeIntervals struct{}

func (f *fakeIntervals) GetAthlete(context.Context) (*intervals.Athlete, error) {
	return &intervals.Athlete{ID: "i123", Timezone: "UTC"}, nil
}

func (f *fakeIntervals) ListActivities(context.Context, string, string, int) ([]intervals.Activity, error) {
	return nil, nil
}

func (f *fakeIntervals) GetActivity(context.Context, string, bool) (*intervals.Activity, error) {
	return &intervals.Activity{ID: "a1"}, nil
}

func (f *fakeIntervals) GetActivityStreams(context.Context, string, []string) ([]intervals.ActivityStream, error) {
	return nil, nil
}

func (f *fakeIntervals) GetWellness(context.Context, string) (*intervals.Wellness, error) {
	return nil, nil
}

func (f *fakeIntervals) GetAthleteSummary(context.Context, string, string) ([]intervals.Summary, error) {
	return nil, nil
}

func (f *fakeIntervals) ListEvents(context.Context, string, string, []string, int) ([]intervals.Event, error) {
	return nil, nil
}

func (f *fakeIntervals) GetEvent(context.Context, int) (*intervals.Event, error) {
	return nil, nil
}
