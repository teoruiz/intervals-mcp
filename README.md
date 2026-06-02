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

## Quality Gates

```sh
make fix
make fmt
make vet
make test
make lint
```

`make fix` runs `go fix ./...` so new Go modernizations are applied as part of the normal workflow.
