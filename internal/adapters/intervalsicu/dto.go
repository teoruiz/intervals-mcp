package intervalsicu

import "github.com/teoruiz/intervals-mcp/internal/domain"

type athleteDTO domain.Athlete
type activityDTO domain.Activity
type activityStreamDTO domain.ActivityStream
type recoveryDTO domain.Recovery
type athleteSummaryDTO domain.AthleteSummary
type calendarEventDTO domain.CalendarEvent

func mapAthlete(dto athleteDTO) domain.Athlete {
	return domain.Athlete(dto)
}

func mapActivity(dto activityDTO) domain.Activity {
	return domain.Activity(dto)
}

func mapActivities(dtos []activityDTO) []domain.Activity {
	out := make([]domain.Activity, len(dtos))
	for i, dto := range dtos {
		out[i] = mapActivity(dto)
	}
	return out
}

func mapActivityStream(dto activityStreamDTO) domain.ActivityStream {
	return domain.ActivityStream(dto)
}

func mapActivityStreams(dtos []activityStreamDTO) []domain.ActivityStream {
	out := make([]domain.ActivityStream, len(dtos))
	for i, dto := range dtos {
		out[i] = mapActivityStream(dto)
	}
	return out
}

func mapRecovery(dto recoveryDTO) domain.Recovery {
	return domain.Recovery(dto)
}

func mapAthleteSummary(dto athleteSummaryDTO) domain.AthleteSummary {
	return domain.AthleteSummary(dto)
}

func mapAthleteSummaries(dtos []athleteSummaryDTO) []domain.AthleteSummary {
	out := make([]domain.AthleteSummary, len(dtos))
	for i, dto := range dtos {
		out[i] = mapAthleteSummary(dto)
	}
	return out
}

func mapCalendarEvent(dto calendarEventDTO) domain.CalendarEvent {
	return domain.CalendarEvent(dto)
}

func mapCalendarEvents(dtos []calendarEventDTO) []domain.CalendarEvent {
	out := make([]domain.CalendarEvent, len(dtos))
	for i, dto := range dtos {
		out[i] = mapCalendarEvent(dto)
	}
	return out
}
