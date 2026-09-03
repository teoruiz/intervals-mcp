# intervals-mcp

Personal read-only MCP server and CLI for querying one Intervals.icu athlete.

It is meant for questions like:

- "What did I do today?"
- "Show my recent rides with training load and decoupling."
- "What is planned on my calendar this week?"
- "Fetch that activity and include intervals."
- "How does today's recovery/wellness look?"

The server runs unauthenticated on your machine, or in a Cloudflare Container
behind a Worker that handles GitHub OAuth and an explicit user allowlist.
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
| `list_wellness` | Daily wellness records for a date range, including custom wellness fields. |
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
  hydration, readiness, steps, SpO2, respiration, VO2 max, blood pressure,
  comments, macros, and custom wellness fields (for example Garmin stress or
  Body Battery when those keys are enabled in Intervals.icu Garmin settings);
  wellness data is daily — Intervals.icu does not store intraday values;
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
Use intervals to plot my stress, HRV, and sleep over the last month.
Fetch the first activity from that search and include intervals.
```

Example tool inputs, for clients that show or support direct MCP tool calls:

```jsonl
{"tool": "today_context", "arguments": {"date": "2026-06-02"}}
{"tool": "list_recent_activities", "arguments": {"oldest": "2026-05-01", "newest": "2026-06-02", "limit": 10}}
{"tool": "get_activity", "arguments": {"id": "ACTIVITY_ID", "include_intervals": true}}
{"tool": "get_recovery", "arguments": {"date": "2026-06-02"}}
{"tool": "list_wellness", "arguments": {"oldest": "2026-05-05", "newest": "2026-06-02"}}
{"tool": "list_calendar", "arguments": {"oldest": "2026-06-02", "newest": "2026-06-09", "categories": ["WORKOUT"]}}
{"tool": "search", "arguments": {"query": "threshold"}}
{"tool": "fetch", "arguments": {"id": "activity:ACTIVITY_ID"}}
```

If your MCP client needs HTTP instead of stdio, keep using the compatibility
server:

```sh
intervals-mcp
# or from the repo:
go run -buildvcs=false ./cmd/intervals-mcp
```

Register HTTP clients at `http://127.0.0.1:8080/mcp`.
The server has no authentication of its own. Keep it bound to `127.0.0.1`;
anything that can reach the address can read the configured athlete's data.
Remote deployments put the Cloudflare Worker in front of it.

## CLI Examples

The CLI only needs the Intervals.icu env vars.

```sh
intervals today
intervals activities --oldest 2026-05-01 --newest 2026-06-02 --limit 10
intervals activity <id> --intervals --json
intervals recovery --date 2026-06-02
intervals wellness --oldest 2026-05-05 --newest 2026-06-02
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

## Remote MCP on Cloudflare

Remote deployment is a Cloudflare Worker in front of a Cloudflare Container:

```text
MCP client
   │ HTTPS /mcp + OAuth bearer token
   ▼
Cloudflare Worker
   ├─ GitHub OAuth + consent + ALLOWED_GITHUB_USERS
   ├─ OAuth grants/tokens in Workers KV
   └─ strips credentials and proxies to one named Container instance
                                      │
                                      ▼
Go MCP container (unauthenticated, scales to zero after 5 minutes)
   └─ HTTPS egress deny-by-default, only intervals.icu allowed
```

Only the OAuth KV namespace is persistent. The container holds no state.

Requirements:

- Node.js 24 or newer, which the OAuth provider package requires.
- Docker with an amd64-capable builder.
- A Cloudflare account on the Workers Paid plan with Containers available.
- One Workers KV namespace for OAuth state, grants, registrations, and tokens.
- Separate local and production GitHub OAuth Apps.

The production GitHub OAuth App needs the Worker's public URL as its callback,
and you only learn that URL by deploying. So deploy first, then create the app,
then add the secrets. Deploying before the secrets exist is safe: the allowlist
fails closed while `ALLOWED_GITHUB_USERS` is unset, so nobody can authorize.

### 1. Install and pick a KV namespace

```sh
npm ci
wrangler kv namespace list
```

`wrangler.jsonc` already points at the account's existing `OAUTH_KV` namespace.
That namespace is shared with another Worker. Sharing is safe, because each
Worker pins `resourceMetadata.resource` to its own `/mcp` URL and rejects any
token whose audience is not exactly that URL, so tokens never cross between
services. It does couple the two services' OAuth state, so for a clean split
create a dedicated namespace and put its id in `wrangler.jsonc`:

```sh
wrangler kv namespace create OAUTH_KV_INTERVALS
```

### 2. Deploy once to claim the URL

```sh
npm run deploy
```

Note the `https://intervals-mcp.<your-subdomain>.workers.dev` URL it prints.
The first deploy builds and pushes the container image, so it takes a few
minutes.

### 3. Create the production GitHub OAuth App

Set its homepage to that Worker URL and its callback to that URL plus
`/callback`. Create a second, separate app for local development with homepage
`http://localhost:8787` and callback `http://localhost:8787/callback`.

### 4. Add the secrets

Copy `.env.example` to `.env`, fill in the six values, and upload them in one
request:

```sh
cp .env.example .env
wrangler secret bulk .env
```

`secret bulk` uploads every uncommented key in the file, which is why
`.env.example` keeps its optional local-only settings commented out. The upload
redeploys the Worker, so no separate deploy is needed. If you change `worker/`
or the Go server afterwards, run `npm run deploy` again.

`.env` holds the production GitHub OAuth App and is also the dotenv file the Go
server and CLI read locally. `.dev.vars` is separate and holds the local OAuth
App for `npm run dev`. Both are gitignored.

Generate the cookie key with `openssl rand -hex 32`. The allowlist is a
comma-separated set of GitHub logins and fails closed when empty. The OAuth
provider supports PKCE, protected-resource and authorization-server discovery,
Client ID Metadata Documents, and Dynamic Client Registration for older clients.
Users see a local consent page after GitHub authentication.

### 5. Verify the deployment

#### Container sleep and cold starts

The container sleeps after `sleepAfter` of inactivity, currently `30s`, set in
`worker/src/index.ts`. The timer resets when the last in-flight request
finishes. Sleeping costs nothing in correctness: the MCP handler is stateless,
so no session lives in the container.

The application contributes almost nothing to a cold start. Measured locally,
the Go server takes about 13 ms from process start to serving, and about 150 ms
from `docker run` to a healthy `/healthz`.

Measured on the deployment, a cold `today_context` call costs:

| Phase | Time |
| --- | ---: |
| Waking the sleeping container (`wake_ms`) | 576 ms |
| Go server handling, five upstream Intervals.icu calls (`handle_ms`) | 333 ms |
| End to end through the Worker (`duration_ms`) | 1,020 ms |

A warm call skips the first row entirely. Under a second for a cold start is
well inside the noise of a model turn, which is what makes a short `sleepAfter`
worth it.

Every proxied call is logged, so `wrangler tail` gives you the number directly:

```sh
wrangler tail --format pretty
```

Two lines per request:

- `{"msg":"container request","cold":true,"wake_ms":...,"handle_ms":...}` from
  the container's Durable Object. `wake_ms` is the wake itself, isolated: it is
  the time spent in `startAndWaitForPorts` before the request is served, and is
  `0` whenever `cold` is `false`. `handle_ms` is the Go server's own work.
- `{"msg":"mcp proxy","status":...,"duration_ms":...}` from the Worker, which
  is the end-to-end cost including the hop to the Durable Object.

To force a cold sample, leave the server idle for longer than `sleepAfter`, then
make one tool call and read `wake_ms`. Make a second call right after for the
warm comparison. The sleep itself is visible in the same stream as
`Activity expired, signalling container to stop`, one `sleepAfter` after the
last request.

Tune from there: `sleepAfter` accepts `30s`, `1m`, `2m` and so on. Shorter means
less idle billing and more frequent wakes. Raise it if follow-up questions
within a conversation routinely land on a cold container.

Note that container start does not work under local `wrangler dev` on every
machine. Outbound HTTPS interception has to set up a proxy container first, and
that can time out locally with `Container failed to start`, independent of any
application code. Measure cold starts against the deployment, not locally.

The Wrangler configuration deploys exactly one `basic` instance: 1/4 vCPU,
1 GiB memory, and 4 GB disk. Custom instance sizes are an enterprise-plan
feature and must provide at least 3 GiB of memory per vCPU, so use a named type
(`lite`, `basic`, `standard-1` and up). See the
[Cloudflare Containers docs](https://developers.cloudflare.com/containers/),
[pricing](https://developers.cloudflare.com/containers/platform/pricing/), and
[remote MCP guide](https://developers.cloudflare.com/agents/model-context-protocol/guides/remote-mcp-server/).

After deploy, provisioning can take several minutes:

```sh
wrangler containers list
wrangler containers images list
wrangler tail
```

Use the MCP Inspector's OAuth flow against the public `/mcp` URL. Confirm:

1. An unauthenticated request receives `401` and protected-resource metadata.
2. A non-allowlisted GitHub account receives `403`.
3. Consent returns to the Inspector and exactly eight tools are discovered.
4. A tool request cold-starts the singleton; a second request reuses it.
5. No outbound host other than `intervals.icu` is allowed.

Worker endpoints:

- MCP: `/mcp`
- Authorization: `/authorize`, `/callback`, `/consent`
- Token: `/token`
- Dynamic client registration: `/register`
- Discovery: `/.well-known/oauth-protected-resource` and
  `/.well-known/oauth-authorization-server`

The Go server itself also exposes `/healthz` and `/readyz` inside the container.

### Local development

Local development does not have the URL bootstrap problem, because the address
is always `http://localhost:8787`. Using the local GitHub OAuth App from step 3:

```sh
cp .dev.vars.example .dev.vars
npm run typecheck
npm run test:worker
npm run dev
```

Then check that `curl -i localhost:8787/mcp` returns `401` with a
`WWW-Authenticate` header, and run the MCP Inspector's OAuth flow against
`http://localhost:8787/mcp`.

## Container Image

```sh
make docker-build
docker run --rm -p 8080:8080 \
  -e INTERVALS_ICU_API_KEY=... \
  -e INTERVALS_ICU_ATHLETE_ID=... \
  intervals-mcp:local
```

The image targets `linux/amd64`, which Cloudflare Containers requires. It sets
`MCP_ADDR=0.0.0.0:8080` and runs as a non-root user.

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
and `config doctor` redact secret keys, but print non-secret identifiers such as
the athlete id in cleartext — scrub their output before pasting it into issues
or logs.

Required for every mode:

- `INTERVALS_ICU_API_KEY`: Intervals.icu API key.
- `INTERVALS_ICU_ATHLETE_ID`: Intervals.icu athlete id.
- `INTERVALS_ICU_BASE_URL`: optional, defaults to `https://intervals.icu`.

Used by the HTTP server only:

- `MCP_ADDR`: listen address. The `--addr` flag wins, then a process
  environment `MCP_ADDR`, then `127.0.0.1:8080`.
- `REQUEST_TIMEOUT` and `SHUTDOWN_TIMEOUT`: optional durations.

Worker secrets are `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`,
`COOKIE_ENCRYPTION_KEY`, `ALLOWED_GITHUB_USERS`, and the two Intervals.icu
values the Worker passes into the container. Local Worker development reads
them from `.dev.vars`; production takes them from `.env` via
`wrangler secret bulk`. The Go server ignores the GitHub keys.

## Development

Use Go 1.26.3, or the version declared in `go.mod`.

```sh
make fix
make fmt
make check
```

`make check` runs a non-mutating formatter check, `go vet`, race tests, and
`golangci-lint run`, so it expects `golangci-lint` to be installed.

For the Worker:

```sh
make worker-install
make worker-test
npm run format:check
```

GitHub Actions runs the same non-mutating Go and Worker checks on pushes and
pull requests.

## Privacy

Intervals.icu activity, wellness, recovery, and calendar data can be sensitive.
Do not commit real `.env` or `.dev.vars` files, API keys, OAuth client secrets,
cookie keys, access tokens, authorization headers, screenshots, or logs
containing personal data.

## License

MIT. See `LICENSE`.
