package insights

import (
	"context"

	"github.com/teoruiz/intervals-mcp/internal/domain"
)

type AthleteReader interface {
	GetAthlete(context.Context) (*domain.Athlete, error)
}

type ActivityReader interface {
	ListActivities(context.Context, domain.ActivityQuery) ([]domain.Activity, error)
	GetActivity(context.Context, domain.ActivityID, domain.ActivityDetailOptions) (*domain.Activity, error)
	GetActivityRunningDynamicsStreams(context.Context, domain.ActivityID) ([]domain.ActivityStream, error)
}

type RecoveryReader interface {
	GetRecovery(context.Context, domain.LocalDate) (*domain.Recovery, error)
	GetAthleteSummary(context.Context, domain.DateRange) ([]domain.AthleteSummary, error)
}

type CalendarReader interface {
	ListEvents(context.Context, domain.EventQuery) ([]domain.CalendarEvent, error)
	GetEvent(context.Context, domain.EventID) (*domain.CalendarEvent, error)
}

type Repository interface {
	AthleteReader
	ActivityReader
	RecoveryReader
	CalendarReader
}
