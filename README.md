# intervals-mcp

Personal read-only MCP server and CLI for querying one Intervals.icu athlete.

It is meant for questions like:

- "What did I do today?"
- "Show my recent rides with training load and decoupling."
- "What is planned on my calendar this week?"
- "Fetch that activity and include intervals."
- "How does today's recovery/wellness look?"

The server can run locally without auth, or remotely behind Supabase OAuth/OIDC.
The CLI uses the same read-only Intervals.icu client as the MCP server.

You need Go 1.26.3, or the version declared in `go.mod`, plus an Intervals.icu
API key and athlete id.

Install the unified CLI with:

```sh
go install github.com/teoruiz/intervals-mcp/cmd/intervals@latest
```

Until a versioned release is tagged, `@latest` resolves to the latest commit on
the default branch. To build from a checkout instead:

```sh
git clone https://github.com/teoruiz/intervals-mcp
go install ./cmd/intervals
```

Compatibility binaries remain available:

```sh
go install github.com/teoruiz/intervals-mcp/cmd/intervals-cli@latest
go install github.com/teoruiz/intervals-mcp/cmd/intervals-mcp@latest
```

## What It Exposes

The MCP server exposes these tools:

| Tool | Data exposed |
| --- | --- |
| `today_context` | Today's activities, wellness/recovery, athlete summary, planned events, and fueling context. |
| `list_recent_activities` | Recent Intervals.icu activities in descending date order. |
| `get_activity` | One activity by id, optionally including interval data. |
| `get_recovery` | Wellness and fitness summary data for a date. |
| `list_calendar` | Planned workouts, notes, and other calendar events. |
| `search` | Search recent activities, today's recovery, and upcoming planned events. |
| `fetch` | Fetch a record returned by `search`, such as an activity, recovery date, or calendar event. |

Under the hood this currently reads only these Intervals.icu areas:

- athlete profile basics: id, name, email, and timezone;
- athlete summary: fitness, fatigue, form, ramp rate, training load, calories,
  distance, moving time, weight, eFTP, time in zones, and category summaries;
- activities and activity details: name, type, dates, distance, moving/elapsed
  time, calories, carbs, training load, ATL/CTL, heart rate, intensity,
  efficiency, power/HR, decoupling, RPE/feel, description, tags, interval
  summary, and optional interval records;
- wellness/recovery records: CTL/ATL, ramp rate, weight, resting HR, HRV,
  calories consumed, sleep, soreness, fatigue, stress, mood, motivation,
  hydration, readiness, comments, and macros;
- calendar events: planned workouts, notes, dates, duration/distance/load
  targets, intensity, carbs, indoor flag, and tags.

It does not create, update, or delete anything in Intervals.icu. The Intervals
client only performs authenticated `GET` requests.

The bundled `docs/openapi-spec.json` is just an upstream Intervals.icu reference
snapshot. It includes upstream write/delete endpoints, but those endpoints are
not exposed by this project. See `docs/README.md`.

## Quick Start: Local MCP

Stdio mode is easiest for day-to-day MCP use on your own machine. It starts on
demand from your MCP client and does not require a separate HTTP server.

1. Install the CLI:

   ```sh
   go install github.com/teoruiz/intervals-mcp/cmd/intervals@latest
   ```

2. Create your config:

   ```sh
   intervals config init
   ```

   This writes dotenv config to `$XDG_CONFIG_HOME/intervals-mcp/config.env`, or
   `~/.config/intervals-mcp/config.env` when `XDG_CONFIG_HOME` is unset.

3. Register stdio MCP with a client using this command and args:

   ```text
   command: intervals
   args: ["mcp", "stdio"]
   ```

   Example MCP client JSON:

   ```json
   {
     "mcpServers": {
       "intervals": {
         "command": "intervals",
         "args": ["mcp", "stdio"]
       }
     }
   }
   ```

Then ask your MCP client things like:

```text
Use intervals to summarize today's training context.
Use intervals to list my last 10 activities.
Use intervals to search for threshold rides.
Use intervals to show my planned workouts this week.
Fetch the first activity from that search and include intervals.
```

Example tool inputs, for clients that show or support direct MCP tool calls:

```jsonl
{"tool": "today_context", "arguments": {"date": "2026-06-02"}}
{"tool": "list_recent_activities", "arguments": {"oldest": "2026-05-01", "newest": "2026-06-02", "limit": 10}}
{"tool": "get_activity", "arguments": {"id": "ACTIVITY_ID", "include_intervals": true}}
{"tool": "get_recovery", "arguments": {"date": "2026-06-02"}}
{"tool": "list_calendar", "arguments": {"oldest": "2026-06-02", "newest": "2026-06-09", "categories": ["WORKOUT"]}}
{"tool": "search", "arguments": {"query": "threshold"}}
{"tool": "fetch", "arguments": {"id": "activity:ACTIVITY_ID"}}
```

If your MCP client needs HTTP instead of stdio, keep using the compatibility
server:

```sh
intervals-mcp --local
# or from the repo:
go run -buildvcs=false ./cmd/intervals-mcp --local
```

Register HTTP clients at `http://127.0.0.1:8080/mcp`.
HTTP local mode is unauthenticated. Keep it bound to `127.0.0.1`; anything that
can reach the address can read the configured athlete's data.

## CLI Examples

The CLI only needs the Intervals.icu env vars.

```sh
intervals today
intervals activities --oldest 2026-05-01 --newest 2026-06-02 --limit 10
intervals activity <id> --intervals --json
intervals recovery --date 2026-06-02
intervals calendar --category WORKOUT
intervals search ride
intervals explore
```

Useful config commands:

```sh
intervals config init
intervals config path
intervals config doctor
intervals config show
```

Use `--env PATH` to load a specific dotenv file and `--json` for pipeable
output. `intervals-cli` remains available for existing scripts, but new
installations should use `intervals`.

## Remote MCP With Supabase

For a remote MCP server, fill in the authenticated-server fields in `.env`:

```sh
MCP_PUBLIC_URL=https://your-domain.example
SUPABASE_URL=https://your-project-ref.supabase.co
SUPABASE_ANON_KEY=...
OIDC_ALLOWED_EMAIL=you@example.com
```

Supabase setup:

- enable email/password sign-in;
- enable Supabase OAuth Server;
- enable Dynamic Client Registration;
- set Authorization Path to `/oauth/consent`;
- optionally set `SUPABASE_OAUTH_PROVIDERS=github,google` if those providers
  are enabled in Supabase and you want extra login buttons.

Run the authenticated server:

```sh
make run
# or
go run -buildvcs=false ./cmd/intervals-mcp
```

Endpoints:

- MCP: `/mcp`
- OAuth consent: `/oauth/consent`
- OAuth protected resource metadata: `/.well-known/oauth-protected-resource`
- Health: `/healthz`
- Readiness: `/readyz`

## Fly.io

`fly.example.toml` is the tracked reusable config. Copy it to the ignored
`fly.toml` for your deployment:

```sh
cp fly.example.toml fly.toml
```

Edit `fly.toml` with your real app name, region, and `MCP_PUBLIC_URL`.
Store credentials as secrets:

```sh
fly secrets set -a your-real-app-name \
  INTERVALS_ICU_API_KEY=... \
  INTERVALS_ICU_ATHLETE_ID=... \
  SUPABASE_URL=... \
  SUPABASE_ANON_KEY=... \
  OIDC_ALLOWED_EMAIL=...
```

Deploy with:

```sh
fly deploy -c fly.toml
```

For logs and status:

```sh
fly status -a your-real-app-name
fly logs -a your-real-app-name
```

## Configuration Reference

Config is dotenv-style. Exactly one config file is selected, in this order:

1. explicit `--env PATH` (must exist; it is an error if the file is missing);
2. `$XDG_CONFIG_HOME/intervals-mcp/config.env`, or
   `~/.config/intervals-mcp/config.env`;
3. repo-local `.env` for development.

The file sources do not merge: if an XDG config exists, a repo-local `.env` is
not read. Non-empty process environment variables then override whichever file
was selected, so they are effectively highest precedence.

`intervals config init` creates config files with mode `0600`. `config show`
and `config doctor` redact secret keys, but print non-secret identifiers
(athlete id, email, Supabase URL) in cleartext — scrub their output before
pasting it into issues or logs.

Required for every mode:

- `INTERVALS_ICU_API_KEY`: Intervals.icu API key.
- `INTERVALS_ICU_ATHLETE_ID`: Intervals.icu athlete id.
- `INTERVALS_ICU_BASE_URL`: optional, defaults to `https://intervals.icu`.

Required for authenticated remote MCP:

- `MCP_PUBLIC_URL`: public HTTPS origin where this server is deployed.
- `SUPABASE_URL` and `SUPABASE_ANON_KEY`: Supabase project values.
  `SUPABASE_PUBLISHABLE_KEY` is also accepted as an alias.
- `OIDC_ALLOWED_EMAIL` or `OIDC_ALLOWED_SUBJECT`: the single Supabase identity
  allowed to access the configured Intervals.icu athlete.
- `MCP_REQUIRED_SCOPE`: optional. Leave blank for Supabase OAuth Server unless
  you have verified issued access tokens include the required scope claim.

`SUPABASE_OAUTH_PROVIDERS` defaults to empty. Set a comma-separated list only
for social providers enabled in your Supabase project.

## Development

Use Go 1.26.3, or the version declared in `go.mod`.

```sh
make fix
make fmt
make check
```

`make check` runs a non-mutating formatter check, `go vet`, race tests, and
`golangci-lint run`, so it expects `golangci-lint` to be installed.

GitHub Actions runs the same non-mutating checks on pushes and pull requests.

## Privacy

Intervals.icu activity, wellness, recovery, and calendar data can be sensitive.
Do not commit real `.env` files, API keys, Supabase tokens, JWTs, authorization
headers, screenshots, or logs containing personal data.

## License

MIT. See `LICENSE`.
