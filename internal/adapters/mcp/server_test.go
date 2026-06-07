package mcp

import (
	"context"
	"testing"

	"github.com/teoruiz/intervals-mcp/internal/application/insights"
	"github.com/teoruiz/intervals-mcp/internal/domain"
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

func (f *fakeIntervals) GetAthlete(context.Context) (*domain.Athlete, error) {
	return &domain.Athlete{ID: "i123", Timezone: "UTC"}, nil
}

func (f *fakeIntervals) ListActivities(context.Context, domain.ActivityQuery) ([]domain.Activity, error) {
	return nil, nil
}

func (f *fakeIntervals) GetActivity(context.Context, domain.ActivityID, domain.ActivityDetailOptions) (*domain.Activity, error) {
	return &domain.Activity{ID: "a1"}, nil
}

func (f *fakeIntervals) GetActivityRunningDynamicsStreams(context.Context, domain.ActivityID) ([]domain.ActivityStream, error) {
	return nil, nil
}

func (f *fakeIntervals) GetRecovery(context.Context, domain.LocalDate) (*domain.Recovery, error) {
	return nil, nil
}

func (f *fakeIntervals) GetAthleteSummary(context.Context, domain.DateRange) ([]domain.AthleteSummary, error) {
	return nil, nil
}

func (f *fakeIntervals) ListEvents(context.Context, domain.EventQuery) ([]domain.CalendarEvent, error) {
	return nil, nil
}

func (f *fakeIntervals) GetEvent(context.Context, domain.EventID) (*domain.CalendarEvent, error) {
	return nil, nil
}
