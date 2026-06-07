# DDD-Lite Refactor Goal

Use this as the reference context for a `/goal` run.

## Architectural Direction

This repo is small, so use DDD-lite rather than a large enterprise-style package tree. The goal is clear dependency flow, not more ceremony.

Target dependency rule:

```text
cmd -> internal/platform/runtime -> internal/adapters -> internal/application -> internal/domain
```

Rules:

- `internal/domain` must not import MCP, CLI, config, auth, HTTP clients, or Intervals.icu adapter packages.
- `internal/application/insights` owns use-case orchestration and repository/client ports.
- `internal/adapters/intervalsicu` owns Intervals.icu API DTOs, HTTP requests, auth headers, endpoint paths, JSON tags, API errors, and DTO-to-domain mapping.
- `internal/adapters/mcp` and `internal/adapters/cli` are delivery adapters. They should call application services and preserve the public interface.
- `internal/platform/runtime` is the composition root that wires config, adapters, and application services.
- Keep the server and CLI read-only unless explicitly asked otherwise.
- Never print secrets.

## Proposed Package Layout

```text
cmd/
  intervals/
  intervals-mcp/

internal/
  domain/
    athlete.go
    activity.go
    recovery.go
    calendar.go
    summary.go
    running_dynamics.go
    date.go

  application/
    insights/
      service.go
      ports.go
      dto.go
      search.go
      nutrition.go

  adapters/
    intervalsicu/
      client.go
      dto.go
      mapper.go
      streams.go

    mcp/
      server.go

    cli/
      app.go
      runner.go
      config.go

    httpauth/
      jwt.go

    oauthui/
      handler.go

  platform/
    config/
      config.go

    runtime/
      runtime.go
```

## Current To Target Mapping

```text
internal/intervals/client.go       -> internal/adapters/intervalsicu/client.go
internal/intervals/types.go        -> split between internal/domain and internal/adapters/intervalsicu/dto.go
internal/intervals/streams.go      -> domain running-dynamics logic plus intervalsicu stream DTO/request details
internal/insights/*                -> internal/application/insights/*
internal/mcpserver/*               -> internal/adapters/mcp/*
internal/cli/*                     -> internal/adapters/cli/*
internal/config/*                  -> internal/platform/config/*
internal/runtime/*                 -> internal/platform/runtime/*
internal/auth/*                    -> internal/adapters/httpauth/*
internal/oauthui/*                 -> internal/adapters/oauthui/*
```

## Domain Concepts

Start with these domain concepts:

- `Athlete`: identity, profile, timezone.
- `Activity`: training activity facts, load, distance, time, energy, cadence, running metrics, intervals.
- `ActivityStream`: normalized stream data only if needed by domain calculations.
- `RunningDynamics`: calculated Garmin running-dynamics summaries.
- `Recovery` or `Wellness`: wellness and readiness data.
- `AthleteSummary`: fitness, fatigue, form, load, daily totals.
- `CalendarEvent`: planned workouts, notes, races, and other calendar entries.
- `DateRange`: common oldest/newest query shape.

Keep domain names independent from Intervals.icu field names where practical. JSON tags can remain on application DTOs or adapter DTOs, but should not drive the domain model unless preserving current output shape requires a transitional compromise.

## Application Layer

`internal/application/insights` should expose the use cases currently implemented by `internal/insights`:

- `TodayContext`
- `RecentActivities`
- `Activity`
- `Recovery`
- `Calendar`
- `Search`
- `Fetch`

It should define ports that describe what the use cases need, for example:

```go
type AthleteReader interface {
	GetAthlete(context.Context) (*domain.Athlete, error)
}

type ActivityReader interface {
	ListActivities(context.Context, domain.ActivityQuery) ([]domain.Activity, error)
	GetActivity(context.Context, domain.ActivityID, domain.ActivityDetailOptions) (*domain.Activity, error)
	GetActivityStreams(context.Context, domain.ActivityID, []domain.StreamType) ([]domain.ActivityStream, error)
}

type RecoveryReader interface {
	GetRecovery(context.Context, domain.LocalDate) (*domain.Recovery, error)
	GetAthleteSummary(context.Context, domain.DateRange) ([]domain.AthleteSummary, error)
}

type CalendarReader interface {
	ListEvents(context.Context, domain.EventQuery) ([]domain.CalendarEvent, error)
	GetEvent(context.Context, domain.EventID) (*domain.CalendarEvent, error)
}
```

The exact port shape should follow the code as it is refactored. Prefer small interfaces owned by the application package over a single large concrete client dependency.

## Adapter Layer

`internal/adapters/intervalsicu` should:

- Keep `Config`, `Client`, `APIError`, and `ErrNotFound`.
- Keep Basic Auth username `API_KEY` and password `INTERVALS_ICU_API_KEY`.
- Keep endpoint path construction and query parameter construction.
- Decode Intervals.icu JSON into adapter DTOs.
- Normalize Intervals-specific quirks, such as running cadence per leg.
- Map DTOs into `internal/domain` types before returning to the application layer.
- Keep Intervals.icu stream type names private unless the application truly needs to request by name.

## Delivery Adapters

`internal/adapters/mcp` should preserve existing MCP tools:

- `today_context`
- `list_recent_activities`
- `get_activity`
- `get_recovery`
- `list_calendar`
- `search`
- `fetch`

`internal/adapters/cli` should preserve existing CLI commands and output behavior.

The safest first pass is to keep application response DTOs JSON-compatible with the current `internal/insights` output so MCP and CLI consumers do not see a breaking change.

## Migration Plan

1. Move `internal/insights` to `internal/application/insights` with import-path updates only.
2. Move `internal/mcpserver` to `internal/adapters/mcp` with import-path updates only.
3. Move `internal/cli` to `internal/adapters/cli` with import-path updates only.
4. Move `internal/config` to `internal/platform/config` and `internal/runtime` to `internal/platform/runtime`.
5. Move `internal/auth` to `internal/adapters/httpauth` and keep `internal/oauthui` under `internal/adapters/oauthui`.
6. Introduce `internal/domain` types that mirror the stable business concepts.
7. Add mapper functions in `internal/adapters/intervalsicu`.
8. Change `application/insights` ports to return domain types instead of adapter DTOs.
9. Move running-dynamics calculations into the domain where they are not tied to Intervals.icu transport details.
10. Delete transitional aliases and clean up package names after tests pass.

Do this in small commits or small refactor steps. Avoid mixing package moves, behavior changes, and output changes in the same step.

## Quality Gates

Run the existing gates:

```sh
make fix
make fmt
make vet
make test
golangci-lint run
```

Use these project-specific flags if needed:

```sh
GOCACHE=/tmp/go-build-cache go test -buildvcs=false ./...
```

Run `golangci-lint run` only if it is installed.

## Non-Goals

- Do not add write operations.
- Do not change MCP tool names or schemas in the first refactor.
- Do not change CLI command names, flags, or default output in the first refactor.
- Do not introduce a database or persistence abstraction that is not needed.
- Do not split into many tiny bounded-context packages until the codebase is large enough to justify it.
- Do not duplicate Intervals.icu API calls outside the adapter.
