# Agent Notes

- This is a Go 1.27.1 project for a read-only MCP server fronting Intervals.icu, deployed as a Cloudflare Worker (GitHub OAuth) + Container.
- Main server entrypoint: `cmd/intervals-mcp`; CLI entrypoint: `cmd/intervals-cli`.
- Worker sources live in `worker/src` (OAuth, allowlist, container proxying) and tests in `worker/test`. `wrangler.jsonc` and `Dockerfile` define the deployment.
- Reuse existing config, Intervals client, and insights layers instead of duplicating API calls: `internal/config`, `internal/intervals`, `internal/insights`.
- Config loads `.env`; the Intervals API uses Basic Auth username `API_KEY` and password `INTERVALS_ICU_API_KEY`.
- Keep the server and CLI read-only unless explicitly asked otherwise. Never print secrets.
- Prefer small, focused changes that preserve existing package boundaries and tests.
- Use `GOCACHE=/tmp/go-build-cache` if the default Go cache is read-only.
- Use `-buildvcs=false` for builds/tests if VCS metadata causes issues.

Security invariants:

- The Go server has no authentication of its own. Authentication is GitHub OAuth plus the explicit `ALLOWED_GITHUB_USERS` allowlist in the Worker, and it fails closed when the allowlist is empty.
- The Worker strips caller credentials and forwarding headers before proxying, and sets `x-intervals-authenticated-user` itself.
- Container HTTPS egress stays deny-by-default with only `intervals.icu` allowed.
- Never print, commit, or retain Intervals.icu API keys, OAuth secrets, cookie keys, or tokens.

Quality gates:

```sh
make fix
make fmt
make vet
make test
golangci-lint run
```

Run `golangci-lint run` only if it is installed.

For Worker changes:

```sh
npm run typecheck
npm run test:worker
npm run format:check
```
