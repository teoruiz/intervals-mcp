# intervals-mcp

Read-only MCP server and CLI for asking questions about one Intervals.icu
athlete.

The remote MCP server is protected with Supabase OAuth/OIDC. The CLI and local
MCP mode use the same Intervals.icu client and insight layer, but only require
Intervals.icu credentials.

## Read-Only Scope

This project exposes activity, recovery, athlete summary, wellness, and calendar
context. The MCP tools, CLI commands, and Intervals.icu client are intentionally
read-only and only use `GET` requests to Intervals.icu.

The bundled `docs/openapi-spec.json` is upstream Intervals.icu reference
material. It includes upstream write and delete endpoints, but it does not
define this MCP server's writable surface. See `docs/README.md` for details.

## Prerequisites

- Go 1.26.3, or the version declared in `go.mod`.
- An Intervals.icu API key and athlete id.
- A Supabase project for authenticated remote MCP deployments.
- Optional: Docker, Fly.io, `prek`, and `golangci-lint` for deployment and
  development workflows.

## Configuration

Copy `.env.example` to `.env` and fill in the values for the mode you are using.

Required for every mode:

- `INTERVALS_ICU_API_KEY`: Intervals.icu API key.
- `INTERVALS_ICU_ATHLETE_ID`: Intervals.icu athlete id.
- `INTERVALS_ICU_BASE_URL`: optional, defaults to `https://intervals.icu`.

Required for the authenticated server:

- `MCP_PUBLIC_URL`: public HTTPS origin where this server is deployed.
- `SUPABASE_URL` and `SUPABASE_ANON_KEY`: Supabase project values.
  `SUPABASE_PUBLISHABLE_KEY` is also accepted as an alias for newer Supabase
  projects.
- `OIDC_ALLOWED_EMAIL` or `OIDC_ALLOWED_SUBJECT`: the single Supabase identity
  allowed to access the configured Intervals.icu athlete.
- `MCP_REQUIRED_SCOPE`: optional. Leave blank for Supabase OAuth Server unless
  you have verified issued access tokens include the required scope claim.

Supabase Auth should have email/password sign-in enabled. Supabase OAuth Server
should be enabled with Dynamic Client Registration and Authorization Path set to
`/oauth/consent`.

`SUPABASE_OAUTH_PROVIDERS` only controls extra social-login buttons on the
consent page. By default no social-login provider buttons are shown. Set a
comma-separated list such as `SUPABASE_OAUTH_PROVIDERS=github,google` only for
providers enabled in your Supabase project.

## Authenticated MCP Server

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

The server listens on `MCP_ADDR`, defaulting to `:8080`.

## Local Unauthenticated MCP

For local use you can skip Supabase/OIDC entirely and run the same read-only
tools over plain HTTP. Only the Intervals.icu fields in `.env` are required.

```sh
make run-local
# or
go run -buildvcs=false ./cmd/intervals-mcp --local
```

This serves the MCP endpoint at `http://127.0.0.1:8080/mcp` with no
authentication and binds to loopback by default. Override the listen address
with `--addr` or a shell `MCP_ADDR` value, and the dotenv path with `--env PATH`.
`MCP_ADDR` values loaded from `.env` are ignored in local mode so the shared
authenticated-server config cannot expose local mode by accident.

Register it with an MCP client, for example Claude Code:

```sh
claude mcp add --transport http intervals http://127.0.0.1:8080/mcp
```

The local server is unauthenticated. Keep it bound to `127.0.0.1`: anything that
can reach the address has full read access to the configured athlete's data.

## CLI

The read-only CLI uses the same Intervals.icu client and insights service as the
MCP server, but it only requires the Intervals.icu fields in `.env`.

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

Use `--env PATH` to load a different dotenv file and `--json` for pipeable
output.

## Docker and Fly.io

Build the server image:

```sh
docker build --build-arg GO_VERSION=1.26.3 -t intervals-mcp .
```

Run it with an env file:

```sh
docker run --rm --env-file .env -p 8080:8080 intervals-mcp
```

`fly.toml` is an example Fly.io app config. Copy or edit it for your own app
name, region, and scaling settings. Store credentials with `fly secrets set`;
do not put Intervals.icu or Supabase secrets in `fly.toml`.

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

Expanded gate sequence:

```sh
make fix
make fmt
make vet
make test
golangci-lint run
```

`make fix` runs `go fix ./...`, and `make fmt` runs `gofmt -w .`. `make check`
uses the non-mutating formatter check, vet, race tests, and lint target. Run
`golangci-lint run` only when it is installed.

Pre-commit hooks are optional:

```sh
uv tool install prek
make hooks
make precommit
```

If `prek` is already installed but older than the configured minimum, run
`uv tool upgrade prek`.

GitHub Actions runs the non-mutating checks on pushes and pull requests.

## Privacy and Security

Intervals.icu activity, wellness, recovery, and calendar data can be sensitive
personal health data. Treat MCP responses, CLI output, logs, screenshots, and
issue attachments as private unless the athlete has chosen to publish them.

Never commit or paste Intervals.icu API keys, Supabase passwords, bearer tokens,
JWTs, authorization headers, or real `.env` files. If a secret is exposed,
rotate it before sharing details publicly.

Use HTTPS for remote MCP deployments. Keep local unauthenticated mode bound to
loopback unless you fully control the network path.

## License

MIT. See `LICENSE`.
