# intervals-mcp

Remote MCP server for asking read-only questions about one Intervals.icu athlete.

## Configuration

Copy `.env.example` to `.env` and fill in:

- `INTERVALS_ICU_API_KEY`: Intervals API key.
- `INTERVALS_ICU_ATHLETE_ID`: Intervals athlete id.
- `MCP_PUBLIC_URL`: public HTTPS origin where this server is deployed.
- `SUPABASE_URL` and `SUPABASE_ANON_KEY`: Supabase project values. `SUPABASE_PUBLISHABLE_KEY` is also accepted as an alias for newer Supabase projects.
- `OIDC_ALLOWED_EMAIL` or `OIDC_ALLOWED_SUBJECT`: the single Supabase identity allowed to access the Intervals data.
- `MCP_REQUIRED_SCOPE`: optional. Leave blank for Supabase OAuth Server unless you have verified issued access tokens include the required scope claim.

Supabase Auth should have email/password sign-in enabled. OAuth Server should be enabled with Dynamic Client Registration and Authorization Path set to `/oauth/consent`. `SUPABASE_OAUTH_PROVIDERS` is optional and only controls extra social-login buttons on the consent page.

## Run

```sh
make run
```

The MCP endpoint is `/mcp`. The OAuth consent page is `/oauth/consent`.

## CLI

The read-only CLI uses the same Intervals client and insights service as the MCP server, but it only requires the Intervals fields in `.env`:

- `INTERVALS_ICU_API_KEY`
- `INTERVALS_ICU_ATHLETE_ID`
- `INTERVALS_ICU_BASE_URL` optional, defaults to `https://intervals.icu`

Examples:

```sh
go run -buildvcs=false ./cmd/intervals-cli today
go run -buildvcs=false ./cmd/intervals-cli activities --oldest 2026-05-01 --newest 2026-06-02 --limit 10
go run -buildvcs=false ./cmd/intervals-cli activity <id> --intervals --json
go run -buildvcs=false ./cmd/intervals-cli recovery --date 2026-06-02
go run -buildvcs=false ./cmd/intervals-cli calendar --category WORKOUT
go run -buildvcs=false ./cmd/intervals-cli search ride
go run -buildvcs=false ./cmd/intervals-cli explore
```

Use `--env PATH` to load a different dotenv file and `--json` for pipeable output.

## Quality Gates

```sh
make fix
make fmt
make vet
make test
make lint
```

`make fix` runs `go fix ./...` so new Go modernizations are applied as part of the normal workflow.
