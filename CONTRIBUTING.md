# Contributing

Thanks for helping improve `intervals-mcp`.

## Setup

Use Go 1.26.3 or the version declared in `go.mod`.

```sh
cp .env.example .env
GOCACHE=/tmp/go-build-cache go test -buildvcs=false ./...
```

For authenticated server development, fill in the Supabase and Intervals fields
in `.env`. For CLI or local MCP development, only the Intervals fields are
required.

Never commit real `.env` files, API keys, bearer tokens, JWTs, passwords, or
personal activity exports.

## Project Boundaries

- Keep the server and CLI read-only unless a change explicitly scopes and
  reviews write behavior.
- Reuse the existing package layers:
  - `internal/config` for environment and dotenv loading;
  - `internal/intervals` for Intervals.icu API access;
  - `internal/insights` for higher-level aggregation used by MCP and CLI.
- Do not duplicate Intervals.icu API calls in command or server packages.
- Do not print secrets. Redact authorization headers and tokens from logs,
  tests, docs, and issue examples.

## Quality Gates

Developer rewrite commands:

```sh
make fix
make fmt
```

Non-mutating checks:

```sh
make check
```

The expanded gate sequence is:

```sh
make fix
make fmt
make vet
make test
golangci-lint run
```

Run `golangci-lint run` when `golangci-lint` is installed. Use
`GOCACHE=/tmp/go-build-cache` if your default Go cache is not writable, and
`-buildvcs=false` if VCS metadata causes build or test issues.

## Pull Requests

Before opening a pull request:

- confirm tests cover the behavior changed;
- keep docs aligned with config and public behavior;
- avoid unrelated formatting or refactors;
- verify no secrets or personal data are included.
